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
	"gitlab.com/gomidi/midi/writer"
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
var TonePitchController = []uint8{63, 31}
var EchoBbmController = []uint8{61, 29}
var Nd2LsbMsbControllers = [][]uint8{TonePitchController, EchoBbmController}

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
	reader       *reader.Reader
	writer       *writer.Writer
	closeFunc    func() error
	responseChan chan sysex.SysEx
	shutdownChan chan interface{}
}

func NewNd2Connection(inPort, outPort uint) (nd2c *Nd2Connection, errVal error) {
	conn := &Nd2Connection{
		responseChan: make(chan sysex.SysEx, 1),
		shutdownChan: make(chan interface{}, 1),
	}
	in, out, closeFunc, err := util.GetMidiPorts(inPort, outPort)
	if err != nil {
		return nil, err
	}
	defer func() {
		if errVal != nil {
			closeFunc()
		}
	}()
	conn.closeFunc = closeFunc

	conn.reader = reader.New(
		reader.NoLogger(),
		reader.Each(func(pos *reader.Position, msg midi.Message) {
			if sysExMsg, ok := msg.(sysex.SysEx); ok {
				conn.responseChan <- sysExMsg
			}
		}),
	)
	conn.reader.ListenTo(in)

	conn.writer = writer.New(out)

	if err := conn.Handshake(); err != nil {
		return nil, errors.Wrapf(err, "Cannot connect to ND2 on port %d (%s): handshake failed", out.Number(), out.String())
	}

	fmt.Println("Connected to ND2")
	fmt.Printf("MIDI in port:  %d (%s)\n", in.Number(), in.String())
	fmt.Printf("MIDI out port: %d (%s)\n", in.Number(), in.String())

	return conn, nil
}

func (conn *Nd2Connection) GetProgram() (Nd2ProgramSysex, error) {
	sysExData := []byte{51, 127, 127, 8, 3, 5, 0, 19}
	if err := writer.SysEx(conn.writer, sysExData); err != nil {
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
	if err := writer.SysEx(conn.writer, HandshakeRequest); err != nil {
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

func (conn *Nd2Connection) SendControlChange(channel, controller, value uint8) error {
	conn.writer.SetChannel(channel)
	if err := writer.ControlChange(conn.writer, controller, value); err != nil {
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
	return conn.closeFunc()
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
