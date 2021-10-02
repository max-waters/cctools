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
