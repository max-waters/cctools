package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/pkg/errors"
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

const (
	AppName                  = "cct"
	CommandPrintDefaults     = "config"
	CommandList              = "ls"
	CommandLog               = "log"
	CommandListen            = "listen"
	CommandNr2xSet           = "nr2x-set"
	CommandNr2xGet           = "nr2x-get"
	CommandNr2xMakePercVars  = "nr2x-mkpvars"
	CommandNd2Set            = "nd2-set"
	CommandNd2Get            = "nd2-get"
	CommandNd2SetVoice       = "nd2-vset"
	CommandNd2CopyVoice      = "nd2-vcpy"
	CommandNd2RandomiseVoice = "nd2-vrand"
	CommandNd2Decode         = "nd2-decode"
	CommandNd2Test           = "nd2-test"
	CommandNd2Nmg2           = "nd2-nmg2"
	CommandNmG2Get           = "nmg2-get"
	CommandNmG2Morph         = "nmg2-morph"
)

var mainCommand *util.CommandTree

func init() {
	Defaults = &DefaultFlags{}
	if err := yaml.Unmarshal(DefaultsFileBytes, Defaults); err != nil {
		panic(err)
	}

	nd2Command, err := util.NewCommandTree(
		util.WithSubCommand("get", RunNd2Get),
		util.WithSubCommand("set", RunNd2Set),
		util.WithSubCommand("vrand", RunNd2RandomiseVoice),
		util.WithSubCommand("vset", RunNd2SetVoice),
		util.WithSubCommand("vcopy", RunNd2CopyVoice),
	)
	if err != nil {
		panic(err)
	}

	nr2xCommand, err := util.NewCommandTree(
		util.WithSubCommand("get", RunNr2xGet),
		util.WithSubCommand("set", RunNr2xSet),
	)
	if err != nil {
		panic(err)
	}

	nmg2Command, err := util.NewCommandTree(
		util.WithSubCommand("get", RunNmG2Get),
		util.WithSubCommand("morph", RunNmG2Morph),
		util.WithSubCommand("nd2-controller", RunNd2NmG2),
	)
	if err != nil {
		panic(err)
	}

	mainCommand, err = util.NewCommandTree(
		util.WithSubCommand("ls", ListPorts),
		util.WithSubCommand("log", RunMidiLogger),
		util.WithSubCommand("config", PrintDefaultConfig),
		util.WithSubCommandTree("nr2x", nr2xCommand),
		util.WithSubCommandTree("nd2", nd2Command),
		util.WithSubCommandTree("nmg2", nmg2Command),
	)
	if err != nil {
		panic(err)
	}
}

func main() {
	commandName, command, err := util.GetCommand(mainCommand, os.Args)
	if err != nil {
		fmt.Printf("Command line error: %s\n", err)
		os.Exit(1)
	}

	if err := command(); err != nil {
		fmt.Printf("%s exited with error: %s\n", commandName, err)
		os.Exit(1)
	}
}

/*
func _main() {
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
	case CommandNr2xMakePercVars:
		RunNr2xMkVariations()
	// Nord Drum 2
	case CommandNd2Get:
		RunNd2Get()
	case CommandNd2Set:
		RunNd2Set()
	case CommandNd2SetVoice:
		RunNd2SetVoice()
	case CommandNd2CopyVoice:
		RunNd2CopyVoice()
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
*/

// func PrintCommandsAndExit(err error) {
// 	options := mainCommand.FormatSubCommands()
// 	fmt.Printf("%s. Options:\n  %s\n", err, strings.Join(options, "\n  "))
// 	os.Exit(1)
// }

func PrintDefaultConfig(subCommands, args []string) error {
	ParseFlags(subCommands, args)

	bts, err := yaml.Marshal(Defaults)
	if err != nil {
		return err
	}
	fmt.Print(string(bts))
	return nil
}

func ListPorts(subCommands, args []string) error {
	ParseFlags(subCommands, args)

	return util.ListPorts()
}

func RunMidiLogger(subCommands, args []string) error {
	port := flag.UintP("port", "p", 1, "The port to listen to")

	ParseFlags(subCommands, args)

	midiLogger := util.NewMidiLogger(*port - 1)
	CallOnShutdownSignal(midiLogger.Stop)

	return midiLogger.Start()
}

func RunControlChangeListener(subCommands, args []string) error {
	port := flag.UintP("port", "p", 1, "The port to listen to")
	channel := flag.UintP("chan", "c", 1, "The channel to listen to")
	outputfile := flag.StringP("file", "f", "", "Output file name")

	ParseFlags(subCommands, args)

	cclv := util.NewControlChangeListenerView(uint(*port)-1, uint8(*channel)-1, *outputfile)
	CallOnShutdownSignal(cclv.Stop)

	return cclv.Start()
}

func RunNr2xGet(subCommands, args []string) error {
	SetNr2xFlags()
	var perc bool
	flag.BoolVarP(&perc, "percussion", "p", false, "get a percussion kit")

	ParseFlags(subCommands, args, WithRequiredArg("output-file"))

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nr2x.GetProgram(Defaults.Nr2x, perc, filename)
}

func RunNr2xSet(subCommands, args []string) error {
	SetNr2xFlags()
	var perc bool
	flag.BoolVarP(&perc, "perc", "p", false, "set a percussion kit")

	ParseFlags(subCommands, args, WithRequiredArg("input-file"))

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nr2x.SetProgram(Defaults.Nr2x, perc, filename)
}

func RunNr2xMkVariations(subCommands, args []string) error {
	maxMspFormat := false
	files := []string{}
	flag.BoolVarP(&maxMspFormat, "max", "m", false, "output in Max/MSP coll format")
	flag.StringSliceVarP(&files, "files", "f", []string{}, "variation files")

	ParseFlags(subCommands, args, WithRequiredArg("output-file"))

	filename := flag.Args()[0]

	return nr2x.MakePercussionVariations(files, filename, maxMspFormat)
}

func RunNd2Get(subCommands, args []string) error {
	SetNd2Flags()

	ParseFlags(subCommands, args, WithRequiredArg("output-file"))

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nd2.GetProgram(Defaults.Nd2, filename)
}

func RunNd2Set(subCommands, args []string) error {
	SetNd2Flags()

	ParseFlags(subCommands, args, WithRequiredArg("input-file"))

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nd2.SetProgram(Defaults.Nd2, filename)
}

func RunNd2SetVoice(subCommands, args []string) error {
	SetNd2Flags()
	var voice uint8
	flag.Uint8VarP(&voice, "voice", "v", 0, "Voice to set")

	ParseFlags(subCommands, args, WithRequiredArg("input-file"), WithRequiredFlag("voice", "v"))

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nd2.SetVoice(Defaults.Nd2, filename, voice-1)
}

func RunNd2CopyVoice(subCommands, args []string) error {
	SetNd2Flags()
	var from, to uint8
	flag.Uint8VarP(&from, "from", "f", 0, "Voice to copy")
	flag.Uint8VarP(&to, "to", "t", 0, "Voice to set")

	ParseFlags(subCommands, args, WithRequiredFlag("from", "f"), WithRequiredFlag("to", "t"))

	Defaults.SetZeroIndexing()

	return nd2.CopyVoice(Defaults.Nd2, from-1, to-1)
}

func RunNd2RandomiseVoice(subCommands, args []string) error {
	SetNd2Flags()
	var voice uint8
	flag.Uint8VarP(&voice, "voice", "v", 0, "Voice to randomise")

	ParseFlags(subCommands, args, WithRequiredFlag("voice", "v"))

	Defaults.SetZeroIndexing()
	return nd2.SetRandomVoice(Defaults.Nd2, voice-1)
}

func RunNd2NmG2(subCommands, args []string) error {
	SetNd2Flags()
	flag.UintVarP(&Defaults.NmG2.InPort, "g2-in", "ig", Defaults.NmG2.InPort, "Nord G2 MIDI in port")
	flag.UintVarP(&Defaults.NmG2.OutPort, "g2-out", "og", Defaults.NmG2.OutPort, "Nord G2 MIDI out port")

	ParseFlags(subCommands, args)

	nmg2Conn, err := nd2.NewNmG2Connection(Defaults.Nd2, Defaults.NmG2)
	if err != nil {
		return errors.Wrap(err, "initialisation error")
	}
	CallOnShutdownSignal(nmg2Conn.Stop)

	return nmg2Conn.Run()
}

func RunNmG2Get(subCommands, args []string) error {
	SetNmG2Flags()
	maxMspFormat := false
	flag.BoolVarP(&maxMspFormat, "max", "m", false, "output in Max/MSP coll format")

	ParseFlags(subCommands, args, WithRequiredArg("output-file"))

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nmg2.GetVariations(Defaults.NmG2, filename, maxMspFormat)
}

func RunNmG2Morph(subCommands, args []string) error {
	SetNmG2Flags()
	var l, r, m uint8
	flag.Uint8VarP(&l, "left", "l", 117, "Nord G2 target controller num")
	flag.Uint8VarP(&r, "right", "r", 118, "Nord G2 target controller num")
	flag.Uint8VarP(&m, "morph", "m", 119, "Nord G2 morpher controller num")

	ParseFlags(subCommands, args)

	Defaults.SetZeroIndexing()

	morpher, err := nmg2.NewNmG2Morpher(Defaults.NmG2, l, r, m)
	if err != nil {
		return errors.Wrap(err, "initialisation error")
	}
	CallOnShutdownSignal(morpher.Close)

	return morpher.Start()
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

func CallOnShutdownSignal(f func()) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sigChan
		f()
	}()
}

type FlagDef struct {
	Name      string
	ShortName string
}

type RequiredOptions struct {
	Arguments []string
	Flags     []*FlagDef
}

type ParseOpt func(*RequiredOptions)

func WithRequiredArg(arg string) ParseOpt {
	return func(ro *RequiredOptions) {
		ro.Arguments = append(ro.Arguments, arg)
	}
}

func WithRequiredFlag(name, shortName string) ParseOpt {
	return func(ro *RequiredOptions) {
		ro.Flags = append(ro.Flags, &FlagDef{Name: name, ShortName: shortName})
	}
}

func ParseFlags(subCommands, args []string, opts ...ParseOpt) {
	reqOpts := &RequiredOptions{
		Arguments: []string{},
		Flags:     []*FlagDef{},
	}
	for _, opt := range opts {
		opt(reqOpts)
	}

	flag.Usage = func() {
		fmt.Printf("Usage: %s [OPTIONS] %s\n", strings.Join(subCommands, " "), strings.Join(reqOpts.Arguments, " "))
		flag.PrintDefaults()
	}

	flag.CommandLine.Parse(args)

	if flag.NArg() != len(reqOpts.Arguments) {
		if flag.NArg() < len(reqOpts.Arguments) {
			fmt.Printf("argument(s) required: %s\n", strings.Join(reqOpts.Arguments[flag.NArg():], " "))
		} else {
			fmt.Printf("unexpected argument(s): %s\n", strings.Join(flag.Args()[len(reqOpts.Arguments):], " "))
		}
		flag.Usage()
		os.Exit(1)
	}

	seen := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { seen[f.Name] = true })
	for _, req := range reqOpts.Flags {
		if !seen[req.Name] && !seen[req.ShortName] {
			fmt.Printf("flag is required: %s\n", req.Name)
			flag.Usage()
			os.Exit(1)
		}
	}
}
