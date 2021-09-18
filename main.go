package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "embed"

	"gopkg.in/yaml.v2"
	"mvw.org/cctools/nd2"
	"mvw.org/cctools/nr2x"
	"mvw.org/cctools/util"
)

type DefaultFlags struct {
	Nr2x struct {
		InPort         uint  `yaml:"in_port"`
		OutPort        uint  `yaml:"out_port"`
		GlobalMidiChan uint8 `yaml:"global_midi_channel"`
		BaseMidiChan   uint8 `yaml:"base_midi_channel"`
		Voice          uint8 `yaml:"voice"`
	} `yaml:"nr2x"`
	Nd2 struct {
		InPort       uint  `yaml:"in_port"`
		OutPort      uint  `yaml:"out_port"`
		BaseMidiChan uint8 `yaml:"base_midi_channel"`
	} `yaml:"nd2"`
}

//go:embed defaults.yaml
var DefaultsFileBytes []byte
var Defaults *DefaultFlags

const CommandList = "list"
const CommandLog = "log"
const CommandListen = "listen"
const CommandNr2xSet = "nr2x-set"
const CommandNr2xGet = "nr2x-get"
const CommandNd2Set = "nd2-set"
const CommandNd2Get = "nd2-get"
const CommandNd2Decode = "nd2-decode"
const CommandNd2Test = "nd2-test"

func init() {
	Defaults = &DefaultFlags{}
	if err := yaml.Unmarshal(DefaultsFileBytes, Defaults); err != nil {
		panic(err)
	}
}

func main() {
	if len(os.Args) <= 1 {
		fmt.Printf("No command supplied. ")
		PrintCommandsAndExit()
	}

	command := os.Args[1]
	os.Args = os.Args[1:]

	switch command {
	case CommandList:
		ListPorts()
	case CommandLog:
		RunMidiLogger()
	case CommandListen:
		RunControlChangeListener()
	case CommandNr2xGet:
		RunNr2xGet()
	case CommandNr2xSet:
		RunNr2xSet()
	case CommandNd2Get:
		RunNd2Get()
	case CommandNd2Set:
		RunNd2Set()
	case CommandNd2Decode:
		RunNd2Decoder()
	case CommandNd2Test:
		RunNd2Test()
	default:
		fmt.Printf("Unknown command: '%s'. ", command)
		PrintCommandsAndExit()
	}
}

func PrintCommandsAndExit() {
	fmt.Printf("Options:\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n",
		CommandList, CommandLog, CommandListen, CommandNr2xGet, CommandNr2xSet,
		CommandNd2Get, CommandNd2Set, CommandNd2Decode, CommandNd2Test)
	os.Exit(1)
}

func ListPorts() {
	ExitOnErr(util.ListPorts())
}

func RunMidiLogger() {
	port := flag.Uint("p", 0, "The port to listen to")
	flag.Parse()

	midiLogger := util.NewMidiLogger(uint(*port))
	CallOnShutdownSignal(midiLogger.Stop)
	ExitOnErr(midiLogger.Start())
}

func RunControlChangeListener() {
	port := flag.Uint("p", 0, "The port to listen to")
	channel := flag.Uint("c", 0, "The channel to listen to")
	outputfile := flag.String("f", "", "Output file name")
	flag.Parse()

	cclv := util.NewControlChangeListenerView(uint(*port), uint8(*channel), *outputfile)
	ExitOnErr(cclv.Start())
}

func RunNr2xGet() {
	inPort := flag.Uint("i", Defaults.Nr2x.InPort, "MIDI in port")
	outPort := flag.Uint("o", Defaults.Nr2x.OutPort, "MIDI out port")
	globalChan := flag.Uint("g", uint(Defaults.Nr2x.GlobalMidiChan), "Global MIDI channel ")
	baseChan := flag.Uint("c", uint(Defaults.Nr2x.BaseMidiChan), "MIDI channel for voice/slot 0")
	voice := flag.Uint("v", uint(Defaults.Nr2x.Voice), "The voice/slot to get")

	ParseFlagsWithPositionalArg("output-file")
	filename := flag.Args()[0]

	ExitOnErr(nr2x.GetProgram(*inPort, *outPort, uint8(*globalChan), uint8(*baseChan), uint8(*voice), filename))
}

func RunNr2xSet() {
	inPort := flag.Uint("i", Defaults.Nr2x.InPort, "MIDI in port")
	outPort := flag.Uint("o", Defaults.Nr2x.OutPort, "MIDI out port")
	globalChan := flag.Uint("g", uint(Defaults.Nr2x.GlobalMidiChan), "Global MIDI channel ")
	baseChan := flag.Uint("c", uint(Defaults.Nr2x.BaseMidiChan), "MIDI channel for voice/slot 0")
	voice := flag.Uint("v", uint(Defaults.Nr2x.Voice), "The voice/slot to set")

	ParseFlagsWithPositionalArg("input-file")
	filename := flag.Args()[0]

	ExitOnErr(nr2x.SetProgram(*inPort, *outPort, uint8(*globalChan), uint8(*baseChan), uint8(*voice), filename))
}

func RunNd2Get() {
	inPort := flag.Uint("i", Defaults.Nd2.InPort, "MIDI in port")
	outPort := flag.Uint("o", Defaults.Nd2.OutPort, "MIDI out port")
	baseChan := flag.Uint("c", uint(Defaults.Nd2.BaseMidiChan), "MIDI channel for voice 0")

	ParseFlagsWithPositionalArg("output-file")
	filename := flag.Args()[0]

	ExitOnErr(nd2.GetProgram(*inPort, *outPort, uint8(*baseChan), filename))
}

func RunNd2Set() {
	inPort := flag.Uint("i", Defaults.Nd2.InPort, "MIDI in port")
	outPort := flag.Uint("o", Defaults.Nd2.OutPort, "MIDI out port")
	baseChan := flag.Uint("c", uint(Defaults.Nd2.BaseMidiChan), "MIDI channel for voice 0")

	ParseFlagsWithPositionalArg("input-file")
	filename := flag.Args()[0]

	ExitOnErr(nd2.SetProgram(*inPort, *outPort, uint8(*baseChan), filename))
}

func RunNd2Decoder() {
	inPort := flag.Uint("i", Defaults.Nd2.InPort, "MIDI in port")
	outPort := flag.Uint("o", Defaults.Nd2.OutPort, "MIDI out port")
	baseChan := flag.Uint("c", uint(Defaults.Nd2.BaseMidiChan), "MIDI channel for voice 0")
	flag.Parse()

	nd2Decoder, err := nd2.NewNd2Decoder(uint(*inPort), uint(*outPort), uint8(*baseChan))
	ExitOnErr(err)
	CallOnShutdownSignal(nd2Decoder.Stop)
	ExitOnErr(nd2Decoder.Run())
}

func RunNd2Test() {
	inPort := flag.Uint("i", Defaults.Nd2.InPort, "MIDI in port")
	outPort := flag.Uint("o", Defaults.Nd2.OutPort, "MIDI out port")
	baseChan := flag.Uint("c", uint(Defaults.Nd2.BaseMidiChan), "MIDI channel for voice 0")
	flag.Parse()

	nd2Decoder, err := nd2.NewNd2Decoder(uint(*inPort), uint(*outPort), uint8(*baseChan))
	ExitOnErr(err)
	CallOnShutdownSignal(nd2Decoder.Stop)
	ExitOnErr(nd2Decoder.Test())
}

func ExitOnErr(err error) {
	if err != nil {
		fmt.Printf("%s exited with error: %s\n", os.Args[0], err)
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

func ParseFlagsWithPositionalArg(argName string) {
	flag.Usage = func() {
		fmt.Printf("Usage: cctools %s [OPTIONS] %s\n", os.Args[0], argName)
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
}
