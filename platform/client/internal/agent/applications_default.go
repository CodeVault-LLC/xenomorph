//go:build !linux

package agent

func collectInstalledApplications() []string {
	return nil
}
