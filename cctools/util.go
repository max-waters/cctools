package cctools

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gocarina/gocsv"
	"gitlab.com/gomidi/midi"
	driver "gitlab.com/gomidi/rtmididrv"
)

const fileHeader = "controller,value\n"

func getMidiOutPort(port uint8) (outputPort midi.Out, closeFunc func() error, errVal error) {
	drv, err := driver.New()
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if errVal != nil && drv != nil {
			drv.Close()
		}
	}()

	outs, err := drv.Outs()
	if err != nil {
		return nil, nil, err
	}
	if int(port) > len(outs) {
		return nil, nil, fmt.Errorf("unknown port number: %d", port)
	}

	out := outs[port]
	if err := out.Open(); err != nil {
		return nil, nil, err
	}

	closeFunc = func() error {
		if err := drv.Close(); err != nil {
			return err
		}
		return out.Close()
	}

	return out, closeFunc, nil
}

func getMidiInPort(port uint8) (inputPort midi.In, closeFunc func() error, errVal error) {
	drv, err := driver.New()
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if errVal != nil && drv != nil {
			drv.Close()
		}
	}()

	ins, err := drv.Ins()
	if err != nil {
		return nil, nil, err
	}
	if int(port) > len(ins) {
		return nil, nil, fmt.Errorf("unknown port number: %d", port)
	}

	in := ins[port]
	if err := in.Open(); err != nil {
		return nil, nil, err
	}

	closeFunc = func() error {
		if err := drv.Close(); err != nil {
			return err
		}
		return in.Close()
	}

	return in, closeFunc, nil
}

func printInPorts(ins []midi.In) {
	if len(ins) == 0 {
		fmt.Println("No MIDI in ports found")
		return
	}
	fmt.Println("MIDI in ports:")
	for _, port := range ins {
		fmt.Printf("%v: %s\n", port.Number(), port.String())
	}
}

func loadControllerValueMap(filename string) (map[uint8]uint8, error) {
	type controllerValuePair struct {
		Controller uint8 `csv:"controller"`
		Value      uint8 `csv:"value"`
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	csvRows := []*controllerValuePair{}
	if err := gocsv.UnmarshalFile(f, &csvRows); err != nil {
		return nil, err
	}

	controllerValueMap := map[uint8]uint8{}
	for _, row := range csvRows {
		controllerValueMap[row.Controller] = row.Value
	}

	return controllerValueMap, nil
}

func saveControllerValueMap(filename string, controllerValueMap map[uint8]uint8) error {
	sb := strings.Builder{}
	sb.WriteString(fileHeader)
	var i uint8
	for i = 0; i < 128; i++ {
		if v, ok := controllerValueMap[i]; ok {
			sb.WriteString(fmt.Sprintf("%d,%d\n", i, v))
		}
	}
	return writeFile(filename, sb.String())
}

func writeFile(filename, data string) error {
	// create parent directories
	if err := os.MkdirAll(filepath.Dir(filename), os.ModePerm); err != nil {
		return err
	}

	// create file
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// write
	w := bufio.NewWriter(f)
	if _, err := w.WriteString(data); err != nil {
		return err
	}
	return w.Flush()
}

func formatControllerValuePair(controller uint8, value *uint8) string {
	if value != nil {
		return fmt.Sprintf("%03d:%03d", controller, *value)
	}
	return fmt.Sprintf("%03d:   ", controller)
}

func ToBinaryString(u uint8) string {
	s := ""
	for i := 0; i < 8; i++ {
		s = fmt.Sprintf("%d%s", u%2, s)
		u /= 2
	}
	return s
}
