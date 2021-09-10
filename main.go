package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"mvw.org/cctools/cctools"
	// when using portmidi, replace the line above with
	// driver gitlab.com/gomidi/portmididrv
)

const CommandListen = "listen"
const CommandSend = "send"
const CommandList = "list"
const CommandNordAcr = "nord-lead-acr"
const CommandLog = "log"

func main() {
	if len(os.Args) <= 1 {
		fmt.Println("No command supplied")
	}
	command := os.Args[1]
	os.Args = os.Args[1:]

	switch command {
	case CommandListen:
		runControlChangeListener()
	case CommandSend:
		runControlChangeSender()
	case CommandList:
		runPortLister()
	case CommandNordAcr:
		runNordLeadAcr()
	case CommandLog:
		runMidiLogger()
	default:
		fmt.Printf("Unknown command: '%s'\n", command)
	}
}

func runControlChangeListener() {
	port := flag.Uint("p", 0, "The port to listen to")
	channel := flag.Uint("c", 0, "The channel to listen to")
	outputfile := flag.String("f", "", "Output file name")
	timestamp := flag.Bool("t", false, "Append timestamp to filename")
	flag.Parse()

	cclv := cctools.NewControlChangeListenerView(uint8(*port), uint8(*channel), *outputfile, *timestamp)
	if err := cclv.Start(); err != nil {
		os.Exit(1)
	}
}

func runControlChangeSender() {
	port := flag.Uint("p", 0, "The port to send to")
	channel := flag.Uint("c", 0, "The channel to send to")
	inputfile := flag.String("f", "", "Input file name")
	flag.Parse()

	if err := cctools.SendControlChangeData(uint8(*port), uint8(*channel), *inputfile); err != nil {
		os.Exit(1)
	}
}

func runMidiLogger() {
	port := flag.Uint("p", 0, "The port to listen to")
	flag.Parse()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	midiLogger := cctools.NewMidiLogger(uint8(*port))
	go func() {
		<-sigChan
		midiLogger.Stop()
	}()

	if err := midiLogger.Start(); err != nil {
		os.Exit(1)
	}
}

func runNordLeadAcr() {
	port := flag.Uint("p", 0, "The port to listen to")
	slot := flag.Uint("s", 0, "The slot to request")
	globalChan := flag.Uint("g", 0, "The global midi channel")
	flag.Parse()

	if err := cctools.SendAllControllerRequest(uint8(*port), uint8(*slot), uint8(*globalChan)); err != nil {
		os.Exit(1)
	}
}

func runPortLister() {

}
