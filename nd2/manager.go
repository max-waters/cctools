package nd2

import (
	"fmt"

	"mvw.org/cctools/util"
)

func GetProgram(inPort, outPort uint, baseChannel uint8, filename string) error {
	conn, err := NewNd2Connection(inPort, outPort)
	if err != nil {
		return err
	}
	defer conn.Close()

	program, err := conn.GetProgram()
	if err != nil {
		return err
	}

	voiceControllerValues := []*util.VoiceControllerValue{}
	var voice uint8
	for voice = 0; voice < 6; voice++ {
		for _, controller := range Nd2Controllers {
			sysExValue := program.GetSysExValue(voice, ControllerBitRanges[controller])
			controllerValue := ControllerValueSysexMap[controller][sysExValue]
			voiceControllerValues = append(voiceControllerValues, &util.VoiceControllerValue{
				Voice:      voice,
				Controller: controller,
				Value:      controllerValue,
			})
		}
	}

	filename, err = util.SaveVoiceControllerValues(filename, voiceControllerValues)
	if err != nil {
		return err
	}
	fmt.Printf("Saved ND2 program to %s\n", filename)
	return nil
}

func SetProgram(inPort, outPort uint, baseChannel uint8, filename string) error {
	conn, err := NewNd2Connection(inPort, outPort)
	if err != nil {
		return err
	}
	defer conn.Close()

	values, err := util.LoadVoiceControllerValues(filename)
	if err != nil {
		return err
	}

	for _, value := range values {
		if err := conn.SendControlChange(value.Voice+baseChannel, value.Controller, value.Value); err != nil {
			return err
		}
	}
	fmt.Printf("Sent program %s to ND2\n", filename)
	return nil
}
