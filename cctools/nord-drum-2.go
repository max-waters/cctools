package cctools

import (
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/pkg/errors"
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/sysex"
	"gitlab.com/gomidi/midi/reader"
	"gitlab.com/gomidi/midi/writer"
)

// simple controllers
var Nd2Ccs = []uint8{7, 10, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 30,
	46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59}

// controllers with LSB and MSB
var MsbLsbCcs = []uint8{29, 31, 61, 63}

// controllers 0 and 32 (bank select), and 70 (channel focus) are ignored

type Nd2Connection struct {
	reader       *reader.Reader
	writer       *writer.Writer
	closeFunc    func() error
	responseChan chan sysex.SysEx
}

func NewNd2Connection(inPort, outPort, outChan uint8) (*Nd2Connection, error) {
	conn := &Nd2Connection{
		responseChan: make(chan sysex.SysEx, 1),
	}
	in, inCloseFunc, err := getMidiInPort(inPort)
	if err != nil {
		return nil, err
	}

	conn.reader = reader.New(
		reader.NoLogger(),
		reader.Each(func(pos *reader.Position, msg midi.Message) {
			if sysExMsg, ok := msg.(sysex.SysEx); ok {
				conn.responseChan <- sysExMsg
			}
		}),
	)

	fmt.Printf("Listening to port %d (%s)\n", in.Number(), in.String())
	conn.reader.ListenTo(in)

	out, outCloseFunc, err := getMidiOutPort(outPort)
	if err != nil {
		fmt.Printf("Error opening MIDI out port: %s\n", err)
		if err := inCloseFunc(); err != nil {
			fmt.Printf("Error opening MIDI out port: %s\n", err)
		}
		return nil, err
	}
	conn.writer = writer.New(out)
	conn.writer.SetChannel(outChan)

	conn.closeFunc = func() error {
		if err := inCloseFunc(); err != nil {
			return err
		}
		return outCloseFunc()
	}
	return conn, nil
}

func (conn *Nd2Connection) SendProgramRequest() error {
	sysEx := []byte{51, 127, 127, 8, 3, 5, 0, 19}
	if err := writer.SysEx(conn.writer, sysEx); err != nil {
		return errors.Wrap(err, "error sending SysEx message")
	}
	return nil
}

func (conn *Nd2Connection) SendControlChange(controller, value uint8) error {
	if err := writer.ControlChange(conn.writer, controller, value); err != nil {
		return errors.Wrap(err, "error sending control change message")
	}
	return nil
}

type Nd2Hacker struct {
	nd2Conn      *Nd2Connection
	shutdownChan chan interface{}
	bitRanges    map[uint8]*BitRange
	valueByteMap map[uint8]map[uint8]uint8
}

func NewNd2Hacker(inPort, outPort, outChan uint8) (*Nd2Hacker, error) {
	nd2Conn, err := NewNd2Connection(inPort, outPort, outChan)
	if err != nil {
		return nil, err
	}
	return &Nd2Hacker{
		nd2Conn:      nd2Conn,
		shutdownChan: make(chan interface{}, 1),
	}, nil
}

func (hacker *Nd2Hacker) FindControllerBitRanges() error {
	defer hacker.nd2Conn.closeFunc()

	fmt.Println("controller,first,last")
	var c uint8
	for _, c = range Nd2Ccs {
		first, last, err := hacker.findControllerBitRange(c)
		if err != nil {
			return errors.Wrapf(err, "cannot get bit indexes for controller %d", c)
		}
		if first < 0 {
			fmt.Printf("%d,,\n", c)
			continue
		}
		fmt.Printf("%d,%d,%d\n", c, first, last)
	}

	return nil
}

func (hacker *Nd2Hacker) findControllerBitRange(c uint8) (int, int, error) {
	if err := hacker.resetCcs(); err != nil {
		return -1, -1, errors.Wrap(err, "cannot reset control change data")
	}

	if err := hacker.nd2Conn.SendProgramRequest(); err != nil {
		return -1, -1, err
	}
	zeroSysExMsg := <-hacker.nd2Conn.responseChan

	first := 1000000
	last := -1

	var v uint8
	for v = 0; v < 128; v++ {
		if err := hacker.nd2Conn.SendControlChange(c, v); err != nil {
			return -1, -1, err
		}
		time.Sleep(50 * time.Millisecond)
		if err := hacker.nd2Conn.SendProgramRequest(); err != nil {
			return -1, -1, err
		}

		select {
		case <-hacker.shutdownChan:
			fmt.Println("Exiting")
			return -1, -1, errors.New("cancelled")
		case sysExMsg := <-hacker.nd2Conn.responseChan:
			thisFirst, thisLast := getDifferences(sysExMsg.Raw(), zeroSysExMsg.Raw())
			if thisFirst < first {
				first = thisFirst
			}
			if thisLast > last {
				last = thisLast
			}
		}
	}

	return first, last, nil
}

func getDifferences(b1, b2 []byte) (int, int) {
	b1Bin := toBoolArray(b1)
	b2Bin := toBoolArray(b2)
	first := 10000
	for i := 7; i < len(b1Bin)-(5*7); i++ {
		if b1Bin[i] != b2Bin[i] {
			first = i
			break
		}
	}

	last := -1
	for i := len(b1Bin) - (6 * 7); i >= first; i-- {
		if b1Bin[i] != b2Bin[i] {
			last = i
			break
		}
	}
	return first, last

}

func (hacker *Nd2Hacker) resetCcs() error {
	var c uint8
	for _, c = range Nd2Ccs {
		if err := hacker.nd2Conn.SendControlChange(c, 0); err != nil {
			return err
		}
	}
	return nil
}

func (hacker *Nd2Hacker) FindControllerByteValues() error {
	defer hacker.nd2Conn.closeFunc()

	bitRanges, err := LoadControllerBitRanges()
	if err != nil {
		return err
	}
	hacker.bitRanges = bitRanges

	if err := hacker.resetCcs(); err != nil {
		return errors.Wrap(err, "cannot reset control change data")
	}

	fmt.Println("controller,controller_value,byte_value")
	var v uint8
	for _, c := range Nd2Ccs {
		for v = 0; v < 128; v++ {
			if err := hacker.nd2Conn.SendControlChange(c, v); err != nil {
				return err
			}

			time.Sleep(50 * time.Millisecond)
			if err := hacker.nd2Conn.SendProgramRequest(); err != nil {
				return err
			}

			select {
			case <-hacker.shutdownChan:
				return errors.New("cancelled")
			case sysExMsg := <-hacker.nd2Conn.responseChan:
				val := ParseControllerByteValue(sysExMsg, hacker.bitRanges[c])
				fmt.Printf("%d,%d,%d\n", c, v, val)
			}
		}
	}
	return nil
}

func (hacker *Nd2Hacker) Test() error {
	controllerValueByteMap, err := LoadControllerByteValueMap()
	if err != nil {
		return err
	}
	hacker.valueByteMap = controllerValueByteMap

	controllerBitRanges, err := LoadControllerBitRanges()
	if err != nil {
		return err
	}
	hacker.bitRanges = controllerBitRanges

	randomGen := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 0; i < 100; i++ {
		fmt.Printf("Test %d\n", i+1)
		// generate and send random cc values
		rand := make([]uint8, len(Nd2Ccs))
		for j := 0; j < len(rand); j++ {
			rand[j] = uint8(randomGen.Intn(128))
		}

		normalised, err := hacker.sendAndParseControlChanges(rand)
		if err != nil {
			return err
		}

		reply, err := hacker.sendAndParseControlChanges(normalised)
		if err != nil {
			return err
		}

		if !reflect.DeepEqual(normalised, reply) {
			return errors.Errorf("Unexpected controller values.\nExpected %v\nActual   %v", normalised, reply)
		}
	}
	return nil
}

func (hacker *Nd2Hacker) sendAndParseControlChanges(values []uint8) ([]uint8, error) {
	for i, v := range values {
		if err := hacker.nd2Conn.SendControlChange(Nd2Ccs[i], v); err != nil {
			return nil, err
		}
	}

	time.Sleep(50 * time.Millisecond)
	if err := hacker.nd2Conn.SendProgramRequest(); err != nil {
		return nil, err
	}

	select {
	case <-hacker.shutdownChan:
		return nil, errors.New("cancelled")
	case sysExMsg := <-hacker.nd2Conn.responseChan:
		// transform sysex into cc values
		parsed := make([]uint8, len(Nd2Ccs))
		byteVals := make([]uint8, len(Nd2Ccs))
		for j, c := range Nd2Ccs {
			val := ParseControllerByteValue(sysExMsg, hacker.bitRanges[c])
			byteVals[j] = val
			cc, ok := hacker.valueByteMap[c][val]
			if !ok {
				return nil, errors.Errorf("No value found for controller %d and byte value %d", c, val)
			}
			parsed[j] = cc
		}

		return parsed, nil
	}
}

func (hacker *Nd2Hacker) Stop() {
	hacker.shutdownChan <- nil
}

type BitRange struct {
	Controller uint8 `csv:"controller"`
	First      int   `csv:"first"`
	Last       int   `csv:"last"`
}

func ParseControllerByteValue(sysex sysex.SysEx, bitRange *BitRange) uint8 {
	binMsg := toBoolArray(sysex.Raw())
	binVal := binMsg[bitRange.First : bitRange.Last+1]
	return toUint8(binVal)
}

func toUint8(bin []bool) uint8 {
	if len(bin) > 7 {
		panic("too many bits!")
	}

	var val uint8 = 0
	var f uint8 = 1
	for i := len(bin) - 1; i >= 0; i-- {
		if bin[i] {
			val += f
		}
		f *= 2
	}
	return val
}

func toBoolArray(bts []byte) []bool {
	bin := make([]bool, 7*len(bts))
	for i, bt := range bts {
		for j := 6; j >= 0; j-- { // ignore last bit
			bin[(i*7)+j] = bt%2 != 0 // 1 -> true
			bt /= 2
		}
	}
	return bin
}

func LoadControllerByteValueMap() (map[uint8]map[uint8]uint8, error) {
	type cvb struct {
		Controller uint8 `csv:"controller"`
		Value      uint8 `csv:"controller_value"`
		Byte       uint8 `csv:"byte_value"`
	}

	f, err := os.Open("./data/nd2-cc-byte-val-map.csv")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rows := []*cvb{}
	if err := gocsv.UnmarshalFile(f, &rows); err != nil {
		return nil, err
	}

	cvbMap := map[uint8]map[uint8]uint8{}
	for _, row := range rows {
		bts, ok := cvbMap[row.Controller]
		if !ok {
			bts = map[uint8]uint8{}
			cvbMap[row.Controller] = bts
		}
		current, ok := bts[row.Byte]
		if !ok || current > row.Value { // always use lowest value
			bts[row.Byte] = row.Value
		}
	}

	return cvbMap, nil
}

func LoadControllerBitRanges() (map[uint8]*BitRange, error) {
	f, err := os.Open("./data/nd2-cc-bit-ranges.csv")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ranges := []*BitRange{}
	if err := gocsv.UnmarshalFile(f, &ranges); err != nil {
		return nil, err
	}

	bitRanges := map[uint8]*BitRange{}
	for _, bitRange := range ranges {
		bitRanges[bitRange.Controller] = bitRange
	}
	return bitRanges, nil
}
