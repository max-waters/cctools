package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	flag "github.com/spf13/pflag"

	_ "embed"

	"gopkg.in/yaml.v2"
	"mvw.org/cctools/nd2"
	"mvw.org/cctools/nr2x"
	"mvw.org/cctools/util"
)

type DefaultFlags struct {
	Nr2x *nr2x.Nr2xConnectionConfig `yaml:"nr2x"`
	Nd2  *nd2.Nd2ConnectionConfig   `yaml:"nd2"`
	NmG2 *nd2.NmG2Config            `yaml:"nmg2"`
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
const CommandNd2Nmg2 = "nd2-nmg2"

func init() {
	Defaults = &DefaultFlags{}
	if err := yaml.Unmarshal(DefaultsFileBytes, Defaults); err != nil {
		panic(err)
	}
}

func main() {
	if len(os.Args) <= 1 {
		PrintCommandsAndExit("No command supplied")
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
	case CommandNd2Nmg2:
		RunNd2NmG2()
	default:
		PrintCommandsAndExit(fmt.Sprintf("Unknown command: '%s'", command))
	}
}

func PrintCommandsAndExit(cause string) {
	fmt.Printf("%s. Options:\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n", cause,
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
	SetNr2xFlags()
	voice := flag.Uint8("v", 0, "The voice/slot to get")
	ParseFlagsWithPositionalArg("output-file")
	filename := flag.Args()[0]

	ExitOnErr(nr2x.GetProgram(Defaults.Nr2x, *voice, filename))
}

func RunNr2xSet() {
	SetNr2xFlags()
	voice := flag.Uint8("v", 0, "The voice/slot to set")
	ParseFlagsWithPositionalArg("input-file")
	filename := flag.Args()[0]

	ExitOnErr(nr2x.GetProgram(Defaults.Nr2x, *voice, filename))
}

func RunNd2Get() {
	SetNd2Flags()
	ParseFlagsWithPositionalArg("output-file")
	filename := flag.Args()[0]

	ExitOnErr(nd2.GetProgram(Defaults.Nd2, filename))
}

func RunNd2Set() {
	SetNd2Flags()
	ParseFlagsWithPositionalArg("input-file")
	filename := flag.Args()[0]

	ExitOnErr(nd2.SetProgram(Defaults.Nd2, filename))
}

func RunNd2Decoder() {
	SetNd2Flags()
	flag.Parse()

	nd2Decoder, err := nd2.NewNd2Decoder(Defaults.Nd2)
	ExitOnErr(err)
	CallOnShutdownSignal(nd2Decoder.Stop)
	ExitOnErr(nd2Decoder.Run())
}

func RunNd2Test() {
	SetNd2Flags()
	flag.Parse()

	nd2Decoder, err := nd2.NewNd2Decoder(Defaults.Nd2)
	ExitOnErr(err)
	CallOnShutdownSignal(nd2Decoder.Stop)
	ExitOnErr(nd2Decoder.Test())
}

func RunNd2NmG2() {
	SetNd2Flags()
	flag.UintVar(&Defaults.NmG2.InPort, "ig", Defaults.NmG2.InPort, "G2 MIDI in port")
	flag.UintVar(&Defaults.NmG2.OutPort, "og", Defaults.NmG2.OutPort, "G2 MIDI out port")
	flag.Parse()

	nmg2Conn, err := nd2.NewNmG2Connection(Defaults.Nd2, Defaults.NmG2)
	ExitOnErr(err)
	CallOnShutdownSignal(nmg2Conn.Stop)
	ExitOnErr(nmg2Conn.Run())
}

func SetNr2xFlags() {
	flag.UintVar(&Defaults.Nr2x.InPort, "i", Defaults.Nr2x.InPort, "MIDI in port")
	flag.UintVar(&Defaults.Nr2x.OutPort, "o", Defaults.Nr2x.OutPort, "MIDI out port")
	flag.Uint8Var(&Defaults.Nr2x.BaseMidiChan, "b", Defaults.Nr2x.BaseMidiChan, "MIDI channel for voice/slot 0")
	flag.Uint8Var(&Defaults.Nr2x.GlobalMidiChan, "g", Defaults.Nr2x.GlobalMidiChan, "Global MIDI channel ")
}

func SetNd2Flags() {
	flag.UintVar(&Defaults.Nd2.InPort, "i", Defaults.Nd2.InPort, "MIDI in port")
	flag.UintVar(&Defaults.Nd2.OutPort, "o", Defaults.Nd2.InPort, "MIDI in port")
	flag.Uint8Var(&Defaults.Nd2.BaseMidiChannel, "b", Defaults.Nd2.BaseMidiChannel, "MIDI channel for voice 0")
	flag.Uint8Var(&Defaults.Nd2.GlobalMidiChannel, "g", Defaults.Nd2.GlobalMidiChannel, "Global MIDI channel")
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
