package nd2

import (
	"fmt"
	"math/rand"

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
	fmt.Printf("Sent voice %d in program %s to ND2\n", voice+1, filename)
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
	fmt.Printf("Copied voice %d to voice %d in ND2\n", from+1, to+1)
	return nil
}

func SetRandomVoice(conf *Nd2ConnectionConfig, voice uint8, incLevel, incPan, incEcho bool) error {
	conn, err := NewNd2Connection(conf)
	if err != nil {
		return err
	}
	defer conn.Close()

	for _, c := range Nd2Controllers {
		if c == ControllerLevel && !incLevel {
			continue
		}
		if c == ControllerPan && !incPan {
			continue
		}
		if (c == EchoBbmController[0] || c == EchoBbmController[1]) && !incEcho {
			continue
		}

		if err := conn.SendControlChange(voice, c, uint8(rand.Intn(127))); err != nil {
			return err
		}
	}

	fmt.Printf("Sent random program to ND2 voice %d\n", voice+1)
	return nil
}
