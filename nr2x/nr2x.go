package nr2x

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/channel"
	"gitlab.com/gomidi/midi/reader"
	"mvw.org/cctools/util"
)

const ConnectionMaxWaitTime = time.Second * 5
const NumNr2xControllers = 42

type Nr2xConnection struct {
	Config       *Nr2xConnectionConfig
	readerWriter *util.MidiReaderWriter
	responseChan chan *channel.ControlChange
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
		responseChan: make(chan *channel.ControlChange, NumNr2xControllers),
		shutdownChan: make(chan interface{}, 1),
	}

	rw, err := util.NewMidiReaderWriter(conf.InPort, conf.OutPort, func(pos *reader.Position, msg midi.Message) {
		if ccMsg, ok := msg.(channel.ControlChange); ok {
			conn.responseChan <- &ccMsg
		}
	})
	if err != nil {
		return nil, err
	}
	conn.readerWriter = rw

	conn.readerWriter.PrintPorts()

	return conn, nil
}

func (conn *Nr2xConnection) SendControlChange(controller, value uint8) error {
	if err := conn.readerWriter.ControlChange(conn.Config.voiceMidiChan, controller, value); err != nil {
		return errors.Wrap(err, "error sending control change message")
	}
	return nil
}

func (conn *Nr2xConnection) GetControllerValues() ([]*util.ControllerValue, error) {
	acrSysEx := []byte{51, conn.Config.GlobalMidiChan, 4, 28, conn.Config.voiceMidiChan}
	if err := conn.readerWriter.SysEx(conn.Config.voiceMidiChan, acrSysEx); err != nil {
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

func (conn *Nr2xConnection) Close() {
	conn.shutdownChan <- nil
}

func GetProgram(conf *Nr2xConnectionConfig, filename string) error {
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
	fmt.Printf("Saved NR2X voice %s to %s\n", conf.Voice, filename)
	return nil
}

func SetProgram(conf *Nr2xConnectionConfig, filename string) error {
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
		time.Sleep(50 * time.Millisecond)
	}

	fmt.Printf("Sent program %s to NR2X voice %s\n", filename, conf.Voice)
	return nil
}
