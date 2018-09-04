package main

import (
	"fmt"
	"testing"

	"github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v2"
)

const nodeRequestHex = "test_files/1/1/firmware.hex"

var testClient = mqtt.NewClient(mqtt.NewClientOptions())

func defaultTestMQTT() *MysbMQTT {
	var testConfig = `
        settings:
          clientid: 'GoMySysBootloader'
          broker: "tcp://fake.mosquitto.org:1883"
          subtopic: 'mysensors_rx'
          pubtopic: 'mysensors_tx'

        control:
            nextid: 12
            firmwarebasepath: 'test_files'
            nodes:
                default: { type: 1, version: 1 }
                1: { type: 1, version: 1, queueMessages: true }
    `

	myMqtt := MysbMQTT{}
	err := yaml.Unmarshal([]byte(testConfig), &myMqtt)
	if err != nil {
		panic(err)
	}
	return &myMqtt
}

func TestMqttIDRequest(t *testing.T) {
	myMQTT := defaultTestMQTT()
	var tests = []struct {
		Request       string
		Response      string
		AutoIDEnabled bool
	}{
		{fmt.Sprintf("%s/255/255/3/0/3", myMQTT.Settings.SubTopic), fmt.Sprintf("%s/255/255/3/0/4 %s", myMQTT.Settings.PubTopic, "13"), true},
		{fmt.Sprintf("%s/255/255/3/0/3", myMQTT.Settings.SubTopic), "", false},
	}
	for _, v := range tests {
		msg := &mockMessage{
			topic:   v.Request,
			payload: []byte(""),
		}

		expected := v.Response

		myMQTT.LastPublished = ""
		myMQTT.Control.AutoIDEnabled = v.AutoIDEnabled
		myMQTT.idRequest(testClient, msg)
		if myMQTT.LastPublished != expected {
			t.Errorf("Wrong topic or payload - Actual: %s, Expected: %s", myMQTT.LastPublished, expected)
		}
	}
}

func TestMqttConfigurationRequest(t *testing.T) {
	myMQTT := defaultTestMQTT()
	msg := &mockMessage{
		topic:   fmt.Sprintf("%s/1/255/4/0/0", myMQTT.Settings.SubTopic),
		payload: []byte("010001005000D446"),
	}

	expected := fmt.Sprintf("%s/1/255/4/0/1 %s", myMQTT.Settings.PubTopic, "010001005000D446")
	myMQTT.configurationRequest(testClient, msg)
	if myMQTT.LastPublished != expected {
		t.Errorf("Wrong topic or payload - Actual: %s, Expected: %s", myMQTT.LastPublished, expected)
	}
}

func TestMqttDataRequest(t *testing.T) {
	myMQTT := defaultTestMQTT()
	msg := &mockMessage{
		topic:   fmt.Sprintf("%s/1/255/4/0/0", myMQTT.Settings.SubTopic),
		payload: []byte("010001000100"),
	}

	expected := fmt.Sprintf("%s/1/255/4/0/3 %s", myMQTT.Settings.PubTopic, "0100010001000C946E000C946E000C946E000C946E00")

	myMQTT.dataRequest(testClient, msg)
	if myMQTT.LastPublished != expected {
		t.Errorf("Wrong topic or payload - Actual: %s, Expected: %s", myMQTT.LastPublished, expected)
	}
}

func TestMqttBootloaderCommand(t *testing.T) {
	myMQTT := defaultTestMQTT()
	var tests = []struct {
		To              string
		Cmd             string
		Payload         string
		ExpectedPayload string
	}{
		{"1", "1", "", "0100000000007ADA"},
		{"2", "2", "9", "0200090000007ADA"},
	}

	for _, v := range tests {
		msg := &mockMessage{
			topic:   fmt.Sprintf("mysensors/bootloader/%s/%s", v.To, v.Cmd),
			payload: []byte(v.Payload),
		}

		myMQTT.bootloaderCommand(testClient, msg)
		if _, ok := myMQTT.Control.BootloaderCommands[v.To]; !ok {
			t.Error("Bootloader command not found")
		} else {
			if ok := myMQTT.runBootloaderCommand(testClient, v.To); !ok {
				t.Error("Bootloader command not run")
			} else {
				expected := fmt.Sprintf("%s/%s/255/4/0/1 %s", myMQTT.Settings.PubTopic, v.To, v.ExpectedPayload)
				if myMQTT.LastPublished != expected {
					t.Errorf("Wrong topic or payload - Actual: %s, Expected: %s", myMQTT.LastPublished, expected)
				}
			}
		}
	}
}

func TestMqttBadBootloaderCommand(t *testing.T) {
	myMQTT := defaultTestMQTT()
	if ok := myMQTT.runBootloaderCommand(testClient, "1"); ok {
		t.Error("Bootloader command didn't exist, should not have returned true")
	}
}

func TestMqttStart(t *testing.T) {
	myMQTT := defaultTestMQTT()
	if err := myMQTT.Start(); err == nil {
		t.Error("Something went wrong; expected a failure to connect!")
	}

	myMQTT.Stop()
}

func TestMqttConnect(t *testing.T) {
	myMQTT := defaultTestMQTT()
	myMQTT.onConnect(testClient)
}

type mockMessage struct {
	topic   string
	payload []byte
}

func (m *mockMessage) Duplicate() bool {
	return true
}

func (m *mockMessage) Qos() byte {
	return 'a'
}

func (m *mockMessage) Retained() bool {
	return true
}

func (m *mockMessage) Topic() string {
	return m.topic
}

func (m *mockMessage) MessageID() uint16 {
	return 0
}

func (m *mockMessage) Payload() []byte {
	return m.payload
}
