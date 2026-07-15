package wire

import "testing"

func TestValidateCommandResult(t *testing.T) {
	t.Parallel()

	valid := CommandResult{
		CommandType: "support.notice", State: uint64(CommandResultStateExecuted),
		RespondedAtMilliseconds: 1, ResultRevision: 1,
	}
	if err := ValidateCommandResult(valid); err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*CommandResult)
	}{
		{name: "missing command type", mutate: func(result *CommandResult) { result.CommandType = "" }},
		{name: "unassigned state", mutate: func(result *CommandResult) { result.State = 4 }},
		{name: "missing response time", mutate: func(result *CommandResult) { result.RespondedAtMilliseconds = 0 }},
		{name: "unsupported result revision", mutate: func(result *CommandResult) { result.ResultRevision = 2 }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			candidate := valid
			test.mutate(&candidate)

			if err := ValidateCommandResult(candidate); err == nil {
				t.Fatal("invalid command result accepted")
			}
		})
	}
}
