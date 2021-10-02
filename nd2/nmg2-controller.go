package nd2

import (
	"fmt"

	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/channel"
	"gitlab.com/gomidi/midi/reader"
	"mvw.org/cctools/util"
)

var VoiceChangeKeys = map[uint8]uint8{72: 0, 74: 1, 76: 2, 77: 3, 79: 4, 81: 5}

const BaseVoiceChangeNote1 = 72
const BaseVoiceChangeNote2 = 74
const BaseVoiceChangeNote3 = 76
const ProgramUpdateChannel = 0 // TBA

type NmG2Controller struct {
	Nd2Connection  *Nd2Connection
	NmG2Connection *util.MidiReaderWriter
	Nd2Program     map[uint8]map[uint8]uint8
	currentVoice   uint8
	shutdownChan   chan interface{}
}

type NmG2Config struct {
	InPort       uint  `yaml:"in_port"`
	OutPort      uint  `yaml:"out_port"`
	BaseMidiChan uint8 `yaml:"base_midi_channel"`
}

func NewNmG2Connection(nd2Config *Nd2ConnectionConfig, nmG2Config *NmG2Config) (*NmG2Controller, error) {
	cont := &NmG2Controller{
		currentVoice: 0, // might be wrong until NM2 sends something
		shutdownChan: make(chan interface{}, 1),
	}

	nd2Conn, err := NewNd2Connection(nd2Config)
	if err != nil {
		return nil, err
	}
	cont.Nd2Connection = nd2Conn
	if err := cont.GetNd2Program(); err != nil {
		return nil, err
	}

	nmG2Conn, err := util.NewMidiReaderWriter(nmG2Config.InPort, nmG2Config.OutPort, cont.ProcessNmG2Msg)
	if err != nil {
		return nil, err
	}
	cont.NmG2Connection = nmG2Conn

	fmt.Println("Listening to G2")
	fmt.Printf("MIDI in port:  %d (%s)\n", cont.NmG2Connection.In.Number(), cont.NmG2Connection.In.String())
	fmt.Printf("MIDI out port: %d (%s)\n", cont.NmG2Connection.Out.Number(), cont.NmG2Connection.Out.String())

	if err := cont.UpdateNmG2(); err != nil {
		return nil, err
	}

	return cont, nil
}

func (cont *NmG2Controller) Run() (errVal error) {
	defer func() {
		if err := cont.Nd2Connection.Close(); err != nil {
			errVal = err
		}
		if err := cont.NmG2Connection.Close(); err != nil {
			errVal = err
		}
	}()
	<-cont.shutdownChan
	return nil
}

func (cont *NmG2Controller) ProcessNmG2Msg(pos *reader.Position, msg midi.Message) {
	if ccMsg, ok := msg.(channel.ControlChange); ok {
		// pull program from ND2
		if ccMsg.Channel() == ProgramUpdateChannel {
			fmt.Printf("Getting ND2 program")
			if err := cont.GetNd2Program(); err != nil {
				fmt.Printf("Error refreshing ND2 program: %s\n", err)
			}
			if err := cont.UpdateNmG2(); err != nil {
				fmt.Printf("Error updating G2 controller values: %s\n", err)
			}
			return
		}

		// forward to correct channel/voice
		cont.Nd2Connection.SendControlChange(cont.currentVoice, ccMsg.Controller(), ccMsg.Value())
		return
	}

	// change voice
	if noMsg, ok := msg.(channel.NoteOn); ok {
		if voice, ok := VoiceChangeKeys[noMsg.Key()]; ok {
			fmt.Printf("Updating G2 with ND2 voice %d\n", voice)
			cont.currentVoice = voice
			if err := cont.Nd2Connection.SendVoiceFocusChange(cont.currentVoice); err != nil {
				fmt.Printf("Error changing ND2 voice focus: %s\n", err)
			}
			if err := cont.UpdateNmG2(); err != nil {
				fmt.Printf("Error updating G2 controller values: %s\n", err)
			}
			return
		}
	}

	// note off for a voice change, ignore as note on wasn't sent
	if noMsg, ok := msg.(channel.NoteOff); ok {
		if _, ok := VoiceChangeKeys[noMsg.Key()]; ok {
			return
		}
	}

	// forward to ND2
	if err := cont.Nd2Connection.readerWriter.Writer.Write(msg); err != nil {
		fmt.Printf("Error forwarding MIDI msg to ND2: %s\n", err)
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
		if err := cont.NmG2Connection.ControlChange(cont.currentVoice, controller, value); err != nil {
			return err
		}
	}
	return nil
}

func (cont *NmG2Controller) Stop() {
	cont.shutdownChan <- nil
}
