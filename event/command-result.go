package event

// CommandResult is the stat/<sensor>/RESULT reply of a Matter bridge to a command:
// {"Identify":"Done"} -> {Command: "Identify", Result: "Done"}; {"Command":"Unknown"} ->
// {Command: "", Result: "Unknown"}. Results: Done, Failed, Error (invalid payload), Busy, Unknown.
type CommandResult struct {
	Command string
	Result  string
}

const (
	CommandDone    = "Done"
	CommandFailed  = "Failed"
	CommandError   = "Error"
	CommandBusy    = "Busy"
	CommandUnknown = "Unknown"
)

// NewCommandResult reads a one-key RESULT payload; false if it isn't one.
func NewCommandResult(raw map[string]any) (*CommandResult, bool) {
	if len(raw) != 1 {
		return nil, false
	}
	for k, v := range raw {
		result, ok := v.(string)
		if !ok {
			return nil, false
		}
		if k == "Command" {
			k = ""
		}
		return &CommandResult{Command: k, Result: result}, true
	}
	return nil, false
}
