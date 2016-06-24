package transport

import (
	"github.com/mannkind/mysb/ota"
	"fmt"
	"log"
	"os"
	"strconv"
)

// Control - Control the interaction of Transport and OTA
type Control struct {
	NextID           uint8
	FirmwareBasePath string
	Nodes            map[string]map[string]string
	Commands         map[string]ota.Configuration
}

// FWType - The type of firmware used
type FWType int

// FWType - The type of firmware used
const (
	FWUnknown FWType = iota
	FWNode
	FWReq
	FWDefault
)

// FirmwareInfo - Get the firmware to use given a nodeid, or firmware type/version
func (c Control) FirmwareInfo(nodeID string, firmwareType string, firmwareVersion string) (string, string, string, FWType) {
	outType, outVer, outPath, outFW := "0", "0", "", FWUnknown
	nodeMapping := c.Nodes[nodeID]
	if nodeMapping != nil {
		outType, _ = nodeMapping["type"]
		outVer, _ = nodeMapping["version"]
		outPath = fmt.Sprintf("%s/%s/%s/firmware.hex", c.FirmwareBasePath, outType, outVer)
		outFW = FWNode
	}

	if _, err := os.Stat(outPath); err != nil {
		outType, outVer, outFW = firmwareType, firmwareVersion, FWReq
		outPath = fmt.Sprintf("%s/%s/%s/firmware.hex", c.FirmwareBasePath, outType, outVer)
	}

	if _, err := os.Stat(outPath); err != nil {
		outType, outVer, outFW = "0", "0", FWUnknown
		defaultMapping := c.Nodes["default"]
		if defaultMapping != nil {
			outType, _ = defaultMapping["type"]
			outVer, _ = defaultMapping["version"]
			outFW = FWDefault
		}
		outPath = fmt.Sprintf("%s/%s/%s/firmware.hex", c.FirmwareBasePath, outType, outVer)
	}

	if _, err := os.Stat(outPath); err != nil {
		outPath = ""
	}

	return outType, outVer, outPath, outFW
}

// IDRequest - Handle incoming ID requests
func (c *Control) IDRequest() string {
	log.Println("ID Request")
	c.NextID++
	return fmt.Sprintf("%d", c.NextID)
}

// ConfigurationRequest - Handle incoming firmware configuration requets
func (c *Control) ConfigurationRequest(to string, payload string) string {
	req := ota.Configuration{}
	req.Load(payload)

	typ, ver, filename, _ := c.FirmwareInfo(to, fmt.Sprintf("%d", req.Type), fmt.Sprintf("%d", req.Version))
	firmware := ota.Firmware{}
	firmware.Load(filename)

	ftype, _ := c.parseUint16(typ)
	fver, _ := c.parseUint16(ver)
	resp := ota.Configuration{
		Type:    ftype,
		Version: fver,
		Blocks:  firmware.Blocks,
		Crc:     firmware.Crc,
	}

	log.Printf("Request: %s; Response: %s\n", req.String(), resp.String())
	return resp.String()
}

// DataRequest - Handle incoming firmware requests
func (c *Control) DataRequest(to string, payload string) string {
	req := ota.Data{}
	req.Load(payload)

	ftype, fver, fname, _ := c.FirmwareInfo(
		to,
		fmt.Sprintf("%d", req.Type),
		fmt.Sprintf("%d", req.Version),
	)

	firmware := ota.Firmware{}
	firmware.Load(fname)

	firmwareType, _ := c.parseUint16(ftype)
	firmwareVer, _ := c.parseUint16(fver)
	resp := ota.Data{
		Type:    firmwareType,
		Version: firmwareVer,
		Block:   req.Block,
	}

	if req.Block+1 == firmware.Blocks {
		log.Printf("Data Request: From: %s, Payload: %s\n", to, payload)
		log.Printf("Sending last block of %d\n", firmware.Blocks)
	} else if req.Block == 0 {
		log.Printf("Sending first block of %d\n", firmware.Blocks)
	} else if req.Block%50 == 0 {
		log.Printf("Sending block %d of %d\n", req.Block, firmware.Blocks)
	}

	return resp.String(firmware.Data(req.Block))
}

// BootloaderCommand - Handle bootloader commands
func (c *Control) BootloaderCommand(to string, cmd string, payload string) {
	command, _ := c.parseUint16(cmd)
	pl, _ := c.parseUint16(payload)

	resp := ota.Configuration{
		Type:    command,
		Version: 0,
		Blocks:  0,
		Crc:     0xDA7A,
	}

	/*
	 Bootloader commands
	 0x01 - Erase EEPROM
	 0x02 - Set NodeID
	 0x03 - Set ParentID
	*/
	if resp.Type == 0x02 || resp.Type == 0x03 {
		resp.Version = pl
	}

	log.Printf("Bootloader Command: To: %s Cmd: %s Payload: %s\n", to, cmd, payload)
	c.Commands[to] = resp
}

func (c Control) parseUint16(input string) (uint16, error) {
	val, err := strconv.ParseUint(input, 16, 16)
	if err != nil {
		return 0, err
	}

	return uint16(val), nil
}
