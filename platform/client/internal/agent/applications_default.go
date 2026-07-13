//go:build !linux && !windows && !darwin

package agent

func scanInstalledApplications() []string {
	return nil
}
