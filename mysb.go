package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/eclipse/paho.mqtt.golang"
)

// mysb - MQTT all the things!
type mysb struct {
	ClientID         string          `env:"MQTT_CLIENTID" envDefault:"DefaultMysbClientID"`
	Broker           string          `env:"MQTT_BROKER" envDefault:"tcp://mosquitto.org:1883"`
	SubTopic         string          `env:"MQTT_SUBTOPIC" envDefault:"mysensors_rx"`
	PubTopic         string          `env:"MQTT_PUBTOPIC" envDefault:"mysensors_tx"`
	Username         string          `env:"MQTT_USERNAME"`
	Password         string          `env:"MQTT_PASSWORD"`
	AutoIDEnabled    bool            `env:"MYSB_AUTOID" envDefault:"false"`
	NextID           uint            `env:"MYSB_NEXTID" envDefault:"1"`
	FirmwareBasePath string          `env:"MYSB_FIRMWAREBASEPATH" envDefault:"/config/firmware"`
	Nodes            nodeSettingsMap `env:"MYSB_NODES"`

	bootloaderCommands bootloaderCmdMap
	lastPublished      string
}

func (t *mysb) start() error {
	log.Println("Connecting to MQTT")
	opts := mqtt.NewClientOptions().
		AddBroker(t.Broker).
		SetClientID(t.ClientID).
		SetOnConnectHandler(t.onConnect).
		SetConnectionLostHandler(func(client mqtt.Client, err error) {
			log.Printf("Disconnected from MQTT: %s.", err)
		}).
		SetUsername(t.Username).
		SetPassword(t.Password)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); !token.Wait() || token.Error() != nil {
		return token.Error()
	}

	return nil
}

func (t *mysb) onConnect(client mqtt.Client) {
	log.Println("Connected to MQTT")

	// Subscribe to topics
	subscriptions := map[string]mqtt.MessageHandler{
		fmt.Sprintf(idRequestTopic, t.SubTopic):             t.idRequest,
		fmt.Sprintf(firmwareConfigRequestTopic, t.SubTopic): t.configurationRequest,
		fmt.Sprintf(firmwareRequestTopic, t.SubTopic):       t.dataRequest,
		firmwareBootloaderCommandTopic:                      t.bootloaderCommand,
	}

	//
	if !client.IsConnected() {
		log.Print("Subscribe Error: Not Connected (Reloading Config?)")
		return
	}

	for topic, handler := range subscriptions {
		log.Printf("Subscribing: %s", topic)
		if token := client.Subscribe(topic, 0, handler); !token.Wait() || token.Error() != nil {
			log.Printf("Subscribe Error: %s", token.Error())
		}
	}
}

func (t *mysb) idRequest(client mqtt.Client, msg mqtt.Message) {
	log.Println("ID Request")
	if !t.AutoIDEnabled {
		return
	}

	t.NextID++

	log.Printf("Assigning ID: %d\n", t.NextID)
	t.publish(client, fmt.Sprintf(idResponseTopic, t.PubTopic), fmt.Sprintf("%d", t.NextID))
}

func (t *mysb) configurationRequest(client mqtt.Client, msg mqtt.Message) {
	_, payload, to := t.msgParts(msg)

	// Attempt to run any bootloader commands
	if t.runBootloaderCommand(client, to) {
		return
	}

	req := newFirmwareConfiguration(payload)
	fw := t.firmwareInfo(to, req.Type, req.Version)
	firmware := newFirmware(fw.Path)
	resp := firmwareConfiguration{
		Type:    fw.Type,
		Version: fw.Version,
		Blocks:  firmware.Blocks,
		Crc:     firmware.Crc,
	}

	respTopic := fmt.Sprintf(firmwareConfigResponseTopic, t.PubTopic, to)
	respPayload := resp.String()

	log.Printf("Configuration Request: From: %s; Request: %s; Response: %s\n", to, req.String(), respPayload)
	t.publish(client, respTopic, respPayload)
}

func (t *mysb) dataRequest(client mqtt.Client, msg mqtt.Message) {
	_, payload, to := t.msgParts(msg)

	req := newFirmwareRequest(payload)
	fw := t.firmwareInfo(to, req.Type, req.Version)
	firmware := newFirmware(fw.Path)
	resp := firmwareRequest{
		Type:    fw.Type,
		Version: fw.Version,
		Block:   req.Block,
	}

	data, _ := firmware.dataForBlock(req.Block)
	respTopic := fmt.Sprintf(firmwareResponseTopic, t.PubTopic, to)
	respPayload := resp.String(data)

	if req.Block+1 == firmware.Blocks {
		log.Printf("Data Request: From: %s; Payload: %s\n", to, payload)
		log.Printf("Sending last block of %d to %s\n", firmware.Blocks, to)
	} else if req.Block == 0 {
		log.Printf("Sending first block of %d to %s\n", firmware.Blocks, to)
	} else if req.Block%50 == 0 {
		log.Printf("Sending block %d of %d to %s\n", req.Block, firmware.Blocks, to)
	}
	t.publish(client, respTopic, respPayload)
}

// Bootloader commands:
// * 0x01 - Erase EEPROM
// * 0x02 - Set NodeID
// * 0x03 - Set ParentID
func (t *mysb) bootloaderCommand(client mqtt.Client, msg mqtt.Message) {
	topic, payload, _ := t.msgParts(msg)

	parts := strings.Split(topic, "/")
	to := parts[2]
	cmd := parts[3]

	blCmd, _ := strconv.ParseUint(cmd, 10, 16)
	resp := firmwareConfiguration{
		Type:    uint16(blCmd),
		Version: 0,
		Blocks:  0,
		Crc:     0xDA7A,
	}

	if resp.Type == 0x02 || resp.Type == 0x03 {
		blVersion, _ := strconv.ParseUint(payload, 10, 16)
		resp.Version = uint16(blVersion)
	}

	log.Printf("Bootloader Command: To: %s; Cmd: %s; Payload: %s\n", to, cmd, payload)
	if t.bootloaderCommands == nil {
		t.bootloaderCommands = make(bootloaderCmdMap)
	}
	t.bootloaderCommands[to] = resp
}

func (t *mysb) runBootloaderCommand(client mqtt.Client, to string) bool {
	if blcmd, ok := t.bootloaderCommands[to]; ok {
		outTopic := fmt.Sprintf(firmwareConfigResponseTopic, t.PubTopic, to)
		outPayload := blcmd.String()
		t.publish(client, outTopic, outPayload)

		delete(t.bootloaderCommands, to)
		return true
	}

	return false
}

func (t *mysb) firmwareInfo(nodeID string, firmwareType uint16, firmwareVersion uint16) firmwareInformation {
	fw := firmwareInformation{
		Source: fwUnknown,
	}

	// Attempt to load firmware from the assignment in config.yaml
	fw = t.firmwareInfoAssignment(nodeID, fwNode)

	// Attempt to load firmware based on the node's request
	if _, err := os.Stat(fw.Path); err != nil {
		fw.Type, fw.Version, fw.Source = firmwareType, firmwareVersion, fwReq
		fw.Path = fmt.Sprintf("%s/%d/%d/firmware.hex", t.FirmwareBasePath, fw.Type, fw.Version)
	}

	// Attempt to laod the default firmware
	if _, err := os.Stat(fw.Path); err != nil {
		fw = t.firmwareInfoAssignment("default", fwDefault)
	}

	// Awww, nothing worked...
	if _, err := os.Stat(fw.Path); err != nil {
		fw.Type, fw.Version, fw.Path, fw.Source = 0, 0, "", fwUnknown
	}

	return fw
}

func (t *mysb) firmwareInfoAssignment(nodeID string, source firmwareSource) firmwareInformation {
	fw := firmwareInformation{
		Source: fwUnknown,
	}

	// Attempt to load firmware from the assignment in config.yaml
	nodeSettings := t.Nodes[nodeID]
	fw.Type = nodeSettings.Type
	fw.Version = nodeSettings.Version
	fw.Path = fmt.Sprintf("%s/%d/%d/firmware.hex", t.FirmwareBasePath, fw.Type, fw.Version)
	fw.Source = source

	return fw
}

func (t *mysb) msgParts(msg mqtt.Message) (string, string, string) {
	topic := msg.Topic()
	payload := string(msg.Payload())
	to := strings.Split(topic, "/")[1]

	return topic, payload, to
}

func (t *mysb) publish(client mqtt.Client, topic string, payload string) {
	if token := client.Publish(topic, 0, false, payload); token.Wait() && token.Error() != nil {
		log.Printf("Publish Error: %s", token.Error())
	}
	t.lastPublished = fmt.Sprintf("%s %s", topic, payload)
}
