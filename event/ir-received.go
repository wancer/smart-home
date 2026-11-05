package event

import "strings"

// IrReceived is an IR code seen by the device's receiver:
// tele/<topic>/RESULT {"IrReceived":{"Protocol":"NEC","Bits":32,"Data":"0x00F740BF","Repeat":0}}
type IrReceived struct {
	Protocol string `json:"Protocol"`
	Bits     int    `json:"Bits"`
	Data     string `json:"Data"`
	Repeat   int    `json:"Repeat"`
}

// NormalizedCode returns the code as stored in the DB: "0x" + upper-case hex.
func (e *IrReceived) NormalizedCode() string {
	d := strings.TrimSpace(e.Data)
	if len(d) > 2 && (d[:2] == "0x" || d[:2] == "0X") {
		d = d[2:]
	}
	return "0x" + strings.ToUpper(d)
}
