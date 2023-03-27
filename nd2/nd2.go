package nd2

import (
	"fmt"

	"mvw.org/cctools/util"
)

func GetProgram(conf *Nd2ConnectionConfig, filename string) error {
	conn, err := NewNd2Connection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	program, err := conn.GetProgram()
	if err != nil {
		return err
	}

	voiceControllerValues, err := program.GetVoiceControllerValues()
	if err != nil {
		return err
	}

	filename, err = util.SaveVoiceControllerValues(filename, voiceControllerValues)
	if err != nil {
		return err
	}

	fmt.Printf("Saved ND2 program to %s\n", filename)
	return nil
}

func SetProgram(conf *Nd2ConnectionConfig, filename string) error {
	conn, err := NewNd2Connection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	values, err := util.LoadVoiceControllerValues(filename)
	if err != nil {
		return err
	}

	for _, value := range values {
		if err := conn.SendControlChange(value.Voice, value.Controller, value.Value); err != nil {
			return err
		}
	}
	fmt.Printf("Sent program %s to ND2\n", filename)
	return nil
}

func SetVoice(conf *Nd2ConnectionConfig, filename string, voice uint8) error {
	conn, err := NewNd2Connection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	values, err := util.LoadVoiceControllerValues(filename)
	if err != nil {
		return err
	}

	for _, value := range values {
		if value.Voice == voice {
			if err := conn.SendControlChange(value.Voice, value.Controller, value.Value); err != nil {
				return err
			}
		}
	}
	fmt.Printf("Sent voice %d in program %s to ND2\n", voice, filename)
	return nil
}

func CopyVoice(conf *Nd2ConnectionConfig, from, to uint8) error {
	conn, err := NewNd2Connection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	program, err := conn.GetProgram()
	if err != nil {
		return err
	}

	voiceControllerValues, err := program.GetVoiceControllerValues()
	if err != nil {
		return err
	}

	for _, value := range voiceControllerValues {
		if value.Voice == from {
			if err := conn.SendControlChange(to, value.Controller, value.Value); err != nil {
				return err
			}
		}
	}
	fmt.Printf("Copied voice %d to voice %d in ND2\n", from, to)
	return nil
}
