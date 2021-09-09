package cctools

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/gomidi/midi"
)

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
