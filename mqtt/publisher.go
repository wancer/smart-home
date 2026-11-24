package mqtt

import (
	"fmt"
	"log/slog"
	"smart-home/model"

	driver "github.com/eclipse/paho.mqtt.golang"
)

type Publisher struct {
	client driver.Client // interface
}

func NewPublisher(client driver.Client) *Publisher {
	return &Publisher{client: client}
}

// PublishStates polls a device that has just come online (or was just subscribed): the
// profile (template, firmware; the STATUS10 reply detects the sensor type), power and sensors,
// plus restart reason and uptime for the "back online" notification.
func (p *Publisher) PublishStates(device *model.Device) {
	p.GetTemplate(device)
	p.GetFirmware(device)
	p.GetRestartReason(device)
	p.GetStatus(device)
	if device.SensorType == model.SensorTypeIR {
		// no power; poll the chip temperature and TelePeriod, which sets the telemetry timeout
		p.GetTelePeriod(device)
		p.GetSensors(device)
		return
	}

	p.GetOnOff(device)
	p.GetSensors(device)
	p.GetTelePeriod(device) // sets the automatic offline timeout
}

func (p *Publisher) GetOnOff(device *model.Device) {
	p.publish(device, "POWER", "")
}

func (p *Publisher) OnOff(device *model.Device, state bool) {
	var value string
	if state {
		value = "ON"
	} else {
		value = "OFF"
	}

	p.publish(device, "POWER", value)
}

func (p *Publisher) GetSensors(device *model.Device) {
	p.publish(device, "STATUS10", "10")
}

// GetStatus returns the same payload as tele/STATE (uptime, load average, ...), wrapped in StatusSTS.
func (p *Publisher) GetStatus(device *model.Device) {
	p.publish(device, "STATUS11", "11")
}

// GetRestartReason requests STATUS1 (StatusPRM.RestartReason).
func (p *Publisher) GetRestartReason(device *model.Device) {
	p.publish(device, "STATUS1", "1")
}

func (p *Publisher) GetFirmware(device *model.Device) {
	p.publish(device, "STATUS2", "2")
}

// Upgrade makes the device download firmware from its OtaUrl and restart.
func (p *Publisher) Upgrade(device *model.Device) {
	p.publish(device, "Upgrade", "1")
}

func (p *Publisher) GetTimers(device *model.Device) {
	for i := 1; i <= 16; i++ {
		p.GetTimer(device, i)
	}
}

func (p *Publisher) GetRules(device *model.Device) {
	for i := 1; i <= 3; i++ {
		p.GetRule(device, i)
	}
}

func (p *Publisher) GetRule(device *model.Device, n int) {
	p.publish(device, fmt.Sprintf("Rule%d", n), "")
}

func (p *Publisher) SetRule(device *model.Device, n int, value string) {
	p.publish(device, fmt.Sprintf("Rule%d", n), value)
}

func (p *Publisher) GetTimer(device *model.Device, n int) {
	p.publish(device, fmt.Sprintf("Timer%d", n), "")
}

func (p *Publisher) SetTimer(device *model.Device, n int, value string) {
	p.publish(device, fmt.Sprintf("Timer%d", n), value)
}

func (p *Publisher) SetVoltage(device *model.Device, voltage int) {
	value := fmt.Sprintf("%d", voltage)
	p.publish(device, "VoltageSet", value)
}

func (p *Publisher) SetPower(device *model.Device, volts uint, power int) {
	value := fmt.Sprintf("%d, %d", power, volts)
	p.publish(device, "PowerSet", value)
}

func (p *Publisher) GetTimezone(device *model.Device) {
	p.publish(device, "Timezone", "")
}

func (p *Publisher) SetTimezone(device *model.Device, offset string) {
	p.publish(device, "Timezone", offset)
}

func (p *Publisher) GetTimeStd(device *model.Device) {
	p.publish(device, "TimeStd", "")
}

func (p *Publisher) SetTimeStd(device *model.Device, value string) {
	p.publish(device, "TimeStd", value)
}

func (p *Publisher) GetTimeDst(device *model.Device) {
	p.publish(device, "TimeDst", "")
}

func (p *Publisher) SetTimeDst(device *model.Device, value string) {
	p.publish(device, "TimeDst", value)
}

func (p *Publisher) GetLedPower(device *model.Device) {
	p.publish(device, "LedPower", "")
}

func (p *Publisher) SetLedPower(device *model.Device, state bool) {
	value := "OFF"
	if state {
		value = "ON"
	}
	p.publish(device, "LedPower", value)
}

func (p *Publisher) GetLedState(device *model.Device) {
	p.publish(device, "LedState", "")
}

func (p *Publisher) SetLedState(device *model.Device, value int) {
	p.publish(device, "LedState", fmt.Sprintf("%d", value))
}

// GetTemplate asks for the GPIO template: relays, LEDs / lights, IR (stored in the device profile).
func (p *Publisher) GetTemplate(device *model.Device) {
	p.publish(device, "Template", "")
}

// GetLight asks for the light state (power, dimmer, HSB colour) in one reply.
func (p *Publisher) GetLight(device *model.Device, rgb bool) {
	if rgb {
		p.publish(device, "HSBColor", "")
	} else {
		p.publish(device, "Dimmer", "")
	}
}

// SetLightPower switches the light, which is POWER<index> (after the relays).
func (p *Publisher) SetLightPower(device *model.Device, index int, state bool) {
	value := "OFF"
	if state {
		value = "ON"
	}
	p.publish(device, fmt.Sprintf("POWER%d", index), value)
}

func (p *Publisher) SetDimmer(device *model.Device, value int) {
	p.publish(device, "Dimmer", fmt.Sprintf("%d", value))
}

// SetHueSat changes the colour, keeping the brightness (dimmer).
func (p *Publisher) SetHueSat(device *model.Device, hue, sat uint) {
	p.publish(device, "Backlog", fmt.Sprintf("HSBColor1 %d; HSBColor2 %d", hue, sat))
}

func (p *Publisher) GetTelePeriod(device *model.Device) {
	p.publish(device, "TelePeriod", "")
}

func (p *Publisher) SetTelePeriod(device *model.Device, value int) {
	formatted := fmt.Sprintf("%d", value)
	p.publish(device, "TelePeriod", formatted)
}

func (p *Publisher) GetLedPwmMode(device *model.Device) {
	p.publish(device, "LedPwmMode", "")
}

func (p *Publisher) SetLedPwmMode(device *model.Device, state bool) {
	var value string
	if state {
		value = "ON"
	} else {
		value = "OFF"
	}
	p.publish(device, "LedPwmMode", value)
}

func (p *Publisher) GetLedPwmOff(device *model.Device) {
	p.publish(device, "LedPwmOff", "")
}

func (p *Publisher) SetLedPwmOff(device *model.Device, value int) {
	formatted := fmt.Sprintf("%d", value)
	p.publish(device, "LedPwmOff", formatted)
}

func (p *Publisher) GetLedPwmOn(device *model.Device) {
	p.publish(device, "LedPwmOn", "")
}

func (p *Publisher) SetLedPwmOn(device *model.Device, value int) {
	formatted := fmt.Sprintf("%d", value)
	p.publish(device, "LedPwmOn", formatted)
}

// publish sends a Tasmota command. Other platforms (ESPHome) are configured in their YAML
// and never get one.
func (p *Publisher) publish(device *model.Device, command string, value string) {
	if !device.IsTasmota() {
		slog.Debug("Not a Tasmota device, command skipped", "topic", device.Topic, "command", command)
		return
	}
	topic := fmt.Sprintf("cmnd/%s/%s", device.Topic, command)
	token := p.client.Publish(topic, 1, false, value)
	token.Wait()

	slog.Debug("Send to topic", "topic", topic, "value", value)
}

// SendIR transmits an infra-red code via Tasmota: cmnd/<topic>/IRsend {"Protocol":..,"Bits":..,"Data":"0x.."}
func (p *Publisher) SendIR(device *model.Device, protocol string, bits int, code string) {
	payload := fmt.Sprintf(`{"Protocol":%q,"Bits":%d,"Data":%q}`, protocol, bits, code)
	p.publish(device, "IRsend", payload)
}
