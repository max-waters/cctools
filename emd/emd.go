package emd

import (
	"fmt"
	"log"
	"time"

	"github.com/pkg/errors"
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/sysex"
	"gitlab.com/gomidi/midi/reader"
	"mvw.org/cctools/util"
)

const ConnectionMaxWaitTime = time.Second * 5
const KitSysexLength = 1231

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
func (conn *EmdConnection) GetKit(kitNum uint8) ([]byte, error) {
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

func GetSysexKit(conf *EmdConnectionConfig, kit uint8, filename string) error {
	conn, err := NewEmdConnection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	patch, err := conn.GetKit(kit)
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
