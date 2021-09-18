package nr2x

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/channel"
	"gitlab.com/gomidi/midi/reader"
	"gitlab.com/gomidi/midi/writer"
	"mvw.org/cctools/util"
)

const ConnectionMaxWaitTime = time.Second * 5
const NumNr2xControllers = 42

type Nr2xConnection struct {
	globalChannel uint8
	baseChannel   uint8
	reader        *reader.Reader
	writer        *writer.Writer
	closeFunc     func() error
	responseChan  chan *channel.ControlChange
	shutdownChan  chan interface{}
}

func NewNr2xConnection(inPort, outPort uint, globalChannel, baseChannel uint8) (c *Nr2xConnection, errVal error) {
	conn := &Nr2xConnection{
		globalChannel: globalChannel,
		baseChannel:   baseChannel,
		responseChan:  make(chan *channel.ControlChange, NumNr2xControllers),
		shutdownChan:  make(chan interface{}, 1),
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
			if ccMsg, ok := msg.(channel.ControlChange); ok {
				conn.responseChan <- &ccMsg
			}
		}),
	)
	conn.reader.ListenTo(in)
	conn.writer = writer.New(out)

	fmt.Printf("MIDI in port:  %d (%s)\n", in.Number(), in.String())
	fmt.Printf("MIDI out port: %d (%s)\n", in.Number(), in.String())

	return conn, nil
}

func (conn *Nr2xConnection) SendControlChange(voice, controller, value uint8) error {
	conn.writer.SetChannel(conn.baseChannel + voice)
	if err := writer.ControlChange(conn.writer, controller, value); err != nil {
		return errors.Wrap(err, "error sending control change message")
	}
	return nil
}

func (conn *Nr2xConnection) GetControllerValues(voice uint8) ([]*util.ControllerValue, error) {
	acrSysEx := []byte{51, conn.globalChannel, 4, 28, voice}
	if err := writer.SysEx(conn.writer, acrSysEx); err != nil {
		return nil, errors.Wrap(err, "error sending all controllers request")
	}

	controllerValues := make([]*util.ControllerValue, NumNr2xControllers)
	for i := 0; i < NumNr2xControllers; i++ {
		cvMsg, err := conn.waitForControllerValueMsg()
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

func (conn *Nr2xConnection) waitForControllerValueMsg() (*channel.ControlChange, error) {
	select {
	case <-conn.shutdownChan:
		return nil, errors.New("cancelled")
	case <-time.After(ConnectionMaxWaitTime):
		return nil, errors.New("all controller request timed out")
	case sysExMsg := <-conn.responseChan:
		return sysExMsg, nil
	}
}

func (conn *Nr2xConnection) Close() error {
	conn.shutdownChan <- nil
	return conn.closeFunc()
}

func GetProgram(inPort, outPort uint, globalChannel, baseChannel, voice uint8, filename string) error {
	conn, err := NewNr2xConnection(inPort, outPort, globalChannel, baseChannel)
	if err != nil {
		return err
	}
	defer conn.Close()

	controllerValues, err := conn.GetControllerValues(voice)
	if err != nil {
		return err
	}

	filename, err = util.SaveControllerValues(filename, controllerValues)
	if err != nil {
		return err
	}
	fmt.Printf("Saved NR2X program to %s\n", filename)
	return nil
}

func SetProgram(inPort, outPort uint, globalChannel, baseChannel, voice uint8, filename string) error {
	conn, err := NewNr2xConnection(inPort, outPort, globalChannel, baseChannel)
	if err != nil {
		return err
	}
	defer conn.Close()

	controllerValues, err := util.LoadControllerValues(filename)
	if err != nil {
		return err
	}

	for _, controllerValue := range controllerValues {
		if err := conn.SendControlChange(voice, controllerValue.Controller, controllerValue.Value); err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}

	fmt.Printf("Sent program %s to NR2X\n", filename)
	return nil
}
