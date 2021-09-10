package main

import (
	"flag"
	"fmt"
	"os"

	"mvw.org/cctools/cctools"
	// when using portmidi, replace the line above with
	// driver gitlab.com/gomidi/portmididrv
)

const CommandListen = "listen"
const CommandSend = "send"
const CommandList = "list"

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

func runPortLister() {

}
