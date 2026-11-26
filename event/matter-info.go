package event

import "strings"

// MatterInfo is tele/<sensor>/INFO of a Matter bridge: the Basic Information cluster of the sensor
// (retained, on report and on connect). Every value is a string and may be empty.
type MatterInfo struct {
	VendorName      string `json:"VendorName"`      // e.g. "IKEA of Sweden"
	ProductName     string `json:"ProductName"`     // e.g. "KLIPPBOK"
	NodeLabel       string `json:"NodeLabel"`       // user-set name
	HardwareVersion string `json:"HardwareVersion"` // e.g. "1"
	SoftwareVersion string `json:"SoftwareVersion"` // e.g. "1.0.0"
	SerialNumber    string `json:"SerialNumber"`
}

// Hardware describes the sensor for the profile's chip field: "IKEA of Sweden KLIPPBOK, HW 1,
// SN ABC123" (empty parts left out; "" when nothing is known).
func (i *MatterInfo) Hardware() string {
	parts := []string{}
	if name := strings.TrimSpace(strings.TrimSpace(i.VendorName) + " " + strings.TrimSpace(i.ProductName)); name != "" {
		parts = append(parts, name)
	}
	if hw := strings.TrimSpace(i.HardwareVersion); hw != "" {
		parts = append(parts, "HW "+hw)
	}
	if sn := strings.TrimSpace(i.SerialNumber); sn != "" {
		parts = append(parts, "SN "+sn)
	}
	return strings.Join(parts, ", ")
}
