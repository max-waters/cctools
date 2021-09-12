package cctools

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/eiannone/keyboard"
	"gitlab.com/gomidi/midi"
	"gitlab.com/gomidi/midi/midimessage/channel"
	"gitlab.com/gomidi/midi/midimessage/sysex"
	"gitlab.com/gomidi/midi/reader"
)

type ControlChangeListener struct {
	port       uint8
	channel    uint8
	in         midi.In
	reader     *reader.Reader
	notifyFunc func(controller, value uint8)
	closeFunc  func() error
}

func NewControlChangeListener(port, channel uint8, notifyFunc func(controller, value uint8)) *ControlChangeListener {
	return &ControlChangeListener{
		port:       port,
		channel:    channel,
		notifyFunc: notifyFunc,
	}
}

func (ccl *ControlChangeListener) Start() error {
	in, closeFunc, err := getMidiInPort(ccl.port)
	if err != nil {
		return err
	}
	ccl.in = in
	ccl.closeFunc = closeFunc

	// to disable logging, pass mid.NoLogger() as option
	ccl.reader = reader.New(
		reader.NoLogger(),
		reader.Each(ccl.processMessage),
	)

	if err := ccl.reader.ListenTo(ccl.in); err != nil {
		return err
	}

	return nil
}

func (ccl *ControlChangeListener) processMessage(pos *reader.Position, msg midi.Message) {
	if ccMsg, ok := msg.(channel.ControlChange); ok {
		if ccMsg.Channel() == ccl.channel {
			ccl.notifyFunc(ccMsg.Controller(), ccMsg.Value())
		}
	}
}

func (ccl *ControlChangeListener) Close() error {
	return ccl.closeFunc()
}

type ControlChangeListenerView struct {
	ccl                *ControlChangeListener
	controllerValueMap map[uint8]uint8
	inputBuffer        string
	lastLog            string
	outputFile         string
	timestamp          bool
}

func NewControlChangeListenerView(port, channel uint8, outputFile string, timestamp bool) *ControlChangeListenerView {
	cclv := &ControlChangeListenerView{
		controllerValueMap: map[uint8]uint8{},
		outputFile:         outputFile,
		timestamp:          timestamp,
	}
	cclv.ccl = NewControlChangeListener(port, channel, cclv.update)
	return cclv
}

func (cclv *ControlChangeListenerView) Start() error {
	if err := cclv.ccl.Start(); err != nil {
		fmt.Printf("Cannot start control change listener: %s\n", err)
		return err
	}
	cclv.print(true)
	return cclv.startInput()
}

func (cclv *ControlChangeListenerView) update(controller, value uint8) {
	cclv.controllerValueMap[controller] = value
	cclv.print(false)
}

func (cclv *ControlChangeListenerView) log(format string, args ...interface{}) {
	cclv.lastLog = fmt.Sprintf(format, args...)
	cclv.print(true)
}

func (cclv *ControlChangeListenerView) print(clearScreen bool) {
	sb := strings.Builder{}

	if clearScreen {
		sb.WriteString("\033[H\033[2J")
	}

	sb.WriteString("\033[0;0H") // move to top
	sb.WriteString(fmt.Sprintf("Listening to channel %d on port %d (%s)\n", cclv.ccl.channel, cclv.ccl.in.Number(), cclv.ccl.in.String()))
	sb.WriteString("---------------------------------------------------------------\n")
	var i uint8
	for i = 0; i < 128; i++ {
		if v, ok := cclv.controllerValueMap[i]; ok {
			sb.WriteString(fmt.Sprintf("%s ", formatControllerValuePair(i, &v)))
		} else {
			sb.WriteString(fmt.Sprintf("%s ", formatControllerValuePair(i, nil)))
		}
		if (i+1)%8 == 0 {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("---------------------------------------------------------------\n")
	sb.WriteString(fmt.Sprintf("%s\n", cclv.lastLog))
	sb.WriteString(cclv.inputBuffer)

	fmt.Print(sb.String())
}

func (cclv *ControlChangeListenerView) startInput() error {
	if err := keyboard.Open(); err != nil {
		return err
	}
	defer keyboard.Close()

	for {
		r, k, err := keyboard.GetSingleKey()
		if err != nil {
			return err
		}
		switch k {
		case keyboard.KeyEnter:
			inputBuffer := cclv.inputBuffer
			cclv.inputBuffer = ""
			switch inputBuffer {
			case "q", "quit":
				cclv.close()
				return nil
			case "s", "save":
				cclv.saveFile()
			case "": //ignore
			default:
				cclv.log("Unknown command: '%s'", inputBuffer)
			}
		case keyboard.KeyBackspace, keyboard.KeyBackspace2:
			if len(cclv.inputBuffer) > 0 {
				cclv.inputBuffer = cclv.inputBuffer[0 : len(cclv.inputBuffer)-1]
				cclv.print(true)
			}
		default:
			cclv.inputBuffer = cclv.inputBuffer + string(r)
			cclv.print(false)
		}
	}
}

func (cclv *ControlChangeListenerView) close() {
	cclv.log("Exiting")
	if err := cclv.ccl.Close(); err != nil {
		cclv.log("Error closing MIDI listener: %s", err)
	}
}

func (cclv *ControlChangeListenerView) formatFileName() string {
	if cclv.timestamp {
		extension := filepath.Ext(cclv.outputFile)
		if len(extension) > 0 { // return eg file-timestamp.csv
			name := strings.TrimSuffix(cclv.outputFile, extension)
			return fmt.Sprintf("%s-%s%s", name, time.Now().Format(time.RFC3339), extension)
		}
		// return file-timestamp
		return fmt.Sprintf("%s-%s", cclv.outputFile, time.Now().Format(time.RFC3339))
	}
	return cclv.outputFile
}

func (cclv *ControlChangeListenerView) saveFile() {
	if cclv.outputFile == "" {
		cclv.log("No output file specified")
		return
	}

	outputFile := cclv.formatFileName()
	cclv.log("Saving to file '%s'", outputFile)

	if err := saveControllerValueMap(outputFile, cclv.controllerValueMap); err != nil {
		cclv.log("Cannot save to file '%s': %s", outputFile, err)
	} else {
		cclv.log("Saved to file %s", outputFile)
	}
}

type MidiLogger struct {
	port         uint8
	shutdownChan chan interface{}
}

func NewMidiLogger(port uint8) *MidiLogger {
	return &MidiLogger{
		port:         port,
		shutdownChan: make(chan interface{}, 1),
	}
}

func (logger *MidiLogger) Start() error {
	in, closeFunc, err := getMidiInPort(logger.port)
	if err != nil {
		return err
	}
	defer closeFunc()

	reader := reader.New(
		reader.NoLogger(),
		reader.Each(func(pos *reader.Position, msg midi.Message) {
			fmt.Println(msg.String())
			if sysExMsg, ok := msg.(sysex.SysEx); ok {
				fmt.Println(sysExMsg.Raw())
			}
		}),
	)

	fmt.Printf("Listening to port %d (%s)\n", in.Number(), in.String())
	reader.ListenTo(in)

	<-logger.shutdownChan

	return nil
}

func (logger *MidiLogger) Stop() {
	logger.shutdownChan <- nil
}
