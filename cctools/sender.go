package cctools

import (
	"fmt"

	"gitlab.com/gomidi/midi/writer"
)

func SendControlChangeData(port, channel uint8, filename string) error {
	controllerValueMap, err := loadControllerValueMap(filename)
	if err != nil {
		fmt.Printf("Cannot load control change data from file '%s': %s\n", filename, err)
		return err
	}

	out, closeFunc, err := getMidiOutPort(port)
	if err != nil {
		fmt.Printf("Error opening MIDI port: %s\n", err)
		return err
	}
	defer closeFunc()

	w := writer.New(out)
	w.SetChannel(channel)

	fmt.Printf("Sending to channel %d on port %d (%s)\n", channel, out.Number(), out.String())
	var i uint8
	for i = 0; i < 128; i++ {
		if v, ok := controllerValueMap[i]; ok {
			fmt.Printf("%s\n", formatControllerValuePair(i, &v))
			if err := writer.ControlChange(w, i, v); err != nil {
				fmt.Printf("Error sending control change data: %s\n", err)
				return err
			}
		}
	}
	fmt.Println("Done")
	return nil
}
