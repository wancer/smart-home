package internal

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	LightNone   = ""
	LightRGB    = "rgb"    // WS2812 / 3+ PWM channels: power, dimmer, colour
	LightDimmer = "dimmer" // 1-2 PWM channels: power, dimmer
)

// LedHardware is what the device's GPIO template says about its LEDs.
type LedHardware struct {
	StatusLed bool   // Led / Led_i / LedLink / LedLink_i: LedState, LedPower, LedPwm*
	Light     string // LightNone, LightRGB or LightDimmer
	Relays    int
}

// Tasmota GPIO component code = function id * 32 + index.
const (
	gpioRelay      = 7
	gpioRelayInv   = 8
	gpioLed        = 9
	gpioLedInv     = 10
	gpioPwm        = 13
	gpioPwmInv     = 14
	gpioLedLink    = 17
	gpioLedLinkInv = 18
	gpioWs2812     = 43
)

func DetectLedHardware(gpio []int) LedHardware {
	var hw LedHardware
	pwm, ws := 0, false
	for _, code := range gpio {
		if code <= 0 {
			continue
		}
		switch code / 32 {
		case gpioRelay, gpioRelayInv:
			hw.Relays++
		case gpioLed, gpioLedInv, gpioLedLink, gpioLedLinkInv:
			hw.StatusLed = true
		case gpioPwm, gpioPwmInv:
			pwm++
		case gpioWs2812:
			ws = true
		}
	}

	switch {
	case ws || pwm >= 3:
		hw.Light = LightRGB
	case pwm > 0:
		hw.Light = LightDimmer
	}
	return hw
}

// LightPowerIndex is the POWER<n> of the light: Tasmota puts it after the relays.
// Until the template is known, devices that can't be toggled are assumed to have no relay.
func (s *DeviceState) LightPowerIndex() int {
	if hw := s.Config.LedHardware; hw != nil {
		return hw.Relays + 1
	}
	if s.Device.SupportsToggle {
		return 2
	}
	return 1
}

// HueSatToHex returns the colour at full brightness as "#rrggbb".
func HueSatToHex(hue, sat uint) string {
	h := float64(hue%360) / 60
	s := math.Min(float64(sat), 100) / 100
	c := s
	x := c * (1 - math.Abs(math.Mod(h, 2)-1))
	var r, g, b float64
	switch int(h) {
	case 0:
		r, g = c, x
	case 1:
		r, g = x, c
	case 2:
		g, b = c, x
	case 3:
		g, b = x, c
	case 4:
		r, b = x, c
	default:
		r, b = c, x
	}
	m := 1 - c
	to := func(v float64) int { return int(math.Round((v + m) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", to(r), to(g), to(b))
}

// HexToHueSat parses "#rrggbb" (or "rrggbb") into hue 0..359 and saturation 0..100; brightness is dropped.
func HexToHueSat(hex string) (uint, uint, error) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, fmt.Errorf("invalid colour %q", hex)
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid colour %q", hex)
	}
	r, g, b := float64(v>>16&0xff)/255, float64(v>>8&0xff)/255, float64(v&0xff)/255
	maxC, minC := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	d := maxC - minC
	if maxC == 0 || d == 0 {
		return 0, 0, nil // black / grey: white light
	}
	var h float64
	switch maxC {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return uint(math.Round(h)) % 360, uint(math.Round(d / maxC * 100)), nil
}
