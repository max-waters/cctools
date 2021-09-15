package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"mvw.org/cctools/cctools"
	"mvw.org/cctools/nd2"
)

const CommandListen = "listen"
const CommandSend = "send"
const CommandNordAcr = "nord-lead-acr"
const CommandNordDrumHack = "nd2-decode"
const CommandNordDrumHackTest = "nd2-decode-test"
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
	case CommandNordAcr:
		runNordLeadAcr()
	case CommandLog:
		runMidiLogger()
	case CommandNordDrumHack:
		runNd2Decoder()
	case CommandNordDrumHackTest:
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

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	midiLogger := cctools.NewMidiLogger(uint(*port))
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

	if err := cctools.SendAllControllerRequest(uint(*port), uint8(*slot), uint8(*globalChan)); err != nil {
		os.Exit(1)
	}
}

func runNd2Decoder() {
	inPort := flag.Uint("ip", 0, "The port to listen to")
	outPort := flag.Uint("op", 0, "The port to send to")
	outChan := flag.Uint("c", 0, "The channel to send to")
	flag.Parse()

	nd2Decoder, err := nd2.NewNd2Decoder(uint(*inPort), uint(*outPort), uint8(*outChan))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sigChan
		nd2Decoder.Stop()
	}()
	if err := nd2Decoder.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runNd2DecoderTest() {
	inPort := flag.Uint("ip", 0, "The port to listen to")
	outPort := flag.Uint("op", 0, "The port to send to")
	outChan := flag.Uint("c", 0, "The channel for the first voice")
	flag.Parse()

	nd2Decoder, err := nd2.NewNd2Decoder(uint(*inPort), uint(*outPort), uint8(*outChan))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sigChan
		nd2Decoder.Stop()
	}()
	if err := nd2Decoder.Test(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
