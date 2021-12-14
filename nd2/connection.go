package nd2

import (
	"fmt"
	"reflect"
	"time"

	_ "embed"

	"github.com/gocarina/gocsv"
	"github.com/pkg/errors"
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/sysex"
	"gitlab.com/gomidi/midi/reader"
	"mvw.org/cctools/util"
)

// All controllers, arranged with LSB/MSB pairs in order
// controllers 0 and 32 (bank select), and 70 (channel focus) are ignored
var Nd2Controllers = []uint8{
	// simple controllers
	7, 10, 14, 15, 16, 17, 18, 19, 20, 21, 22,
	23, 24, 25, 26, 27, 28, 30, 46, 47, 48, 49,
	50, 51, 52, 53, 54, 55, 56, 57, 58, 59,
	// LSB/MSB pairs
	61, 29, 63, 31}

// Simple controllers
var SimpleNd2Controllers = []uint8{
	7, 10, 14, 15, 16, 17, 18, 19, 20, 21, 22,
	23, 24, 25, 26, 27, 28, 30, 46, 47, 48, 49,
	50, 51, 52, 53, 54, 55, 56, 57, 58, 59}

// Controllers with LSB/MSB pairs
var TonePitchController = []uint8{63, 31} // semitone, tone
var EchoBbmController = []uint8{61, 29}
var Nd2LsbMsbControllers = [][]uint8{TonePitchController, EchoBbmController}

// Voice change sysex
var VoiceChangeCcValues = []uint8{0, 26, 51, 77, 102, 107}
var VoiceChangeController uint8 = 70

// SysEx program msg format
const ProgramBytes = 210
const ProgramHeaderBytes = 12
const ProgramVoiceBytes = 32
const ChecksumBytes = 4

var HandshakeRequest = []byte{51, 127, 127, 7, 0, 6, 0, 127}
var HandshakeResponse = []byte{51, 127, 25, 7, 0, 7, 0, 0, 75, 0, 0, 55, 64, 24}

//go:embed data/controller-bit-ranges.csv
var ControllerBitRangesData []byte
var ControllerBitRanges map[uint8]*BitRange

//go:embed data/controller-value-sysex-map.csv
var ControllerValueSysexMapData []byte
var ControllerValueSysexMap map[uint8]map[uint8]uint8

const ConnectionSleepTime = time.Millisecond * 10
const ConnectionMaxWaitTime = time.Second * 5

var MsbMaskMap = map[int]uint8{
	1: 0b1111111, 2: 0b111111, 3: 0b11111, 4: 0b1111, 5: 0b111, 6: 0b11, 7: 0b1,
}

func init() {
	LoadControllerBitRanges()
	LoadSysexControllerValueMap()
}

func LoadSysexControllerValueMap() {
	type row struct {
		Controller      uint8 `csv:"controller"`
		ControllerValue uint8 `csv:"controller_value"`
		SysexValue      uint8 `csv:"sysex_value"`
	}

	rows := []*row{}
	if err := gocsv.UnmarshalBytes(ControllerValueSysexMapData, &rows); err != nil {
		panic(err)
	}

	ControllerValueSysexMap = map[uint8]map[uint8]uint8{}
	for _, row := range rows {
		bts, ok := ControllerValueSysexMap[row.Controller]
		if !ok {
			bts = map[uint8]uint8{}
			ControllerValueSysexMap[row.Controller] = bts
		}
		current, ok := bts[row.SysexValue]
		if !ok || current > row.ControllerValue { // always use lowest value
			bts[row.SysexValue] = row.ControllerValue
		}
	}
}

type BitRange struct {
	Controller uint8 `csv:"controller"`
	First      int   `csv:"first"`
	Last       int   `csv:"last"`
}

func LoadControllerBitRanges() {
	rows := []*BitRange{}
	if err := gocsv.UnmarshalBytes(ControllerBitRangesData, &rows); err != nil {
		panic(err)
	}

	ControllerBitRanges = map[uint8]*BitRange{}
	for _, bitRange := range rows {
		ControllerBitRanges[bitRange.Controller] = bitRange
	}
}

type Nd2Connection struct {
	Config       *Nd2ConnectionConfig
	readerWriter *util.MidiReaderWriter
	responseChan chan sysex.SysEx
	shutdownChan chan interface{}
}

type Nd2ConnectionConfig struct {
	InPort            uint  `yaml:"in_port"`
	OutPort           uint  `yaml:"out_port"`
	BaseMidiChannel   uint8 `yaml:"base_midi_channel"`
	GlobalMidiChannel uint8 `yaml:"global_midi_channel"`
}

func NewNd2Connection(config *Nd2ConnectionConfig) (nd2c *Nd2Connection, errVal error) {
	conn := &Nd2Connection{
		Config:       config,
		responseChan: make(chan sysex.SysEx, 1),
		shutdownChan: make(chan interface{}, 1),
	}

	readerWriter, err := util.NewMidiReaderWriter(config.InPort, config.OutPort, func(pos *reader.Position, msg midi.Message) {
		if sysExMsg, ok := msg.(sysex.SysEx); ok {
			conn.responseChan <- sysExMsg
		}
	})
	if err != nil {
		return nil, err
	}

	conn.readerWriter = readerWriter

	if err := conn.Handshake(); err != nil {
		return nil, errors.Wrapf(err, "Cannot connect to ND2 on port %d (%s): handshake failed",
			conn.readerWriter.Out.Number(), conn.readerWriter.Out.String())
	}

	fmt.Println("Connected to ND2")
	conn.readerWriter.PrintPorts()

	return conn, nil
}

func (conn *Nd2Connection) GetProgram() (Nd2ProgramSysex, error) {
	sysExData := []byte{51, 127, 127, 8, 3, 5, 0, 19}
	if err := conn.readerWriter.SysEx(sysExData); err != nil {
		return nil, errors.Wrap(err, "error sending SysEx message")
	}
	time.Sleep(ConnectionSleepTime)
	sysExReply, err := conn.waitForSysExMsg()
	if err != nil {
		return nil, err
	}
	return Nd2ProgramSysex(sysExReply), nil
}

func (conn *Nd2Connection) Handshake() error {
	if err := conn.readerWriter.SysEx(HandshakeRequest); err != nil {
		return errors.Wrap(err, "error sending SysEx message")
	}

	sysExReply, err := conn.waitForSysExMsg()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(sysExReply.Data(), HandshakeResponse) {
		return errors.Errorf("unexpected handshake reply (1): %v", sysExReply.Data())
	}

	return nil
}

func (conn *Nd2Connection) SendVoiceFocusChange(voice uint8) error {
	if err := conn.readerWriter.ControlChange(conn.Config.GlobalMidiChannel, VoiceChangeController, VoiceChangeCcValues[voice]); err != nil {
		return errors.Wrap(err, "error voice focus change change message")
	}
	time.Sleep(ConnectionSleepTime)
	return nil
}

func (conn *Nd2Connection) SendControlChange(voice, controller, value uint8) error {
	if err := conn.readerWriter.ControlChange(conn.Config.BaseMidiChannel+voice, controller, value); err != nil {
		return errors.Wrap(err, "error sending control change message")
	}
	time.Sleep(ConnectionSleepTime)
	return nil
}

func (conn *Nd2Connection) waitForSysExMsg() (sysex.SysEx, error) {
	select {
	case <-conn.shutdownChan:
		return nil, errors.New("cancelled")
	case <-time.After(ConnectionMaxWaitTime):
		return nil, errors.New("program request timed out")
	case sysExMsg := <-conn.responseChan:
		return sysExMsg, nil
	}
}

func (conn *Nd2Connection) Close() error {
	conn.shutdownChan <- nil
	return conn.readerWriter.Close()
}

type Nd2ProgramSysex sysex.SysEx

func (prog Nd2ProgramSysex) GetVoiceBytes(voice uint8) []byte {
	return prog[ProgramHeaderBytes+(voice*ProgramVoiceBytes) : ProgramHeaderBytes+((voice+1)*ProgramVoiceBytes)+1]
}

func (prog Nd2ProgramSysex) GetSysExValue(voice uint8, bitRange *BitRange) uint8 {
	voiceBytes := prog.GetVoiceBytes(voice)

	firstByte := bitRange.First / 8
	firstBit := bitRange.First % 8
	lastByte := bitRange.Last / 8
	lastBit := bitRange.Last % 8

	if firstByte == lastByte {
		return voiceBytes[firstByte] & MsbMaskMap[firstBit] >> (7 - lastBit)
	} else if firstByte+1 == lastByte {
		valMsb := voiceBytes[lastByte] >> (7 - byte(lastBit))
		valLsb := voiceBytes[firstByte] & MsbMaskMap[firstBit] << byte(lastBit)
		return valLsb + valMsb
	} else {
		panic("non-contiguous bytes")
	}
}

func (prog Nd2ProgramSysex) GetVoiceControllerValues() ([]*util.VoiceControllerValue, error) {
	voiceControllerValues := []*util.VoiceControllerValue{}
	var voice uint8
	for voice = 0; voice < 6; voice++ {
		for _, controller := range Nd2Controllers {
			sysExValue := prog.GetSysExValue(voice, ControllerBitRanges[controller])
			controllerValue := ControllerValueSysexMap[controller][sysExValue]
			voiceControllerValues = append(voiceControllerValues, &util.VoiceControllerValue{
				Voice:      voice,
				Controller: controller,
				Value:      controllerValue,
			})
		}
	}

	return voiceControllerValues, nil
}
