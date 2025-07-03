package common

type Command struct {
	Name string   `json:"name"` // Name of the command
	Args []string `json:"args"` // Arguments for the command
}