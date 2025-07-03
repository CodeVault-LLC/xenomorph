package types

import "encoding/json"

type Command struct {
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
