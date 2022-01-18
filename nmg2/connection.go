package nmg2

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/channel"
	"gitlab.com/gomidi/midi/reader"
	"mvw.org/cctools/util"
)

const AcrWaitTime = time.Millisecond * 500

var VoiceIndexMap map[string]uint8 = map[string]uint8{
	"A": 0, "B": 1, "C": 2, "D": 3,
}

type NmG2ConnectionConfig struct {
	InPort          uint             `yaml:"in_port"`
	OutPort         uint             `yaml:"out_port"`
	GlobalMidiChan  uint8            `yaml:"global_midi_channel"`
	Voice           string           `yaml:"voice"`
	voiceMidiChan   uint8            `yaml:"-"`
	VoiceChannelMap map[string]uint8 `yaml:"voice_channel_map"`
}

type NmG2Connection struct {
	Config       *NmG2ConnectionConfig
	readerWriter *util.MidiReaderWriter
	responseChan chan *channel.ControlChange
	shutdownChan chan interface{}
}

func NewNmG2Connection(conf *NmG2ConnectionConfig) (c *NmG2Connection, errVal error) {
	conn := &NmG2Connection{
		Config:       conf,
		responseChan: make(chan *channel.ControlChange, 128), // won't be more that 128?
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

	return conn, nil
}

func (conn *NmG2Connection) SendControlChange(controller, value uint8) error {
	if err := conn.readerWriter.ControlChange(conn.Config.voiceMidiChan, controller, value); err != nil {
		return errors.Wrap(err, "error sending control change message")
	}
	return nil
}

func (conn *NmG2Connection) GetControllerValues() ([]*util.ControllerValue, error) {
	acrSysEx := []byte{51, 127, 10, 64, VoiceIndexMap[conn.Config.Voice]}
	if err := conn.readerWriter.SysEx(acrSysEx); err != nil {
		return nil, errors.Wrap(err, "error sending all controllers request")
	}

	controllerValues := []*util.ControllerValue{}
	for {
		select {
		case <-conn.shutdownChan:
			return nil, errors.New("cancelled")
		case <-time.After(AcrWaitTime):
			return controllerValues, nil
		case cvMsg := <-conn.responseChan:
			controllerValues = append(controllerValues, &util.ControllerValue{
				Controller: cvMsg.Controller(),
				Value:      cvMsg.Value(),
			})
		}
	}
}

func (conn *NmG2Connection) SetVariation(v uint8) error {
	return conn.SendControlChange(70, v)
}

func (conn *NmG2Connection) GetVariations() ([][]*util.ControllerValue, error) {
	variations := make([][]*util.ControllerValue, 8)
	var v uint8
	for v = 0; v < 8; v++ {
		if err := conn.SetVariation(v); err != nil {
			return nil, err
		}
		cvs, err := conn.GetControllerValues()
		if err != nil {
			return nil, err
		}
		variations[v] = cvs
	}
	return variations, nil
}

func (conn *NmG2Connection) ListenForControlChanges(f func(*channel.ControlChange)) {
	for {
		select {
		case <-conn.shutdownChan:
			return
		case cvMsg := <-conn.responseChan:
			f(cvMsg)
		}
	}
}

func (conn *NmG2Connection) Close() {
	conn.shutdownChan <- nil
}

func GetVariations(conf *NmG2ConnectionConfig, filename string) error {
	conn, err := NewNmG2Connection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	variations, err := conn.GetVariations()
	if err != nil {
		return err
	}

	vlist := []*util.VoiceControllerValue{}
	var v uint8
	for v = 0; v < 8; v++ {
		for _, vc := range variations[v] {
			vlist = append(vlist, &util.VoiceControllerValue{Voice: v, Controller: vc.Controller, Value: vc.Value})
		}
	}

	filename, err = util.SaveVoiceControllerValues(filename, vlist)
	if err != nil {
		return err
	}
	fmt.Printf("Saved NmG2 variations for voice %s to %s\n", conf.Voice, filename)
	return nil
}
