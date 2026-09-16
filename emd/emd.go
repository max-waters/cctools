package emd

import (
	"fmt"
	"log"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/max-waters/cctools/util"
	"github.com/pkg/errors"
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/sysex"
	"gitlab.com/gomidi/midi/reader"
)

const ConnectionMaxWaitTime = time.Second * 5
const KitSysexLength = 1231

var ccNames = [24]string{
	"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8",
	"amd", "amf", "eqf", "eqg", "fltf", "fltw", "fltq", "srr",
	"dist", "vol", "pan", "del", "rev", "lfos", "lfod", "lfom",
	// "track", "param", "shp1", "shp2", "updte", "speed", "depth", "shmix",
}

var ccNameIdx = map[string]int{
	"p1": 0, "p2": 1, "p3": 2, "p4": 3, "p5": 4, "p6": 5, "p7": 6, "p8": 7,
	"amd": 8, "amf": 9, "eqf": 10, "eqg": 11, "fltf": 12, "fltw": 13, "fltq": 14, "srr": 15,
	"dist": 16, "vol": 17, "pan": 18, "del": 19, "rev": 20, "lfos": 21, "lfod": 22, "lfom": 23,
}

var ccPageOffset = map[string]int{"syn": 0, "eff": 8, "rtg": 16}

const ccOffset = 25

const machineOffset = 429

var machineIdx = []int{
	0, 5, 9, 14, 18, 23, 27, 32,
	37, 41, 46, 50, 55, 59, 64, 69,
}

// GND: 0 -- 5
// TRX: 16 -- 29
// EFM: 32 -- 39
// E12: 48 -- 63
// P-I: 64 -- 72
// INP: 80 -- 87
// MID: 96 -- 111
// CTR: 112 -- 117
// ROM:
// RAM:
// NFX: 7 -- 9

var machineValues = []byte{0, 1, 2, 3, 4, 5, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 32, 33, 34, 35, 36, 37, 38, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 70, 71, 72}

//var synthOffsets = []int{0, 16, 32, 48, 64}
//var synthCounts = []int{6, 14, 7, 16, 9}

type EmdConnection struct {
	Config       *EmdConnectionConfig
	readerWriter *util.MidiReaderWriter
	responseChan chan midi.Message
	shutdownChan chan any
}

type EmdConnectionConfig struct {
	InPort  uint `yaml:"in_port"`
	OutPort uint `yaml:"out_port"`
}

func NewEmdConnection(conf *EmdConnectionConfig) (*EmdConnection, error) {
	conn := &EmdConnection{
		Config:       conf,
		responseChan: make(chan midi.Message, 1024),
		shutdownChan: make(chan any, 1),
	}

	rw, err := util.NewMidiReaderWriter(conf.InPort, conf.OutPort, func(pos *reader.Position, msg midi.Message) {
		conn.responseChan <- msg
	})
	if err != nil {
		return nil, err
	}
	conn.readerWriter = rw

	conn.readerWriter.LogPorts()

	return conn, nil
}

// kits are numbered 0--63, or 64 for the edit buffer
func (conn *EmdConnection) GetKitSysex(kitNum uint8) ([]byte, error) {
	// [ 240 0 32 60 2 0 83 <kit number> 247 ]
	// NB first and last byte not required
	prSysEx := []byte{0, 32, 60, 2, 0, 83, kitNum}
	if err := conn.readerWriter.SysEx(0, prSysEx); err != nil {
		return nil, errors.Wrap(err, "error sending kit request")
	}

	patchSysex := []byte{}
	lastByte := 0
	for lastByte != 247 {
		msg, err := util.WaitForMsg[sysex.Message](conn.responseChan, conn.shutdownChan, ConnectionMaxWaitTime)
		if err != nil {
			return nil, err
		}
		patchSysex = append(patchSysex, msg.Data()...)
		lastByte = int(msg.Raw()[len(msg.Raw())-1])
	}

	// check length
	if len(patchSysex) != KitSysexLength {
		return nil, errors.Errorf("unexpected sysex kit dump length: %d, expected %d", len(patchSysex), KitSysexLength)
	}

	// check header. NB first and last byte are already removed, and [ 64 1 ] is the firmware version
	// [ 0 32 60 2 0 82 64 1 <kit number > ... ]
	expKit := kitNum
	if kitNum > 63 {
		expKit = 0
	}
	expHeader := []byte{0, 32, 60, 2, 0, 82, 64, 1, expKit}
	for i := range len(expHeader) {
		if patchSysex[i] != expHeader[i] {
			return nil, errors.Errorf("sysex header %s does not match expected %s",
				util.FmtSysEx(patchSysex[:len(expHeader)]),
				util.FmtSysEx(expHeader))
		}
	}

	// check checksum
	csMsb, csLsb := ComputeKitChecksum(patchSysex)
	if patchSysex[len(patchSysex)-4] != csMsb || patchSysex[len(patchSysex)-3] != csLsb {
		return nil, errors.Errorf("sysex checksum %s does not match expected %s",
			util.FmtSysEx(patchSysex[len(patchSysex)-4:len(patchSysex)-3]),
			util.FmtSysEx([]byte{csMsb, csLsb}))
	}

	return patchSysex, nil
}

func (conn *EmdConnection) GetKitCcs(kitNum uint8) (string, error) {
	patchSysex, err := conn.GetKitSysex(kitNum)
	if err != nil {
		return "", err
	}

	sb := strings.Builder{}
	sb.WriteString("voice, controller, value\n")
	for i := range 16 * len(ccNames) {
		fmt.Fprintf(&sb, "%d, %s, %d\n", (i/len(ccNames))+1, ccNames[i%len(ccNames)], patchSysex[i+ccOffset])
	}

	return sb.String(), nil
}

func (conn *EmdConnection) SendKit(kitSysEx []byte, kitNum uint8) error {
	if len(kitSysEx) != KitSysexLength {
		return fmt.Errorf("unexpected sysex kit length %d, expected %d", len(kitSysEx), KitSysexLength)
	}

	// override kit position
	// [ 0 32 60 2 0 82 64 1 <dest kit number> ... ]
	kitSysEx[8] = kitNum

	// re-compute checksum
	kitSysEx[len(kitSysEx)-4], kitSysEx[len(kitSysEx)-3] = ComputeKitChecksum(kitSysEx)

	if err := conn.readerWriter.SysEx(0, kitSysEx); err != nil {
		return errors.Wrap(err, "error sending kit")
	}

	return nil
}

// input: complete kit sysex without 240 prefix or 247 suffix
func ComputeKitChecksum(kitSysex []byte) (msb byte, lsb byte) {
	checksum := uint16(0)
	for j := 8; j < len(kitSysex)-4; j++ {
		checksum += uint16(kitSysex[j])
	}
	return byte((checksum >> 7) & 127), byte(checksum & 127)
}

func (conn *EmdConnection) Close() {
	conn.shutdownChan <- nil
}

func RandomizeVoice(conf *EmdConnectionConfig, kit, voice uint8, machine bool, params []string) error {
	conn, err := NewEmdConnection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	selected := [24]bool{}
	for _, param := range params {
		if idx, ok := ccNameIdx[param]; ok {
			selected[idx] = true
			continue
		}
		if offset, ok := ccPageOffset[param]; ok {
			for i := range 8 {
				selected[offset+i] = true
			}
			continue
		}
		return errors.Errorf("unknown parameter: %s", param)
	}

	kitSysEx, err := conn.GetKitSysex(kit)
	if err != nil {
		return err
	}

	offset := (int(voice) * len(ccNames)) + ccOffset
	for i, sel := range selected {
		if sel {
			kitSysEx[i+offset] = byte(rand.IntN(128))
		}
	}

	if machine {
		val := rand.IntN(len(machineValues)-2) + 2 // range 2 -- 51 inclusive
		idx := machineOffset + machineIdx[voice]
		kitSysEx[idx] = machineValues[val]
	}

	if err := conn.SendKit(kitSysEx, kit); err != nil {
		return errors.Wrap(err, "error sending patch")
	}

	return nil
}

func GetSysexKit(conf *EmdConnectionConfig, kit uint8, filename string) error {
	conn, err := NewEmdConnection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	patch, err := conn.GetKitSysex(kit)
	if err != nil {
		return err
	}

	filename, err = util.SaveSysex(filename, patch)
	if err != nil {
		return err
	}

	kitStr := "edit buffer"
	if kit != 65 {
		kitStr = fmt.Sprintf("kit %d", kit)
	}
	log.Printf("Saved Machinedrum %s to %s\n", kitStr, filename)
	return nil
}

func GetCcValues(conf *EmdConnectionConfig, kit uint8, filename string) error {
	conn, err := NewEmdConnection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	ccs, err := conn.GetKitCcs(kit)
	if err != nil {
		return err
	}

	filename, err = util.SaveSysex(filename, []byte(ccs))
	if err != nil {
		return err
	}

	kitStr := "edit buffer"
	if kit != 65 {
		kitStr = fmt.Sprintf("kit %d", kit)
	}
	log.Printf("Saved Machinedrum %s controller values to %s\n", kitStr, filename)
	return nil
}

func SetSysexKit(conf *EmdConnectionConfig, kit uint8, filename string) error {
	sysEx, err := util.LoadSysEx(filename)
	if err != nil {
		return errors.Wrapf(err, "error reading file '%s'", filename)
	}

	conn, err := NewEmdConnection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.SendKit(sysEx, kit); err != nil {
		return errors.Wrap(err, "error sending patch")
	}

	kitStr := "edit buffer"
	if kit != 65 {
		kitStr = fmt.Sprintf("kit %d", kit)
	}
	log.Printf("Sent kit to Machinedrum %s\n", kitStr)
	return nil
}
