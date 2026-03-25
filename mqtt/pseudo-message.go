package mqtt

import driver "github.com/eclipse/paho.mqtt.golang"

type pseudoMessage struct {
	topic   string
	payload []byte
}

func messageFromResult(msg driver.Message, newPayload []byte) *pseudoMessage {
	return &pseudoMessage{
		topic:   msg.Topic(),
		payload: newPayload,
	}
}

func (m *pseudoMessage) Duplicate() bool {
	return false
}

func (m *pseudoMessage) Qos() byte {
	return 0
}

func (m *pseudoMessage) Retained() bool {
	return false
}

func (m *pseudoMessage) Topic() string {
	return m.topic
}

func (m *pseudoMessage) MessageID() uint16 {
	return 0
}

func (m *pseudoMessage) Payload() []byte {
	return m.payload
}

func (m *pseudoMessage) Ack() {
}
