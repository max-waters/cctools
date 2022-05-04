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
	"mvw.org/cctools/nmg2"
	"mvw.org/cctools/nr2x"
	"mvw.org/cctools/util"
)

type DefaultFlags struct {
	Nr2x *nr2x.Nr2xConnectionConfig `yaml:"nr2x"`
	Nd2  *nd2.Nd2ConnectionConfig   `yaml:"nd2"`
	NmG2 *nmg2.NmG2ConnectionConfig `yaml:"nmg2"`
}

func (def DefaultFlags) SetZeroIndexing() {
	def.Nr2x.GlobalMidiChan--
	def.Nr2x.InPort--
	def.Nr2x.OutPort--
	for v, c := range def.Nr2x.VoiceChannelMap {
		def.Nr2x.VoiceChannelMap[v] = c - 1
	}

	def.Nd2.BaseMidiChannel--
	def.Nd2.GlobalMidiChannel--
	def.Nd2.InPort--
	def.Nd2.OutPort--

	def.NmG2.GlobalMidiChan--
	def.NmG2.InPort--
	def.NmG2.OutPort--
	for v, c := range def.NmG2.VoiceChannelMap {
		def.NmG2.VoiceChannelMap[v] = c - 1
	}
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
const CommandNmG2Get = "nmg2-get"
const CommandNmG2Morph = "nmg2-morph"

const CommandPrintDefaults = "print-defaults"

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
	case CommandPrintDefaults:
		PrintDefaults()
	case CommandList:
		ListPorts()
	case CommandLog:
		RunMidiLogger()
	case CommandListen:
		RunControlChangeListener()
	// Nord Rack 2X
	case CommandNr2xGet:
		RunNr2xGet()
	case CommandNr2xSet:
		RunNr2xSet()
	// Nord Drum 2
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
	// Nord Mdular G2
	case CommandNmG2Morph:
		RunNmG2Morph()
	case CommandNmG2Get:
		RunNmG2Get()
	default:
		PrintCommandsAndExit(fmt.Sprintf("Unknown command: '%s'", command))
	}
}

func PrintCommandsAndExit(cause string) {
	fmt.Printf("%s. Options:\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n  %s\n", cause,
		CommandList, CommandLog, CommandListen, CommandNr2xGet, CommandNr2xSet,
		CommandNd2Get, CommandNd2Set, CommandNmG2Get, CommandNmG2Morph, CommandNd2Nmg2, CommandNd2Decode, CommandNd2Test)
	os.Exit(1)
}

func ListPorts() {
	flag.Parse()
	ExitOnErr(util.ListPorts())
}

func RunMidiLogger() {
	port := flag.UintP("port", "p", 1, "The port to listen to")
	flag.Parse()

	midiLogger := util.NewMidiLogger(*port - 1)
	CallOnShutdownSignal(midiLogger.Stop)
	ExitOnErr(midiLogger.Start())
}

func RunControlChangeListener() {
	port := flag.UintP("port", "p", 1, "The port to listen to")
	channel := flag.UintP("chan", "c", 1, "The channel to listen to")
	outputfile := flag.StringP("file", "f", "", "Output file name")
	flag.Parse()

	cclv := util.NewControlChangeListenerView(uint(*port)-1, uint8(*channel)-1, *outputfile)
	CallOnShutdownSignal(cclv.Stop)
	ExitOnErr(cclv.Start())
}

func RunNr2xGet() {
	SetNr2xFlags()
	var perc bool
	flag.BoolVarP(&perc, "perc", "p", false, "get a percussion kit")
	ParseFlagsWithPositionalArg("output-file")
	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	ExitOnErr(nr2x.GetProgram(Defaults.Nr2x, perc, filename))
}

func RunNr2xSet() {
	SetNr2xFlags()
	var perc bool
	flag.BoolVarP(&perc, "perc", "p", false, "set a percussion kit")
	ParseFlagsWithPositionalArg("input-file")
	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	ExitOnErr(nr2x.SetProgram(Defaults.Nr2x, perc, filename))
}

func RunNd2Get() {
	SetNd2Flags()
	ParseFlagsWithPositionalArg("output-file")
	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	ExitOnErr(nd2.GetProgram(Defaults.Nd2, filename))
}

func RunNd2Set() {
	SetNd2Flags()
	ParseFlagsWithPositionalArg("input-file")
	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	ExitOnErr(nd2.SetProgram(Defaults.Nd2, filename))
}

func RunNd2Decoder() {
	SetNd2Flags()
	flag.Parse()
	Defaults.SetZeroIndexing()

	nd2Decoder, err := nd2.NewNd2Decoder(Defaults.Nd2)
	ExitOnErr(err)
	CallOnShutdownSignal(nd2Decoder.Stop)
	ExitOnErr(nd2Decoder.Run())
}

func RunNd2Test() {
	SetNd2Flags()
	flag.Parse()
	Defaults.SetZeroIndexing()

	nd2Decoder, err := nd2.NewNd2Decoder(Defaults.Nd2)
	ExitOnErr(err)
	CallOnShutdownSignal(nd2Decoder.Stop)
	ExitOnErr(nd2Decoder.Test())
}

func RunNd2NmG2() {
	SetNd2Flags()
	flag.UintVarP(&Defaults.NmG2.InPort, "g2-in", "ig", Defaults.NmG2.InPort, "Nord G2 MIDI in port")
	flag.UintVarP(&Defaults.NmG2.OutPort, "g2-out", "og", Defaults.NmG2.OutPort, "Nord G2 MIDI out port")
	flag.Parse()

	Defaults.SetZeroIndexing()
	nmg2Conn, err := nd2.NewNmG2Connection(Defaults.Nd2, Defaults.NmG2)
	ExitOnErr(err)
	CallOnShutdownSignal(nmg2Conn.Stop)
	ExitOnErr(nmg2Conn.Run())
}

func RunNmG2Get() {
	SetNmG2Flags()
	maxMspFormat := false
	flag.BoolVarP(&maxMspFormat, "max", "m", false, "output in Max/MSP coll format")
	ParseFlagsWithPositionalArg("output-file")
	filename := flag.Args()[0]

	Defaults.SetZeroIndexing()
	ExitOnErr(nmg2.GetVariations(Defaults.NmG2, filename, maxMspFormat))
}

func RunNmG2Morph() {
	SetNmG2Flags()
	var l, r, m uint8
	flag.Uint8VarP(&l, "left", "l", 117, "Nord G2 target controller num")
	flag.Uint8VarP(&r, "right", "r", 118, "Nord G2 target controller num")
	flag.Uint8VarP(&m, "morph", "m", 119, "Nord G2 morpher controller num")
	flag.Parse()

	Defaults.SetZeroIndexing()

	morpher, err := nmg2.NewNmG2Morpher(Defaults.NmG2, l, r, m)
	ExitOnErr(err)
	CallOnShutdownSignal(morpher.Close)
	ExitOnErr(morpher.Start())
}

func SetNr2xFlags() {
	flag.UintVarP(&Defaults.Nr2x.InPort, "in", "i", Defaults.Nr2x.InPort, "Nord Rack 2X MIDI in port")
	flag.UintVarP(&Defaults.Nr2x.OutPort, "out", "o", Defaults.Nr2x.OutPort, "Nord Rack 2X MIDI out port")
	flag.StringVarP(&Defaults.Nr2x.Voice, "voice", "v", Defaults.Nr2x.Voice, "Nord Rack 2X voice/slot [A, B, C, D]")
	flag.Uint8VarP(&Defaults.Nr2x.GlobalMidiChan, "global", "g", Defaults.Nr2x.GlobalMidiChan, "Nord Rack 2X Global MIDI channel")
}

func SetNmG2Flags() {
	flag.UintVarP(&Defaults.NmG2.InPort, "in", "i", Defaults.NmG2.InPort, "Nord G2 MIDI in port")
	flag.UintVarP(&Defaults.NmG2.OutPort, "out", "o", Defaults.NmG2.OutPort, "Nord G2 MIDI out port")
	flag.StringVarP(&Defaults.NmG2.Voice, "voice", "v", Defaults.NmG2.Voice, "Nord G2 voice/slot [A, B, C, D]")
	flag.Uint8VarP(&Defaults.NmG2.GlobalMidiChan, "global", "g", Defaults.NmG2.GlobalMidiChan, "Nord G2 Global MIDI channel")
}

func SetNd2Flags() {
	flag.UintVarP(&Defaults.Nd2.InPort, "in", "i", Defaults.Nd2.InPort, "Nord Drum 2 MIDI in port")
	flag.UintVarP(&Defaults.Nd2.OutPort, "out", "o", Defaults.Nd2.InPort, "Nord Drum 2 MIDI in port")
	flag.Uint8VarP(&Defaults.Nd2.BaseMidiChannel, "base", "b", Defaults.Nd2.BaseMidiChannel, "MIDI channel for Nord Drum 2 voice 1")
	flag.Uint8VarP(&Defaults.Nd2.GlobalMidiChannel, "global", "g", Defaults.Nd2.GlobalMidiChannel, "Nord Drum 2 Global MIDI channel")
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

func PrintDefaults() {
	bts, err := yaml.Marshal(Defaults)
	if err != nil {
		ExitOnErr(err)
	}
	fmt.Print(string(bts))
}
