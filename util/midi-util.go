package util

import (
	"fmt"

	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/sysex"
	"gitlab.com/gomidi/midi/reader"
	"gitlab.com/gomidi/midi/writer"
	driver "gitlab.com/gomidi/rtmididrv"
)

type VoiceControllerValue struct {
	Voice      uint8 `csv:"voice"`
	Controller uint8 `csv:"controller"`
	Value      uint8 `csv:"value"`
}

type ControllerValue struct {
	Controller uint8 `csv:"controller"`
	Value      uint8 `csv:"value"`
}

type MidiLogger struct {
	port         uint
	shutdownChan chan interface{}
}

func NewMidiLogger(port uint) *MidiLogger {
	return &MidiLogger{
		port:         port,
		shutdownChan: make(chan interface{}, 1),
	}
}

func (logger *MidiLogger) Start() error {
	fmt.Printf("Opening port %d\n", logger.port+1)
	in, closeFunc, err := GetMidiInPort(logger.port)
	if err != nil {
		return err
	}
	defer closeFunc()

	reader := reader.New(
		reader.NoLogger(),
		reader.Each(func(pos *reader.Position, msg midi.Message) {
			fmt.Println(msg.String())
			if sysExMsg, ok := msg.(sysex.SysEx); ok {
				fmt.Println(sysExMsg.Raw())
			}
		}),
	)

	fmt.Printf("Listening to port %d (%s)\n", in.Number()+1, in.String())
	reader.ListenTo(in)
	<-logger.shutdownChan
	return nil
}

func (logger *MidiLogger) Stop() {
	logger.shutdownChan <- nil
}

func ListPorts() (errVal error) {
	drv, err := driver.New()
	if err != nil {
		return err
	}
	defer func() {
		if errVal != nil {
			drv.Close()
		}
	}()

	// ins
	ins, err := drv.Ins()
	if err != nil {
		return err
	}
	if len(ins) == 0 {
		fmt.Println("No MIDI in ports found")
	} else {
		fmt.Println("MIDI in ports:")
		for _, port := range ins {
			fmt.Printf("%v: %s\n", port.Number()+1, port.String())
		}
	}

	// outs
	outs, err := drv.Outs()
	if err != nil {
		return err
	}
	if len(outs) == 0 {
		fmt.Println("No MIDI out ports found")
	} else {
		fmt.Println("MIDI out ports:")
		for _, port := range outs {
			fmt.Printf("%v: %s\n", port.Number()+1, port.String())
		}
	}

	return nil
}

func GetMidiPorts(inPortNum, outPortNum uint) (inPort midi.In, outPort midi.Out, closeFunc func() error, errVal error) {
	drv, err := driver.New()
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() {
		if errVal != nil {
			drv.Close()
		}
	}()

	in, err := openMidiInPort(drv, inPortNum)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() {
		if errVal != nil {
			in.Close()
		}
	}()

	out, err := openMidiOutPort(drv, outPortNum)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() {
		if errVal != nil {
			out.Close()
		}
	}()

	return in, out, func() error {
		if err := in.Close(); err != nil {
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return drv.Close()
	}, nil
}

func GetMidiOutPort(port uint) (outputPort midi.Out, closeFunc func() error, errVal error) {
	drv, err := driver.New()
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if errVal != nil {
			drv.Close()
		}
	}()

	out, err := openMidiOutPort(drv, port)
	if err != nil {
		return nil, nil, err
	}

	closeFunc = func() error {
		if err := out.Close(); err != nil {
			return err
		}
		return drv.Close()
	}

	return out, closeFunc, nil
}

func GetMidiInPort(port uint) (inputPort midi.In, closeFunc func() error, errVal error) {
	drv, err := driver.New()
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if errVal != nil {
			drv.Close()
		}
	}()

	in, err := openMidiInPort(drv, port)
	if err != nil {
		return nil, nil, err
	}

	closeFunc = func() error {
		if err := in.Close(); err != nil {
			return err
		}
		return drv.Close()
	}
	return in, closeFunc, nil
}

func openMidiInPort(drv *driver.Driver, inPortNum uint) (inPort midi.In, errVal error) {
	ins, err := drv.Ins()
	if err != nil {
		return nil, err
	}
	if int(inPortNum) >= len(ins) {
		return nil, fmt.Errorf("unknown port number: %d", inPortNum+1)
	}
	in := ins[inPortNum]
	if err := in.Open(); err != nil {
		return nil, err
	}
	return in, nil
}

func openMidiOutPort(drv *driver.Driver, outPortNum uint) (outPort midi.Out, errVal error) {
	outs, err := drv.Outs()
	if err != nil {
		return nil, err
	}
	if int(outPortNum) > len(outs) {
		return nil, fmt.Errorf("unknown port number: %d", outPortNum+1)
	}
	out := outs[outPortNum]
	if err := out.Open(); err != nil {
		return nil, err
	}
	return out, nil
}

type MidiReaderWriter struct {
	Reader    *reader.Reader
	Writer    *writer.Writer
	In        midi.In
	Out       midi.Out
	closeFunc func() error
}

func NewMidiReaderWriter(inPort, outPort uint, msgReadFunction func(pos *reader.Position, msg midi.Message)) (readerWriter *MidiReaderWriter, errVal error) {
	rw := &MidiReaderWriter{}
	in, out, closeFunc, err := GetMidiPorts(inPort, outPort)
	if err != nil {
		return nil, err
	}
	defer func() {
		if errVal != nil {
			closeFunc()
		}
	}()
	rw.closeFunc = closeFunc
	rw.In = in
	rw.Out = out

	rw.Reader = reader.New(
		reader.NoLogger(),
		reader.Each(msgReadFunction),
	)
	if err := rw.Reader.ListenTo(in); err != nil {
		return nil, err
	}
	rw.Writer = writer.New(out)

	return rw, nil
}

func (rw *MidiReaderWriter) Close() error {
	return rw.closeFunc()
}

func (rw *MidiReaderWriter) ControlChange(channel, controller, value uint8) error {
	rw.Writer.SetChannel(channel)
	return writer.ControlChange(rw.Writer, controller, value)
}

func (rw *MidiReaderWriter) SysEx(channel uint8, data []byte) error {
	rw.Writer.SetChannel(channel)
	return writer.SysEx(rw.Writer, data)
}

func (rw *MidiReaderWriter) NoteOn(channel, key, velocity uint8) error {
	rw.Writer.SetChannel(channel)
	return writer.NoteOn(rw.Writer, key, velocity)
}

func (rw *MidiReaderWriter) NoteOff(channel, key uint8) error {
	rw.Writer.SetChannel(channel)
	return writer.NoteOff(rw.Writer, key)
}

func (rw *MidiReaderWriter) PrintPorts() {
	// add one for zero indexing
	fmt.Printf("MIDI in port:  %d (%s)\n", rw.In.Number()+1, rw.In.String())
	fmt.Printf("MIDI out port: %d (%s)\n", rw.Out.Number()+1, rw.Out.String())
}
