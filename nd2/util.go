package nd2

import (
	"fmt"
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
const HeaderBytes = 12
const VoiceBytes = 32
const ChecksumBytes = 4

//go:embed data/controller-bit-ranges.csv
var ControllerBitRanges []byte

//go:embed data/controller-value-sysex-map.csv
var ControllerValueSysexMap []byte

const ConnectionSleepTime = time.Millisecond * 50

var MsbMaskMap = map[int]uint8{
	1: 0b1111111, 2: 0b111111, 3: 0b11111, 4: 0b1111, 5: 0b111, 6: 0b11, 7: 0b1,
}

func LoadSysexControllerValueMap() (map[uint8]map[uint8]uint8, error) {
	type row struct {
		Controller      uint8 `csv:"controller"`
		ControllerValue uint8 `csv:"controller_value"`
		SysexValue      uint8 `csv:"sysex_value"`
	}

	rows := []*row{}
	if err := gocsv.UnmarshalBytes(ControllerValueSysexMap, &rows); err != nil {
		return nil, err
	}

	scvMap := map[uint8]map[uint8]uint8{}
	for _, row := range rows {
		bts, ok := scvMap[row.Controller]
		if !ok {
			bts = map[uint8]uint8{}
			scvMap[row.Controller] = bts
		}
		current, ok := bts[row.SysexValue]
		if !ok || current > row.ControllerValue { // always use lowest value
			bts[row.SysexValue] = row.ControllerValue
		}
	}
	return scvMap, nil
}

type BitRange struct {
	Controller uint8 `csv:"controller"`
	First      int   `csv:"first"`
	Last       int   `csv:"last"`
}

func LoadControllerBitRanges() (map[uint8]*BitRange, error) {
	rows := []*BitRange{}
	if err := gocsv.UnmarshalBytes(ControllerBitRanges, &rows); err != nil {
		return nil, err
	}

	controllerBitRanges := map[uint8]*BitRange{}
	for _, bitRange := range rows {
		controllerBitRanges[bitRange.Controller] = bitRange
	}
	return controllerBitRanges, nil
}

type Nd2Connection struct {
	reader       *reader.Reader
	writer       *writer.Writer
	closeFunc    func() error
	responseChan chan sysex.SysEx
}

func NewNd2Connection(inPort, outPort uint) (nd2c *Nd2Connection, errVal error) {
	conn := &Nd2Connection{
		responseChan: make(chan sysex.SysEx, 1),
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
	fmt.Printf("Listening to port %d (%s)\n", in.Number(), in.String())

	conn.writer = writer.New(out)

	return conn, nil
}

func (conn *Nd2Connection) SendProgramRequest() error {
	sysEx := []byte{51, 127, 127, 8, 3, 5, 0, 19}
	if err := writer.SysEx(conn.writer, sysEx); err != nil {
		return errors.Wrap(err, "error sending SysEx message")
	}
	time.Sleep(ConnectionSleepTime)
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

func (conn *Nd2Connection) Close() error {
	return conn.closeFunc()
}

type Nd2ProgramSysex sysex.SysEx

func (prog Nd2ProgramSysex) GetVoiceBytes(voice int) []byte {
	return prog[HeaderBytes+(voice*VoiceBytes) : HeaderBytes+((voice+1)*VoiceBytes)+1]
}

func (prog Nd2ProgramSysex) GetSysExValue(first, last int) uint8 {
	firstByte := first / 8
	firstBit := first % 8
	lastByte := last / 8
	lastBit := last % 8

	if firstByte == lastByte {
		return prog[firstByte] & MsbMaskMap[firstBit] >> (7 - lastBit)
	} else if firstByte+1 == lastByte {
		valMsb := prog[lastByte] >> (7 - byte(lastBit))
		valLsb := prog[firstByte] & MsbMaskMap[firstBit] << byte(lastBit)
		return valLsb + valMsb
	} else {
		panic("non-contiguous bytes")
	}
}
