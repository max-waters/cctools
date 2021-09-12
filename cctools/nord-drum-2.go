package cctools

import (
	"fmt"

	"github.com/pkg/errors"
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/sysex"
	"gitlab.com/gomidi/midi/reader"
	"gitlab.com/gomidi/midi/writer"
)

var Nd2Ccs = []uint8{7, 10, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31,
	46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 61, 63}

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

func (hacker *Nd2Hacker) Start() error {
	closeFunc, err := hacker.init()
	if err != nil {
		return err
	}
	defer closeFunc()

	var c uint8
	for _, c = range Nd2Ccs {
		//fmt.Printf("Controller %d\n", c)
		first, last, err := hacker.getBytesIndexesForCc(c)
		if err != nil {
			return errors.Wrapf(err, "cannot get byte indexes for controller %d", c)
		}
		if first == nil && last == nil {
			fmt.Printf("No difference found")
			continue
		}
		fmt.Printf("%d,%d,%d,%d,%d\n", c, first.byteNum, first.bitNum, last.byteNum, last.bitNum)
		//fmt.Printf("%d,%d -> %d,%d\n", first.byteNum, first.bitNum, last.byteNum, last.bitNum)
	}

	return nil
}

func (hacker *Nd2Hacker) getBytesIndexesForCc(c uint8) (*BytePos, *BytePos, error) {
	if err := hacker.resetCcs(); err != nil {
		return nil, nil, errors.Wrap(err, "cannot reset control change data")
	}

	if err := hacker.getProgram(); err != nil {
		return nil, nil, errors.Wrap(err, "cannot get initial program dump")
	}
	zeroSysExMsg := <-hacker.responseChan

	first := &BytePos{byteNum: 1000000, bitNum: 9}
	last := &BytePos{byteNum: 0, bitNum: 0}

	var v uint8
	for v = 0; v < 128; v++ {
		if err := writer.ControlChange(hacker.writer, c, v); err != nil {
			return nil, nil, errors.Wrapf(err, "error sending control change msg")
		}
		if err := hacker.getProgram(); err != nil {
			return nil, nil, errors.Wrap(err, "cannot get program dump")
		}

		select {
		case <-hacker.shutdownChan:
			fmt.Println("Exiting")
			return nil, nil, errors.New("cancelled")
		case sysExMsg := <-hacker.responseChan:
			thisFirst, thisLast := getDifferences(sysExMsg.Raw(), zeroSysExMsg.Raw())
			if thisFirst == nil && thisLast == nil { // no diff
				continue
			}
			if thisFirst.getBitPos() < first.getBitPos() {
				first = thisFirst
			}
			if thisLast.getBitPos() > last.getBitPos() {
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

type BytePos struct {
	byteNum uint
	bitNum  uint
}

func (bp *BytePos) getBitPos() uint {
	return (bp.byteNum * 8) + bp.bitNum
}

func getDifferences(b1, b2 []byte) (*BytePos, *BytePos) {
	// ignore first byte, and last 5 bytes
	var first *BytePos
	for i := 0; i < len(b1)-5; i++ {
		if b1[i] != b2[i] {
			first = &BytePos{byteNum: uint(i)}
			break
		}

	}
	if first == nil {
		return nil, nil
	}
	f, _ := getBitDifferences(b1[first.byteNum], b2[first.byteNum])
	first.bitNum = f

	var last *BytePos
	for i := len(b1) - 6; i >= int(first.byteNum); i-- {
		if b1[i] != b2[i] {
			last = &BytePos{byteNum: uint(i)}
			break
		}
	}
	_, l := getBitDifferences(b1[last.byteNum], b2[last.byteNum])
	last.bitNum = l

	return first, last
}

func getBitDifferences(b1, b2 uint8) (uint, uint) {
	f := -1
	l := -1
	for i := 0; i <= 7; i++ {
		if b1%2 != b2%2 {
			l = i
			if f == -1 {
				f = i
			}
		}
		b1 /= 2
		b2 /= 2
	}
	if f == -1 || l == -1 {
		panic("bit diffs")
	}
	return uint(8 - f), uint(8 - l)
}

func (hacker *Nd2Hacker) Stop() {
	hacker.shutdownChan <- nil
}
