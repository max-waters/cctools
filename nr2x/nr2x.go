package nr2x

import (
	"fmt"
	"log"
	"time"

	"github.com/pkg/errors"
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/channel"
	"gitlab.com/gomidi/midi/midimessage/sysex"
	"gitlab.com/gomidi/midi/reader"
	"mvw.org/cctools/util"
)

const ConnectionMaxWaitTime = time.Second * 5
const NumNr2xControllers = 42

type Nr2xConnection struct {
	Config       *Nr2xConnectionConfig
	readerWriter *util.MidiReaderWriter
	responseChan chan midi.Message
	shutdownChan chan interface{}
}

type Nr2xConnectionConfig struct {
	InPort          uint             `yaml:"in_port"`
	OutPort         uint             `yaml:"out_port"`
	GlobalMidiChan  uint8            `yaml:"global_midi_channel"`
	Voice           string           `yaml:"voice"`
	voiceMidiChan   uint8            `yaml:"-"`
	VoiceChannelMap map[string]uint8 `yaml:"voice_channel_map"`
}

func (conf *Nr2xConnectionConfig) setVoiceMidiChan() error {
	channel, ok := conf.VoiceChannelMap[conf.Voice]
	if !ok {
		return errors.Errorf("no channel configured for voice '%s'", conf.Voice)
	}
	conf.voiceMidiChan = channel
	return nil
}

func NewNr2xConnection(conf *Nr2xConnectionConfig) (c *Nr2xConnection, errVal error) {
	if err := conf.setVoiceMidiChan(); err != nil {
		return nil, err
	}

	conn := &Nr2xConnection{
		Config:       conf,
		responseChan: make(chan midi.Message, 1024),
		shutdownChan: make(chan interface{}, 1),
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

func (conn *Nr2xConnection) SendControlChange(controller, value uint8) error {
	if err := conn.readerWriter.ControlChange(conn.Config.voiceMidiChan, controller, value); err != nil {
		return errors.Wrap(err, "error sending control change message")
	}
	return nil
}

func (conn *Nr2xConnection) SendPercussionEdit(perc uint8) error {
	key := 37 + ((perc / 2) * 12) + ((perc % 2) * 5)
	if err := conn.readerWriter.NoteOn(conn.Config.voiceMidiChan, key, 127); err != nil {
		return errors.Wrapf(err, "error setting note on for key %d", key)
	}
	time.Sleep(50 * time.Millisecond)
	if err := conn.readerWriter.NoteOff(conn.Config.voiceMidiChan, key); err != nil {
		return errors.Wrapf(err, "error setting note on for key %d", key)
	}
	return nil
}

func (conn *Nr2xConnection) SendPatch(patchSysEx []byte, percussion bool) error {
	if percussion && len(patchSysEx) != 1056 {
		return fmt.Errorf("unexpected sysex percussion kit length %d, expected 1056", len(patchSysEx))
	}
	if !percussion && len(patchSysEx) != 132 {
		return fmt.Errorf("unexpected sysex patch length %d, expected 132", len(patchSysEx))
	}

	if isEmptySysEx(patchSysEx) {
		return fmt.Errorf("empty sysex data")
	}

	slotNum, err := getSlotNum(conn.Config.Voice)
	if err != nil {
		return err
	}

	// NB for patches, <slot num> is 0-3, for percussion kits its 16-19
	if percussion {
		slotNum = slotNum + 16
	}

	// [ 240 51 <global midi> 4 <bank> <slot num> ]
	header := []byte{51, conn.Config.GlobalMidiChan, 4, 0, slotNum}
	patchSysEx = append(header, patchSysEx...)

	if err := conn.readerWriter.SysEx(conn.Config.GlobalMidiChan, patchSysEx); err != nil {
		return errors.Wrap(err, "error sending patch")
	}

	return nil
}

func (conn *Nr2xConnection) GetPatch(percussion bool) ([]byte, error) {
	slotNum, err := getSlotNum(conn.Config.Voice)
	if err != nil {
		return nil, err
	}

	// [ 51 <global midi chan> 4 14 <slot num 0-3> ]
	// NB manual says 10, not 14, but this is a mistake?
	prSysEx := []byte{51, conn.Config.GlobalMidiChan, 4, 14, slotNum}
	if err := conn.readerWriter.SysEx(conn.Config.GlobalMidiChan, prSysEx); err != nil {
		return nil, errors.Wrap(err, "error sending patch dump request")
	}

	patchSysex := []byte{}
	lastByte := 0
	for lastByte != 247 {
		msg, err := waitForMsg[sysex.Message](conn)
		if err != nil {
			return nil, err
		}
		patchSysex = append(patchSysex, msg.Data()...)
		lastByte = int(msg.Raw()[len(msg.Raw())-1])
	}

	// check length. normal patch is 5+132, percussion patch is 5+1056
	if percussion && len(patchSysex) != 1061 {
		return nil, errors.Errorf("unexpected sysex percussion kit dump length: %d, expected 1061 -- set active slot on NR2X and retry", len(patchSysex))
	}
	if !percussion && len(patchSysex) != 137 {
		return nil, errors.Errorf("unexpected sysex patch dump length: %d, expected 137 -- set active slot on NR2X and retry", len(patchSysex))
	}

	// check header
	// [ 51 <global midi chan> 4 0 <slot num> ]
	// NB for patches, <slot num> is 0-3, for percussion kits it's 16-19
	expSlotNum := slotNum
	if percussion {
		expSlotNum = expSlotNum + 16
	}

	expHeader := []byte{51, conn.Config.GlobalMidiChan, 4, 0, expSlotNum}
	for i := 0; i < len(expHeader); i++ {
		if patchSysex[i] != expHeader[i] {
			return nil, errors.Errorf("sysex header %s does not match expected %s",
				util.FmtSysEx(patchSysex, len(expHeader)),
				util.FmtSysEx(expHeader, len(expHeader)))
		}
	}

	// strip header, we just save the patch data
	patchSysex = patchSysex[len(expHeader):]
	if isEmptySysEx(patchSysex) {
		return nil, fmt.Errorf("empty sysex data received -- set active slot on NR2X and retry")
	}

	return patchSysex, nil
}

func isEmptySysEx(sysEx []byte) bool {
	for _, b := range sysEx {
		if int(b) != 0 {
			return false
		}
	}
	return true
}

func getSlotNum(vc string) (uint8, error) {
	switch vc {
	case "A":
		return 0, nil
	case "B":
		return 1, nil
	case "C":
		return 2, nil
	case "D":
		return 3, nil
	default:
		return 0, fmt.Errorf("unknown voice '%s'", vc)
	}
}

func (conn *Nr2xConnection) GetControllerValues() ([]*util.ControllerValue, error) {
	// [ 51 <global midi chan> 4 28 <slot num 0-3> ]
	// NB manual says 20, not 28. 20 prompts a sysex msg, 28 prompts midi cc values
	acrSysEx := []byte{51, conn.Config.GlobalMidiChan, 4, 28, conn.Config.voiceMidiChan}
	if err := conn.readerWriter.SysEx(conn.Config.voiceMidiChan, acrSysEx); err != nil {
		return nil, errors.Wrap(err, "error sending all controllers request")
	}

	controllerValues := make([]*util.ControllerValue, NumNr2xControllers)
	for i := 0; i < NumNr2xControllers; i++ {
		cvMsg, err := waitForMsg[channel.ControlChange](conn)
		if err != nil {
			return nil, err
		}
		controllerValues[i] = &util.ControllerValue{
			Controller: cvMsg.Controller(),
			Value:      cvMsg.Value(),
		}
	}
	return controllerValues, nil
}

func waitForMsg[A midi.Message](conn *Nr2xConnection) (A, error) {
	select {
	case <-conn.shutdownChan:
		var zero A
		return zero, errors.New("cancelled")
	case <-time.After(ConnectionMaxWaitTime):
		var zero A
		return zero, errors.New("request timed out")
	case msg := <-conn.responseChan:
		cast, ok := msg.(A)
		if !ok {
			var zero A
			return zero, errors.Errorf("cannot cast %v (%T) as a %T", msg.String(), msg, zero)
		}
		return cast, nil
	}
}

func (conn *Nr2xConnection) Close() {
	conn.shutdownChan <- nil
}

func GetSysexProgram(conf *Nr2xConnectionConfig, perc bool, filename string) error {
	conn, err := NewNr2xConnection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	patch, err := conn.GetPatch(perc)
	if err != nil {
		return err
	}

	filename, err = util.SaveSysex(filename, patch)
	if err != nil {
		return err
	}
	log.Printf("Saved NR2X voice %s to %s\n", conf.Voice, filename)
	return nil

}

func GetProgram(conf *Nr2xConnectionConfig, perc bool, filename string) error {
	if perc {
		return GetPercussionProgram(conf, filename)
	}
	return GetStandardProgram(conf, filename)
}

func GetStandardProgram(conf *Nr2xConnectionConfig, filename string) error {
	conn, err := NewNr2xConnection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	controllerValues, err := conn.GetControllerValues()
	if err != nil {
		return err
	}

	filename, err = util.SaveControllerValues(filename, controllerValues)
	if err != nil {
		return err
	}
	log.Printf("Saved NR2X voice %s to %s\n", conf.Voice, filename)
	return nil
}

func SetSysExProgram(conf *Nr2xConnectionConfig, perc bool, filename string) error {
	sysEx, err := util.LoadSysEx(filename)
	if err != nil {
		return errors.Wrapf(err, "error reading file '%s'", filename)
	}

	conn, err := NewNr2xConnection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.SendPatch(sysEx, perc); err != nil {
		return errors.Wrap(err, "error sending patch")
	}

	return nil
}

func SetProgram(conf *Nr2xConnectionConfig, perc bool, filename string) error {
	if perc {
		return SetPercussionProgram(conf, filename)
	}
	return SetStandardProgram(conf, filename)
}

func SetStandardProgram(conf *Nr2xConnectionConfig, filename string) error {
	conn, err := NewNr2xConnection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	controllerValues, err := util.LoadControllerValues(filename)
	if err != nil {
		return err
	}

	for _, controllerValue := range controllerValues {
		if err := conn.SendControlChange(controllerValue.Controller, controllerValue.Value); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}

	log.Printf("Sent program %s to NR2X voice %s\n", filename, conf.Voice)
	return nil
}

func GetPercussionProgram(conf *Nr2xConnectionConfig, filename string) error {
	conn, err := NewNr2xConnection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	percussionValues := []*util.VoiceControllerValue{}
	var i uint8
	for i = 0; i < 8; i++ {
		if err := conn.SendPercussionEdit(i); err != nil {
			return err
		}
		controllerValues, err := conn.GetControllerValues()
		if err != nil {
			return err
		}
		for _, cv := range controllerValues {
			percussionValues = append(percussionValues, &util.VoiceControllerValue{
				Voice:      i,
				Controller: cv.Controller,
				Value:      cv.Value,
			})
		}
	}

	filename, err = util.SaveVoiceControllerValues(filename, percussionValues)
	if err != nil {
		return err
	}
	log.Printf("Saved NR2X percussion voice %s to %s\n", conf.Voice, filename)
	return nil
}

func SetPercussionProgram(conf *Nr2xConnectionConfig, filename string) error {
	conn, err := NewNr2xConnection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	percussionValues, err := util.LoadVoiceControllerValues(filename)
	if err != nil {
		return err
	}
	percussionValuesMap := map[uint8][]*util.ControllerValue{}
	var i uint8
	for i = 0; i < 8; i++ {
		percussionValuesMap[i] = []*util.ControllerValue{}
	}
	for _, vcv := range percussionValues {
		percussionValuesMap[vcv.Voice] = append(percussionValuesMap[vcv.Voice], &util.ControllerValue{
			Controller: vcv.Controller,
			Value:      vcv.Value,
		})
	}

	for perc, controllerValues := range percussionValuesMap {
		if err := conn.SendPercussionEdit(perc); err != nil {
			return err
		}
		for _, controllerValue := range controllerValues {
			if err := conn.SendControlChange(controllerValue.Controller, controllerValue.Value); err != nil {
				return err
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	log.Printf("Sent percussion program %s to NR2X voice %s\n", filename, conf.Voice)
	return nil
}

func MakePercussionVariations(varFiles []string, outFile string, maxMspFormat bool) error {
	voiceVarConVals := make([][]*util.VoiceControllerValue, 8)
	for i := 0; i < 8; i++ {
		voiceVarConVals[i] = []*util.VoiceControllerValue{}
	}
	for i, varFile := range varFiles {
		vcvs, err := util.LoadVoiceControllerValues(varFile)
		if err != nil {
			return err
		}

		for _, vcv := range vcvs {
			varConVal := &util.VoiceControllerValue{
				Voice:      uint8(i), // ie variation
				Controller: vcv.Controller,
				Value:      vcv.Value,
			}
			voiceVarConVals[vcv.Voice] = append(voiceVarConVals[vcv.Voice], varConVal)
		}
	}

	filename, err := util.SaveVoiceVariationControllerValuesAsMaxMsp(outFile, voiceVarConVals)
	if err != nil {
		return err
	}
	log.Printf("Saved NR2X percussion variations to %s\n", filename)
	return nil
}
