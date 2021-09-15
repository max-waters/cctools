package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"mvw.org/cctools/cctools"
	"mvw.org/cctools/nd2"
	"mvw.org/cctools/util"
)

const CommandList = "list"
const CommandLog = "log"
const CommandListen = "listen"
const CommandSend = "send"
const CommandNordAcr = "nord-lead-acr"
const CommandNd2Decode = "nd2-decode"
const CommandNd2DecodeTest = "nd2-test"

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
	case CommandNordAcr:
		runNordLeadAcr()
	case CommandLog:
		runMidiLogger()
	case CommandList:
		listPorts()
	case CommandNd2Decode:
		runNd2Decoder()
	case CommandNd2DecodeTest:
		runNd2DecoderTest()
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

	cclv := cctools.NewControlChangeListenerView(uint(*port), uint8(*channel), *outputfile, *timestamp)
	if err := cclv.Start(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runControlChangeSender() {
	port := flag.Uint("p", 0, "The port to send to")
	channel := flag.Uint("c", 0, "The channel to send to")
	inputfile := flag.String("f", "", "Input file name")
	flag.Parse()

	if err := cctools.SendControlChangeData(uint(*port), uint8(*channel), *inputfile); err != nil {
		os.Exit(1)
	}
}

func runMidiLogger() {
	port := flag.Uint("p", 0, "The port to listen to")
	flag.Parse()

	midiLogger := util.NewMidiLogger(uint(*port))
	CallOnShutdownSignal(midiLogger.Stop)
	if err := midiLogger.Start(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runNordLeadAcr() {
	port := flag.Uint("p", 0, "The port to listen to")
	slot := flag.Uint("s", 0, "The slot to request")
	globalChan := flag.Uint("g", 0, "The global midi channel")
	flag.Parse()

	if err := cctools.SendAllControllerRequest(uint(*port), uint8(*slot), uint8(*globalChan)); err != nil {
		os.Exit(1)
	}
}

func runNd2Decoder() {
	inPort := flag.Uint("i", 0, "The port to listen to")
	outPort := flag.Uint("o", 0, "The port to send to")
	outChan := flag.Uint("c", 8, "The channel for the first voice")
	flag.Parse()

	nd2Decoder, err := nd2.NewNd2Decoder(uint(*inPort), uint(*outPort), uint8(*outChan))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	CallOnShutdownSignal(nd2Decoder.Stop)
	if err := nd2Decoder.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runNd2DecoderTest() {
	inPort := flag.Uint("i", 0, "The port to listen to")
	outPort := flag.Uint("o", 0, "The port to send to")
	outChan := flag.Uint("c", 8, "The channel for the first voice")
	flag.Parse()

	nd2Decoder, err := nd2.NewNd2Decoder(uint(*inPort), uint(*outPort), uint8(*outChan))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	CallOnShutdownSignal(nd2Decoder.Stop)
	if err := nd2Decoder.Test(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func listPorts() {
	if err := util.ListPorts(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func CallOnShutdownSignal(f func()) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sigChan
		f()
	}()
}
