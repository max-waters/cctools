package nd2

import (
	"fmt"

	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/channel"
	"gitlab.com/gomidi/midi/reader"
	"mvw.org/cctools/util"
)

const VoiceChangeChannel = 10
const ProgramUpdateChannel = 11

type NmG2Controller struct {
	Nd2Connection  *Nd2Connection
	NmG2Connection *util.MidiReaderWriter
	Nd2Program     map[uint8]map[uint8]uint8
	baseChannel    uint8
	currentVoice   uint8
}

func NewNmG2Connection(nd2inPort, nd2outPort uint, nd2baseChannel uint8, nmG2inPort, nmG2outPort uint) (*NmG2Controller, error) {
	cont := &NmG2Controller{
		baseChannel:  nd2baseChannel,
		currentVoice: 1, // might be wrong until NM2 sends something
	}

	nd2Conn, err := NewNd2Connection(nd2inPort, nd2outPort)
	if err != nil {
		return nil, err
	}
	cont.Nd2Connection = nd2Conn
	if err := cont.GetNd2Program(); err != nil {
		return nil, err
	}

	nmG2Conn, err := util.NewMidiReaderWriter(nmG2inPort, nmG2outPort, cont.ProcessNmG2Msg)
	if err != nil {
		return nil, err
	}
	cont.NmG2Connection = nmG2Conn
	if err := cont.UpdateNmG2(); err != nil {
		return nil, err
	}

	return cont, nil
}

func (cont *NmG2Controller) ProcessNmG2Msg(pos *reader.Position, msg midi.Message) {
	ccMsg, ok := msg.(channel.ControlChange)
	if !ok {
		return
	}

	switch ccMsg.Channel() {
	case VoiceChangeChannel:
		// TODO: set cont.CurrentVoice based on ccMsg.Value()
		if err := cont.UpdateNmG2(); err != nil {
			fmt.Printf("Error updating G2 controller values: %s\n", err)
		}
	case ProgramUpdateChannel:
		if err := cont.GetNd2Program(); err != nil {
			fmt.Printf("Error refreshing ND2 program: %s\n", err)
		}
		if err := cont.UpdateNmG2(); err != nil {
			fmt.Printf("Error updating G2 controller values: %s\n", err)
		}
	default:
		if err := cont.Nd2Connection.SendControlChange(cont.currentVoice+cont.baseChannel, ccMsg.Controller(), ccMsg.Value()); err != nil {
			fmt.Printf("Error sending control change message: %s\n", err)
		}
	}
}

func (cont *NmG2Controller) GetNd2Program() error {
	cont.Nd2Program = map[uint8]map[uint8]uint8{}
	for i := 0; i < 6; i++ {
		cont.Nd2Program[uint8(i)] = map[uint8]uint8{}
	}

	program, err := cont.Nd2Connection.GetProgram()
	if err != nil {
		return err
	}

	voiceControllerValues, err := program.GetVoiceControllerValues()
	if err != nil {
		return err
	}

	for _, voiceControllerValue := range voiceControllerValues {
		cont.Nd2Program[voiceControllerValue.Voice][voiceControllerValue.Controller] = voiceControllerValue.Value
	}

	return nil
}

func (cont *NmG2Controller) UpdateNmG2() error {
	for controller, value := range cont.Nd2Program[cont.currentVoice] {
		if err := cont.NmG2Connection.ControlChange(cont.currentVoice+cont.baseChannel, controller, value); err != nil {
			return err
		}
	}
	return nil
}

func (cont *NmG2Controller) Close() error {
	if err := cont.Nd2Connection.Close(); err != nil {
		return err
	}
	return cont.NmG2Connection.Close()
}
