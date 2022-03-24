package nmg2

import (
	"time"

	"github.com/pkg/errors"
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/channel"
	"gitlab.com/gomidi/midi/reader"
	"mvw.org/cctools/util"
)

const VarChangeController uint8 = 70
const AcrWaitTime = time.Millisecond * 500

// NB not the same as the voice's channel
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

func (conf *NmG2ConnectionConfig) setVoiceMidiChan() error {
	channel, ok := conf.VoiceChannelMap[conf.Voice]
	if !ok {
		return errors.Errorf("no channel configured for voice '%s'", conf.Voice)
	}
	conf.voiceMidiChan = channel
	return nil
}

type NmG2Connection struct {
	Config       *NmG2ConnectionConfig
	readerWriter *util.MidiReaderWriter
	responseChan chan *channel.ControlChange
	shutdownChan chan interface{}
}

func NewNmG2Connection(conf *NmG2ConnectionConfig) (c *NmG2Connection, errVal error) {
	if err := conf.setVoiceMidiChan(); err != nil {
		return nil, err
	}

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
	if err := conn.readerWriter.SysEx(conn.Config.voiceMidiChan, acrSysEx); err != nil {
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
			if cvMsg.Channel() == conn.Config.voiceMidiChan {
				controllerValues = append(controllerValues, &util.ControllerValue{
					Controller: cvMsg.Controller(),
					Value:      cvMsg.Value(),
				})
			}
		}
	}
}

func (conn *NmG2Connection) SetVariation(v uint8) error {
	return conn.SendControlChange(70, v*16) // 0-15, 16-31 etc
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
		filteredCvs := []*util.ControllerValue{}
		for _, cv := range cvs {
			// ignore the variation change controller
			if cv.Controller != VarChangeController {
				filteredCvs = append(filteredCvs, cv)
			}
		}

		variations[v] = filteredCvs
	}
	return variations, nil
}

func (conn *NmG2Connection) ListenForControlChanges(f func(*channel.ControlChange) error) error {
	for {
		select {
		case <-conn.shutdownChan:
			return nil
		case cvMsg := <-conn.responseChan:
			if cvMsg.Channel() == conn.Config.voiceMidiChan {
				if err := f(cvMsg); err != nil {
					return err
				}
			}
		}
	}
}

func (conn *NmG2Connection) Close() {
	conn.shutdownChan <- nil
}
