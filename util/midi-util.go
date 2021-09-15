package util

import (
	"fmt"

	"gitlab.com/gomidi/midi"
	driver "gitlab.com/gomidi/rtmididrv"
)

func GetMidiPorts(inPortNum, outPortNum uint) (inPort midi.In, outPort midi.Out, closeFunc func() error, errVal error) {
	drv, err := driver.New()
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() {
		if errVal != nil {
			drv.Close()
		}
	}()

	in, err := openMidiInPort(drv, inPortNum)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() {
		if errVal != nil {
			in.Close()
		}
	}()

	out, err := openMidiOutPort(drv, outPortNum)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() {
		if errVal != nil {
			out.Close()
		}
	}()

	return in, out, func() error {
		if err := in.Close(); err != nil {
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return drv.Close()
	}, nil
}

func GetMidiOutPort(port uint) (outputPort midi.Out, closeFunc func() error, errVal error) {
	drv, err := driver.New()
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if errVal != nil {
			drv.Close()
		}
	}()

	out, err := openMidiOutPort(drv, port)
	if err != nil {
		return nil, nil, err
	}

	closeFunc = func() error {
		if err := out.Close(); err != nil {
			return err
		}
		return drv.Close()
	}

	return out, closeFunc, nil
}

func GetMidiInPort(port uint) (inputPort midi.In, closeFunc func() error, errVal error) {
	drv, err := driver.New()
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if errVal != nil {
			drv.Close()
		}
	}()

	in, err := openMidiInPort(drv, port)
	if err != nil {
		return nil, nil, err
	}

	closeFunc = func() error {
		if err := in.Close(); err != nil {
			return err
		}
		return drv.Close()
	}
	return in, closeFunc, nil
}

func openMidiInPort(drv *driver.Driver, inPortNum uint) (inPort midi.In, errVal error) {
	ins, err := drv.Ins()
	if err != nil {
		return nil, err
	}
	if int(inPortNum) > len(ins) {
		return nil, fmt.Errorf("unknown port number: %d", inPortNum)
	}
	in := ins[inPortNum]
	if err := in.Open(); err != nil {
		return nil, err
	}
	return in, nil
}

func openMidiOutPort(drv *driver.Driver, outPortNum uint) (outPort midi.Out, errVal error) {
	outs, err := drv.Outs()
	if err != nil {
		return nil, err
	}
	if int(outPortNum) > len(outs) {
		return nil, fmt.Errorf("unknown port number: %d", outPortNum)
	}
	out := outs[outPortNum]
	if err := out.Open(); err != nil {
		return nil, err
	}
	return out, nil
}
