package event

import "fmt"

// Lwt is Tasmota's last-will message (tele/<topic>/LWT): true for "Online", false for "Offline".
type Lwt bool

func NewLwt(payload string) (Lwt, error) {
	switch payload {
	case "Online":
		return Lwt(true), nil
	case "Offline":
		return Lwt(false), nil
	}
	return Lwt(false), fmt.Errorf("unknown LWT value %s", payload)
}
