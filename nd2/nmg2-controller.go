package nd2

import (
	"fmt"

	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/channel"
	"gitlab.com/gomidi/midi/reader"
	"mvw.org/cctools/util"
)

const Ng2PitchController uint8 = 71
const Ng2OctaveController uint8 = 62

var VoiceChangeKeys = map[uint8]uint8{72: 0, 74: 1, 76: 2, 77: 3, 79: 4, 81: 5}

var ControllerScaleFactors = map[uint8]float32{
	15: 1.165, // filt type
	19: 0.59,  // atk mode
	20: 0.748, // decay type
	24: 1.495, // dist type
	48: 1.396, // punch
}

var ControllerMap = map[uint8]uint8{
	17: 13, 13: 17, // filter res
	18: 12, 12: 18, // noise attack rate
	7: 9, 9: 7, // level
}

func init() {
	for _, controller := range Nd2Controllers {
		if _, ok := ControllerScaleFactors[controller]; !ok {
			ControllerScaleFactors[controller] = 1
		}
		if _, ok := ControllerMap[controller]; !ok {
			ControllerMap[controller] = controller
		}
	}
}

const ProgramUpdateController = 0 // TBA

type NmG2Controller struct {
	Nd2Connection  *Nd2Connection
	NmG2Connection *util.MidiReaderWriter
	NmG2Config     *NmG2Config
	Nd2Program     map[uint8]map[uint8]uint8
	nd2Voice       uint8
	shutdownChan   chan interface{}
}

type NmG2Config struct {
	InPort       uint  `yaml:"in_port"`
	OutPort      uint  `yaml:"out_port"`
	BaseMidiChan uint8 `yaml:"base_midi_channel"`
}

func NewNmG2Connection(nd2Config *Nd2ConnectionConfig, nmG2Config *NmG2Config) (*NmG2Controller, error) {
	cont := &NmG2Controller{
		NmG2Config:   nmG2Config,
		nd2Voice:     0,
		shutdownChan: make(chan interface{}, 1),
	}

	nd2Conn, err := NewNd2Connection(nd2Config)
	if err != nil {
		return nil, err
	}
	cont.Nd2Connection = nd2Conn

	nmG2Conn, err := util.NewMidiReaderWriter(nmG2Config.InPort, nmG2Config.OutPort, cont.ProcessNmG2Msg)
	if err != nil {
		return nil, err
	}
	cont.NmG2Connection = nmG2Conn

	fmt.Println("Listening to G2")
	fmt.Printf("MIDI in port:  %d (%s)\n", cont.NmG2Connection.In.Number(), cont.NmG2Connection.In.String())
	fmt.Printf("MIDI out port: %d (%s)\n", cont.NmG2Connection.Out.Number(), cont.NmG2Connection.Out.String())

	// set to voice 0
	if err := cont.Nd2Connection.SendVoiceFocusChange(0); err != nil {
		return nil, err
	}

	// get ND2 program
	if err := cont.GetNd2Program(); err != nil {
		return nil, err
	}

	// push ND2 program to G2
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
		if ccMsg.Controller() == ProgramUpdateController {
			fmt.Printf("Getting ND2 program")
			// if err := cont.GetNd2Program(); err != nil {
			// 	fmt.Printf("Error refreshing ND2 program: %s\n", err)
			// }
			// if err := cont.UpdateNmG2(); err != nil {
			// 	fmt.Printf("Error updating G2 controller values: %s\n", err)
			// }
			return
		}

		// convert G2 controller/values to ND2
		for _, controllerValue := range cont.Ng2ToNd2(ccMsg.Controller(), ccMsg.Value()) {
			// save to local version of program
			voiceProgram := cont.Nd2Program[cont.nd2Voice]
			if _, ok := voiceProgram[controllerValue.Controller]; ok {
				voiceProgram[controllerValue.Controller] = controllerValue.Value
			} else {
				fmt.Printf("Unknown ND2 controller: %d\n", controllerValue.Controller)
			}

			// forward to correct channel/voice
			if err := cont.Nd2Connection.SendControlChange(cont.nd2Voice, controllerValue.Controller, controllerValue.Value); err != nil {
				fmt.Printf("Error sending control change msg: %s\n", err)
			}
		}

		return
	}

	// change voice if it's a voice key
	if noMsg, ok := msg.(channel.NoteOn); ok {
		if voice, ok := VoiceChangeKeys[noMsg.Key()]; ok && voice != cont.nd2Voice {
			cont.nd2Voice = voice
			if err := cont.Nd2Connection.SendVoiceFocusChange(cont.nd2Voice); err != nil {
				fmt.Printf("Error changing ND2 voice focus: %s\n", err)
			}
			fmt.Printf("Updating G2 with ND2 voice %d\n", voice)
			if err := cont.UpdateNmG2(); err != nil {
				fmt.Printf("Error updating G2 controller values: %s\n", err)
			}
		}
	}

	// forward to ND2
	if err := cont.Nd2Connection.readerWriter.Writer.Write(msg); err != nil {
		fmt.Printf("Error forwarding MIDI msg to ND2: %s\n", err)
	}
}

func (cont *NmG2Controller) Nd2ToNg2(controller, value uint8) []*util.ControllerValue {
	if controller == TonePitchController[0] { // semitone
		return cont.Nd2PitchToNg2Pitch(cont.Nd2Program[cont.nd2Voice][TonePitchController[1]], value)
	} else if controller == TonePitchController[1] { // tone
		return cont.Nd2PitchToNg2Pitch(value, cont.Nd2Program[cont.nd2Voice][TonePitchController[0]])
	}

	return []*util.ControllerValue{
		{Controller: ControllerMap[controller], Value: uint8(float32(value) / ControllerScaleFactors[controller])},
	}
}

func (cont *NmG2Controller) Nd2PitchToNg2Pitch(pitch, semitone uint8) []*util.ControllerValue {
	ng2Pitch := pitch / 2
	if semitone >= 64 {
		ng2Pitch++
	}

	octave := 0
	if pitch >= 64 {
		octave = 64
	}

	return []*util.ControllerValue{
		{Controller: Ng2PitchController, Value: ng2Pitch},
		{Controller: Ng2OctaveController, Value: uint8(octave)}}
}

func (cont *NmG2Controller) Ng2ToNd2(controller, value uint8) []*util.ControllerValue {
	if controller == Ng2PitchController || controller == Ng2OctaveController {
		// get current pitch and
		ng2Pitch := cont.Nd2PitchToNg2Pitch(cont.Nd2Program[cont.nd2Voice][TonePitchController[1]], cont.Nd2Program[cont.nd2Voice][TonePitchController[0]])
		if controller == Ng2PitchController {
			return cont.Ng2PitchToNd2Pitch(value, ng2Pitch[1].Value)
		} else if controller == Ng2OctaveController {
			return cont.Ng2PitchToNd2Pitch(ng2Pitch[0].Value, value)
		}
	}

	nd2Controller := ControllerMap[controller]
	return []*util.ControllerValue{
		{Controller: nd2Controller, Value: uint8(float32(value) * ControllerScaleFactors[nd2Controller])},
	}
}

func (cont *NmG2Controller) Ng2PitchToNd2Pitch(pitch, octave uint8) []*util.ControllerValue {
	nd2Pitch := pitch / 2
	if octave >= 64 {
		nd2Pitch += 64
	}

	return []*util.ControllerValue{
		{Controller: TonePitchController[0], Value: (pitch % 2) * 64}, // semitone
		{Controller: TonePitchController[1], Value: nd2Pitch}}         // tone
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
	for controller, value := range cont.Nd2Program[cont.nd2Voice] {
		for _, controllerValue := range cont.Nd2ToNg2(controller, value) { // map to ng2 values
			if err := cont.NmG2Connection.ControlChange(cont.NmG2Config.BaseMidiChan, controllerValue.Controller, controllerValue.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

func (cont *NmG2Controller) Stop() {
	cont.shutdownChan <- nil
}
