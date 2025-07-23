package types

import "encoding/json"

type Command struct {
	ID 	uint32	 `json:"id"`   // Unique identifier for the command
	Name string   `json:"name"` // Name of the command
	Args []string `json:"args"` // Arguments for the command
}

func (c *Command) ToJSON() string {
	data, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	
	return string(data)
}

type CommandResponse struct {
	ID 		uint32 `json:"id"`      // Unique identifier for the command response
	Output string `json:"output"` // Output of the command execution
	Error  string `json:"error"`  // Error message if the command execution failed

	Duration int64  `json:"duration"` // Duration of the command execution in milliseconds
}

func (r *CommandResponse) ToJSON() string {
	data, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	
	return string(data)
}

// CommandData holds metadata about a command execution
// This is used to track the command's execution time and server processing duration.
type CommandData struct {
	ID        uint32   `json:"id"`
	Timestamp int64    `json:"timestamp"` // optional: for tracking when it was issued
	TargetID  string   `json:"target_id"` // optional: which session it was for
}