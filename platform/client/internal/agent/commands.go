package agent

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var allowedCommandTypes = map[string]struct{}{
	"support.notice":             {},
	"support.request_screenshot": {},
}

type CommandApprover interface {
	Approve(cmd CommandEnvelope) (bool, error)
}

type ZenityApprover struct{}

func (z ZenityApprover) Approve(cmd CommandEnvelope) (bool, error) {
	if _, err := exec.LookPath("zenity"); err != nil {
		return false, fmt.Errorf("zenity is required for user consent prompt: %w", err)
	}

	question := fmt.Sprintf(
		"Remote support command request\\n\\nType: %s\\nRequested By: %s\\nReason: %s\\nCommand ID: %s\\n\\nAccept this command?",
		nonEmptyText(cmd.Type),
		nonEmptyText(cmd.RequestedBy),
		nonEmptyText(cmd.Reason),
		nonEmptyText(cmd.CommandID),
	)

	c := exec.Command(
		"zenity",
		"--question",
		"--title", "Xenomorph Remote Support Request",
		"--width", "480",
		"--ok-label", "Allow",
		"--cancel-label", "Deny",
		"--text", question,
	)
	err := c.Run()
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() == 1 {
			return false, nil
		}
	}

	return false, err
}

type CommandDecision struct {
	Result        CommandResultPayload
	DisconnectNow bool
}

func HandleCommandWithConsent(cmd CommandEnvelope, approver CommandApprover, disconnectOnDeny bool) (CommandDecision, error) {
	if approver == nil {
		approver = ZenityApprover{}
	}

	hostname, _ := os.Hostname()
	decision := CommandDecision{
		Result: CommandResultPayload{
			CommandID:      cmd.CommandID,
			Type:           cmd.Type,
			RespondedAt:    time.Now().UTC(),
			ClientHostname: strings.TrimSpace(hostname),
		},
	}

	if err := validateCommandEnvelope(cmd); err != nil {
		decision.Result.Status = "rejected"
		decision.Result.Reason = err.Error()
		decision.Result.UserApproved = false
		decision.Result.DisconnectNow = false
		return decision, nil
	}

	approved, err := approver.Approve(cmd)
	if err != nil {
		decision.Result.Status = "failed"
		decision.Result.Reason = fmt.Sprintf("approval prompt failed: %v", err)
		decision.Result.UserApproved = false
		decision.Result.DisconnectNow = disconnectOnDeny
		decision.DisconnectNow = disconnectOnDeny
		return decision, nil
	}

	if !approved {
		decision.Result.Status = "denied"
		decision.Result.Reason = "user denied command"
		decision.Result.UserApproved = false
		decision.Result.DisconnectNow = disconnectOnDeny
		decision.DisconnectNow = disconnectOnDeny
		return decision, nil
	}

	outcome := executeAllowedCommand(cmd)
	decision.Result.Status = "executed"
	decision.Result.Reason = outcome.reason
	decision.Result.OutputData = outcome.outputData
	decision.Result.UserApproved = true
	decision.Result.DisconnectNow = false
	decision.DisconnectNow = false
	return decision, nil
}

func validateCommandEnvelope(cmd CommandEnvelope) error {
	if strings.TrimSpace(cmd.CommandID) == "" {
		return fmt.Errorf("missing command_id")
	}
	if strings.TrimSpace(cmd.Type) == "" {
		return fmt.Errorf("missing command type")
	}
	if _, ok := allowedCommandTypes[cmd.Type]; !ok {
		return fmt.Errorf("command type %q is not allowed", cmd.Type)
	}

	now := time.Now().UTC()
	if !cmd.ExpiresAt.IsZero() && now.After(cmd.ExpiresAt) {
		return fmt.Errorf("command expired")
	}
	if !cmd.IssuedAt.IsZero() && cmd.IssuedAt.After(now.Add(2*time.Minute)) {
		return fmt.Errorf("command issued_at is in the future")
	}
	if strings.TrimSpace(cmd.Signature) == "" {
		return fmt.Errorf("missing command signature")
	}

	return nil
}

type commandOutcome struct {
	reason     string
	outputData []byte
}

func executeAllowedCommand(cmd CommandEnvelope) commandOutcome {
	switch cmd.Type {
	case "support.notice":
		log.Printf("ℹ️ operator notice acknowledged command_id=%s", cmd.CommandID)
		return commandOutcome{reason: "support notice acknowledged"}
	case "support.request_screenshot":
		data, err := captureScreenshot()
		if err != nil {
			return commandOutcome{reason: fmt.Sprintf("screenshot failed: %v", err)}
		}
		return commandOutcome{reason: "screenshot captured", outputData: data}
	default:
		return commandOutcome{reason: "no-op"}
	}
}

func captureScreenshot() ([]byte, error) {
	tmpDir := "/tmp"
	if d := os.TempDir(); d != "" {
		tmpDir = d
	}

	outputPath := filepath.Join(tmpDir, fmt.Sprintf("xeno-screenshot-%d.png", time.Now().UnixMilli()))

	var (
		cmd  *exec.Cmd
		name string
	)

	if _, err := exec.LookPath("import"); err == nil {
		cmd = exec.Command("import", "-window", "root", outputPath)
		name = "import"
	} else if _, err := exec.LookPath("gnome-screenshot"); err == nil {
		cmd = exec.Command("gnome-screenshot", "-f", outputPath)
		name = "gnome-screenshot"
	} else if _, err := exec.LookPath("scrot"); err == nil {
		cmd = exec.Command("scrot", outputPath)
		name = "scrot"
	} else if _, err := exec.LookPath("maim"); err == nil {
		cmd = exec.Command("maim", outputPath)
		name = "maim"
	} else {
		return nil, fmt.Errorf("no screenshot tool found (install imagemagick, scrot, maim, or gnome-screenshot)")
	}

	log.Printf("📸 capturing screenshot using %s to %s", name, outputPath)

	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%s failed: %w\noutput: %s", name, err, string(out))
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read screenshot: %w", err)
	}

	os.Remove(outputPath)

	return data, nil
}

func LoadDisconnectOnDenyFromEnv() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("XENOMORPH_DISCONNECT_ON_DENY")))
	if raw == "" {
		return true
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return value
}

func nonEmptyText(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "n/a"
	}
	return trimmed
}
