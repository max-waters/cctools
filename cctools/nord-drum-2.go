package cctools

import (
	"fmt"
	"os"
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

type Nd2Hacker struct {
	inPort       uint8
	outPort      uint8
	outChan      uint8
	reader       *reader.Reader
	writer       *writer.Writer
	shutdownChan chan interface{}
	responseChan chan sysex.SysEx
}

func NewNd2Hacker(inPort, outPort, outChan uint8) *Nd2Hacker {
	return &Nd2Hacker{
		inPort:       inPort,
		outPort:      outPort,
		outChan:      outChan,
		shutdownChan: make(chan interface{}, 1),
		responseChan: make(chan sysex.SysEx, 1),
	}
}

func (hacker *Nd2Hacker) init() (func() error, error) {
	in, inCloseFunc, err := getMidiInPort(hacker.inPort)
	if err != nil {
		return nil, err
	}

	hacker.reader = reader.New(
		reader.NoLogger(),
		reader.Each(func(pos *reader.Position, msg midi.Message) {
			if sysExMsg, ok := msg.(sysex.SysEx); ok {
				hacker.responseChan <- sysExMsg
			}
		}),
	)

	fmt.Printf("Listening to port %d (%s)\n", in.Number(), in.String())
	hacker.reader.ListenTo(in)

	out, outCloseFunc, err := getMidiOutPort(hacker.outPort)
	if err != nil {
		fmt.Printf("Error opening MIDI out port: %s\n", err)
		if err := inCloseFunc(); err != nil {
			fmt.Printf("Error opening MIDI out port: %s\n", err)
		}
		return nil, err
	}
	hacker.writer = writer.New(out)
	hacker.writer.SetChannel(hacker.outChan)

	return func() error {
		if err := inCloseFunc(); err != nil {
			return err
		}
		return outCloseFunc()
	}, nil

}

func (hacker *Nd2Hacker) GetControllerBitLocations() error {
	closeFunc, err := hacker.init()
	if err != nil {
		return err
	}
	defer closeFunc()

	var c uint8
	for _, c = range Nd2Ccs {
		first, last, err := hacker.getBitIndexesForCc(c)
		if err != nil {
			return errors.Wrapf(err, "cannot get bit indexes for controller %d", c)
		}
		if first < 0 {
			fmt.Printf("No bit indexes found")
			continue
		}
		fmt.Printf("%d,%d,%d\n", c, first, last)
	}

	return nil
}

func (hacker *Nd2Hacker) getBitIndexesForCc(c uint8) (int, int, error) {
	if err := hacker.resetCcs(); err != nil {
		return -1, -1, errors.Wrap(err, "cannot reset control change data")
	}

	if err := hacker.getProgram(); err != nil {
		return -1, -1, errors.Wrap(err, "cannot get initial program dump")
	}
	zeroSysExMsg := <-hacker.responseChan

	first := 1000000
	last := -1

	var v uint8
	for v = 0; v < 128; v++ {
		if err := writer.ControlChange(hacker.writer, c, v); err != nil {
			return -1, -1, errors.Wrapf(err, "error sending control change msg")
		}
		time.Sleep(50 * time.Millisecond)
		if err := hacker.getProgram(); err != nil {
			return -1, -1, errors.Wrap(err, "cannot get program dump")
		}

		select {
		case <-hacker.shutdownChan:
			fmt.Println("Exiting")
			return -1, -1, errors.New("cancelled")
		case sysExMsg := <-hacker.responseChan:
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

func (hacker *Nd2Hacker) getProgram() error {
	sysEx := []byte{51, 127, 127, 8, 3, 5, 0, 19}
	if err := writer.SysEx(hacker.writer, sysEx); err != nil {
		return errors.Wrap(err, "error sending SysEx message")
	}
	return nil
}

func (hacker *Nd2Hacker) resetCcs() error {
	var c uint8
	for _, c = range Nd2Ccs {
		if err := writer.ControlChange(hacker.writer, c, 0); err != nil {
			return errors.Wrap(err, "error sending control change message")
		}
	}
	return nil
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

func (hacker *Nd2Hacker) GetControllerByteValueMap() error {
	bitRanges, err := LoadBitRanges()
	if err != nil {
		return errors.Wrap(err, "cannot load bit range file")
	}

	closeFunc, err := hacker.init()
	if err != nil {
		return err
	}
	defer closeFunc()

	if err := hacker.resetCcs(); err != nil {
		return errors.Wrap(err, "cannot reset control change data")
	}

	if err := hacker.getProgram(); err != nil {
		return errors.Wrap(err, "cannot get initial program dump")
	}

	var v uint8
	for _, c := range Nd2Ccs {
		bitRange := bitRanges[c]
		for v = 0; v < 128; v++ {
			if err := writer.ControlChange(hacker.writer, c, v); err != nil {
				return errors.Wrapf(err, "error sending control change msg")
			}
			time.Sleep(50 * time.Millisecond)
			if err := hacker.getProgram(); err != nil {
				return errors.Wrap(err, "cannot get program dump")
			}

			select {
			case <-hacker.shutdownChan:
				return errors.New("cancelled")
			case sysExMsg := <-hacker.responseChan:
				binMsg := toBoolArray(sysExMsg.Raw())
				binVal := binMsg[bitRange.First : bitRange.Last+1]
				val := toIntVal(binVal)
				fmt.Printf("%d,%d,%d\n", c, v, val)
			}
		}
	}

	return nil
}

type BitRange struct {
	Controller uint8 `csv:"controller"`
	First      int   `csv:"first"`
	Last       int   `csv:"last"`
}

func LoadBitRanges() (map[uint8]*BitRange, error) {
	f, err := os.Open("./data/nd2-cc-bit-ranges.csv")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ranges := []*BitRange{}
	if err := gocsv.UnmarshalFile(f, &ranges); err != nil {
		return nil, err
	}

	rangeMap := map[uint8]*BitRange{}
	for _, bitRange := range ranges {
		rangeMap[bitRange.Controller] = bitRange
	}
	return rangeMap, nil
}

func toIntVal(bin []bool) int {
	val := 0
	f := 1
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

func toByteArray(bin []bool) []byte {
	bts := []byte{}
	for i := 0; i < len(bin); i += 7 {
		var bt byte = 0
		var f byte = 1
		for j := 0; j < 7; j++ {
			if bin[i+j] {
				bt += f
			}
			f *= 2
		}
		bts = append(bts, bt)
	}
	return bts
}

func (hacker *Nd2Hacker) Stop() {
	hacker.shutdownChan <- nil
}
