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

func init() {
	Defaults = &DefaultFlags{}
	if err := yaml.Unmarshal(DefaultsFileBytes, Defaults); err != nil {
		panic(err)
	}
}

type Command struct {
	Run         func([]string) error
	SubCommands map[string]*Command
}

func (c *Command) Get(args []string, idx int) (func() error, error) {
	if len(c.SubCommands) == 0 {
		return func() error {
			return c.Run(args[idx:])
		}, nil
	}

	if len(args) == idx {
		return nil, c.FormatError("subcommand expected")
	}

	subCommand, ok := c.SubCommands[args[idx]]
	if !ok {
		return nil, c.FormatError("unknown subcommand '%s'", os.Args[idx])
	}
	return subCommand.Get(args, idx+1)
}

func (c *Command) FormatError(msg string, args ...interface{}) error {
	subcommands := []string{}
	for commandName := range c.SubCommands {
		subcommands = append(subcommands, commandName)
	}
	return errors.Errorf("%s. Options:\n  %s", fmt.Sprintf(msg, args...), strings.Join(subcommands, "\n  "))
}

func NewCommand(f func([]string) error) *Command {
	return &Command{Run: f}
}

var nr2xCommand = &Command{
	SubCommands: map[string]*Command{
		"get": NewCommand(RunNr2xGet),
		"set": NewCommand(RunNr2xSet),
	},
}

var nd2Command = &Command{
	SubCommands: map[string]*Command{
		"get":   NewCommand(RunNd2Get),
		"set":   NewCommand(RunNd2Set),
		"vrand": NewCommand(RunNd2RandomiseVoice),
	},
}

var mainCommand = &Command{
	SubCommands: map[string]*Command{
		"ls":     NewCommand(ListPorts),
		"log":    NewCommand(RunMidiLogger),
		"config": NewCommand(PrintDefaultConfig),
		"nr2x":   nr2xCommand,
		"nd2":    nd2Command,
	},
}

func main() {
	command, err := mainCommand.Get(os.Args, 1)
	if err != nil {
		fmt.Printf("Command line error: %s\n", err)
		os.Exit(1)
	}

	ExitOnErr(command())
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

func PrintDefaultConfig(args []string) error {
	flag.CommandLine.Parse(args)
	bts, err := yaml.Marshal(Defaults)
	if err != nil {
		return err
	}
	fmt.Print(string(bts))
	return nil
}

func ListPorts(args []string) error {
	flag.CommandLine.Parse(args)
	return util.ListPorts()
}

func RunMidiLogger(args []string) error {
	port := flag.UintP("port", "p", 1, "The port to listen to")
	flag.CommandLine.Parse(args)

	midiLogger := util.NewMidiLogger(*port - 1)
	CallOnShutdownSignal(midiLogger.Stop)
	return midiLogger.Start()
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

func RunNr2xGet(args []string) error {
	SetNr2xFlags()
	var perc bool
	flag.BoolVarP(&perc, "perc", "p", false, "get a percussion kit")
	ParseFlags(args, WithRequiredArg("output-file"))
	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nr2x.GetProgram(Defaults.Nr2x, perc, filename)
}

func RunNr2xSet(args []string) error {
	SetNr2xFlags()
	var perc bool
	flag.BoolVarP(&perc, "perc", "p", false, "set a percussion kit")
	ParseFlags(args, WithRequiredArg("input-file"))
	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nr2x.SetProgram(Defaults.Nr2x, perc, filename)
}

func RunNr2xMkVariations(args []string) error {
	maxMspFormat := false
	files := []string{}
	flag.BoolVarP(&maxMspFormat, "max", "m", false, "output in Max/MSP coll format")
	flag.StringSliceVarP(&files, "files", "f", []string{}, "variation files")
	ParseFlags(args, WithRequiredArg("output-file"))
	filename := flag.Args()[0]

	return nr2x.MakePercussionVariations(files, filename, maxMspFormat)
}

func RunNd2Get(args []string) error {
	SetNd2Flags()
	ParseFlags(args, WithRequiredArg("output-file"))
	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nd2.GetProgram(Defaults.Nd2, filename)
}

func RunNd2Set(args []string) error {
	SetNd2Flags()
	ParseFlags(args, WithRequiredArg("input-file"))
	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nd2.SetProgram(Defaults.Nd2, filename)
}

func RunNd2SetVoice(args []string) error {
	SetNd2Flags()
	var voice uint8
	flag.Uint8VarP(&voice, "voice", "v", 0, "Voice to set")
	ParseFlags(args, WithRequiredArg("input-file"), WithRequiredFlag("voice", "v"))

	filename := flag.Args()[0]

	Defaults.SetZeroIndexing()
	return nd2.SetVoice(Defaults.Nd2, filename, voice-1)
}

func RunNd2CopyVoice() {
	SetNd2Flags()
	var from, to uint8

	flag.Uint8VarP(&from, "from", "f", 0, "Voice to copy")
	flag.Uint8VarP(&to, "to", "t", 0, "Voice to set")
	flag.Parse()

	if from == 0 {
		ExitOnErr(errors.New("From voice must be set"))
	}

	if to == 0 {
		ExitOnErr(errors.New("To voice must be set"))
	}

	Defaults.SetZeroIndexing()
	ExitOnErr(nd2.CopyVoice(Defaults.Nd2, from-1, to-1))
}

func RunNd2RandomiseVoice(args []string) error {
	SetNd2Flags()
	var voice uint8
	flag.Uint8VarP(&voice, "voice", "v", 0, "Voice to randomise")

	ParseFlags(args, WithRequiredFlag("voice", "v"))

	Defaults.SetZeroIndexing()
	return nd2.SetRandomVoice(Defaults.Nd2, voice-1)
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

func RunNmG2Get(args []string) {
	SetNmG2Flags()
	maxMspFormat := false
	flag.BoolVarP(&maxMspFormat, "max", "m", false, "output in Max/MSP coll format")
	ParseFlags(args, WithRequiredArg("output-file"))
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

func ParseFlags(args []string, opts ...ParseOpt) {
	reqOpts := &RequiredOptions{
		Arguments: []string{},
		Flags:     []*FlagDef{},
	}
	for _, opt := range opts {
		opt(reqOpts)
	}

	flag.Usage = func() {
		fmt.Printf("Usage: %s %s [OPTIONS] %s\n", AppName, os.Args[0], strings.Join(reqOpts.Arguments, " "))
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
