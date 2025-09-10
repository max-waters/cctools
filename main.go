package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/pkg/errors"
	flag "github.com/spf13/pflag"

	_ "embed"

	"gopkg.in/yaml.v2"
	"mvw.org/cctools/emd"
	"mvw.org/cctools/nd2"
	"mvw.org/cctools/nmg2"
	"mvw.org/cctools/nr2x"
	"mvw.org/cctools/util"
)

type DefaultFlags struct {
	Nr2x *nr2x.Nr2xConnectionConfig `yaml:"nr2x"`
	Nd2  *nd2.Nd2ConnectionConfig   `yaml:"nd2"`
	NmG2 *nmg2.NmG2ConnectionConfig `yaml:"nmg2"`
	Emd  *emd.EmdConnectionConfig   `yaml:"emd"`
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

	def.Emd.InPort--
	def.Emd.OutPort--
}

//go:embed defaults.yaml
var DefaultsFileBytes []byte
var Defaults *DefaultFlags

var mainCommand *util.CommandTree

func init() {
	// just log msg as passed in
	log.SetFlags(0)

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

	emdCommand, err := util.NewCommandTree(
		util.WithSubCommand("get", RunEmdGetKit),
		util.WithSubCommand("set", RunEmdSetKit),
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
		util.WithSubCommandTree("emd", emdCommand),
	)
	if err != nil {
		panic(err)
	}
}

func main() {
	command, args, err := util.GetCommand(mainCommand, os.Args)
	if err != nil {
		fmt.Printf("Command line error: %s\n", err)
		os.Exit(1)
	}

	if err := command(args); err != nil {
		fmt.Printf("%s exited with error: %s\n", args[0], err)
		os.Exit(1)
	}
}

func PrintDefaultConfig(args []string) error {
	ParseArgs(args)

	bts, err := yaml.Marshal(Defaults)
	if err != nil {
		return err
	}
	fmt.Print(string(bts))
	return nil
}

func ListPorts(args []string) error {
	ParseArgs(args)

	return util.ListPorts()
}

func RunMidiLogger(args []string) error {
	port := flag.UintP("port", "p", 1, "The port to listen to")

	ParseArgs(args)

	midiLogger := util.NewMidiLogger(*port - 1)
	CallOnShutdownSignal(midiLogger.Stop)

	return midiLogger.Start()
}

func RunControlChangeListener(args []string) error {
	port := flag.UintP("port", "p", 1, "The port to listen to")
	channel := flag.UintP("chan", "c", 1, "The channel to listen to")
	outputfile := flag.StringP("file", "f", "", "Output file name")

	ParseArgs(args)

	cclv := util.NewControlChangeListenerView(uint(*port)-1, uint8(*channel)-1, *outputfile)
	CallOnShutdownSignal(cclv.Stop)

	return cclv.Start()
}

func RunNr2xGet(args []string) error {
	SetNr2xFlags()
	var perc bool
	flag.BoolVarP(&perc, "perc", "p", false, "get a percussion kit")

	ParseArgs(args, util.WithRequiredArg("output-file"))

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	switch ext := filepath.Ext(filename); ext {
	case ".csv":
		return nr2x.GetProgram(Defaults.Nr2x, perc, filename)
	case ".syx":
		return nr2x.GetSysexProgram(Defaults.Nr2x, perc, filename)
	default:
		return fmt.Errorf("unknown extension: %s", ext)
	}
}

func RunNr2xSet(args []string) error {
	SetNr2xFlags()
	var perc bool
	flag.BoolVarP(&perc, "perc", "p", false, "set a percussion kit")

	ParseArgs(args, util.WithRequiredArg("input-file"))

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	switch ext := filepath.Ext(filename); ext {
	case ".csv":
		return nr2x.SetProgram(Defaults.Nr2x, perc, filename)
	case ".syx":
		return nr2x.SetSysExProgram(Defaults.Nr2x, perc, filename)
	default:
		return fmt.Errorf("unknown extension: %s", ext)
	}
}

func RunNr2xMkVariations(args []string) error {
	maxMspFormat := false
	files := []string{}
	flag.BoolVarP(&maxMspFormat, "max", "m", false, "output in Max/MSP coll format")
	flag.StringSliceVarP(&files, "files", "f", []string{}, "variation files")

	ParseArgs(args, util.WithRequiredArg("output-file"))

	filename := flag.Args()[0]

	return nr2x.MakePercussionVariations(files, filename, maxMspFormat)
}

func RunNd2Get(args []string) error {
	SetNd2Flags()

	ParseArgs(args, util.WithRequiredArg("output-file"))

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nd2.GetProgram(Defaults.Nd2, filename)
}

func RunNd2Set(args []string) error {
	SetNd2Flags()

	ParseArgs(args, util.WithRequiredArg("input-file"))

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nd2.SetProgram(Defaults.Nd2, filename)
}

func RunNd2SetVoice(args []string) error {
	SetNd2Flags()
	var voice uint8
	flag.Uint8VarP(&voice, "voice", "c", 0, "Voice to set")

	ParseArgs(args, util.WithRequiredArg("input-file"), util.WithRequiredOpt("voice", "c"))

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nd2.SetVoice(Defaults.Nd2, filename, voice-1)
}

func RunNd2CopyVoice(args []string) error {
	SetNd2Flags()
	var from, to uint8
	flag.Uint8VarP(&from, "from", "f", 0, "Voice to copy")
	flag.Uint8VarP(&to, "to", "t", 0, "Voice to set")

	ParseArgs(args, util.WithRequiredOpt("from", "f"), util.WithRequiredOpt("to", "t"))

	Defaults.SetZeroIndexing()

	return nd2.CopyVoice(Defaults.Nd2, from-1, to-1)
}

func RunNd2RandomiseVoice(args []string) error {
	SetNd2Flags()
	var voice uint8
	flag.Uint8VarP(&voice, "voice", "c", 0, "Voice to randomise")
	var incLevel, incPan, incEcho bool
	flag.BoolVarP(&incLevel, "level", "l", false, "Randomise level")
	flag.BoolVarP(&incPan, "pan", "p", false, "Randomise pan")
	flag.BoolVarP(&incEcho, "echo", "e", false, "Randomise echo")

	ParseArgs(args, util.WithRequiredOpt("voice", "c"))

	Defaults.SetZeroIndexing()

	return nd2.SetRandomVoice(Defaults.Nd2, voice-1, incLevel, incPan, incEcho)
}

func RunNd2NmG2(args []string) error {
	SetNd2Flags()
	flag.UintVarP(&Defaults.NmG2.InPort, "g2-in", "ig", Defaults.NmG2.InPort, "Nord G2 MIDI in port")
	flag.UintVarP(&Defaults.NmG2.OutPort, "g2-out", "og", Defaults.NmG2.OutPort, "Nord G2 MIDI out port")

	ParseArgs(args)

	nmg2Conn, err := nd2.NewNmG2Connection(Defaults.Nd2, Defaults.NmG2)
	if err != nil {
		return errors.Wrap(err, "initialisation error")
	}
	CallOnShutdownSignal(nmg2Conn.Stop)

	return nmg2Conn.Run()
}

func RunNmG2Get(args []string) error {
	SetNmG2Flags()
	maxMspFormat := false
	flag.BoolVarP(&maxMspFormat, "max", "m", false, "output in Max/MSP coll format")

	ParseArgs(args, util.WithRequiredArg("output-file"))

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	return nmg2.GetVariations(Defaults.NmG2, filename, maxMspFormat)
}

func RunNmG2Morph(args []string) error {
	SetNmG2Flags()
	var l, r, m uint8
	flag.Uint8VarP(&l, "left", "l", 117, "Nord G2 target controller num")
	flag.Uint8VarP(&r, "right", "r", 118, "Nord G2 target controller num")
	flag.Uint8VarP(&m, "morph", "m", 119, "Nord G2 morpher controller num")

	ParseArgs(args)

	Defaults.SetZeroIndexing()

	morpher, err := nmg2.NewNmG2Morpher(Defaults.NmG2, l, r, m)
	if err != nil {
		return errors.Wrap(err, "initialisation error")
	}
	CallOnShutdownSignal(morpher.Close)

	return morpher.Start()
}

func RunEmdGetKit(args []string) error {
	SetEmdFlags()
	var kit uint8
	flag.Uint8VarP(&kit, "kit", "k", 0, "Kit to get (1-64, 0 or 65 for edit buffer)")

	ParseArgs(args, util.WithRequiredArg("output-file"))

	if kit > 65 {
		return fmt.Errorf("kit must be 0-65: %d", kit)
	}
	if kit == 0 {
		kit = 65
	}
	kit-- // zero-index

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	switch ext := filepath.Ext(filename); ext {
	case ".syx":
		return emd.GetSysexKit(Defaults.Emd, kit, filename)
	default:
		return fmt.Errorf("unknown extension: %s", ext)
	}
}

func RunEmdSetKit(args []string) error {
	SetEmdFlags()
	var kit uint8
	flag.Uint8VarP(&kit, "kit", "k", 0, "Kit to get (1-64, 0 or 65 for edit buffer)")

	ParseArgs(args, util.WithRequiredArg("output-file"))

	if kit > 65 {
		return fmt.Errorf("kit must be 0-65: %d", kit)
	}
	if kit == 0 {
		kit = 65
	}
	kit-- // zero-index

	filename := flag.Args()[0]
	Defaults.SetZeroIndexing()

	switch ext := filepath.Ext(filename); ext {
	case ".syx":
		return emd.SetSysexKit(Defaults.Emd, kit, filename)
	default:
		return fmt.Errorf("unknown extension: %s", ext)
	}
}

func SetNr2xFlags() {
	flag.UintVarP(&Defaults.Nr2x.InPort, "in", "i", Defaults.Nr2x.InPort, "Nord Rack 2X MIDI in port")
	flag.UintVarP(&Defaults.Nr2x.OutPort, "out", "o", Defaults.Nr2x.OutPort, "Nord Rack 2X MIDI out port")
	flag.StringVarP(&Defaults.Nr2x.Voice, "voice", "c", Defaults.Nr2x.Voice, "Nord Rack 2X voice/slot [A, B, C, D]")
	flag.Uint8VarP(&Defaults.Nr2x.GlobalMidiChan, "global", "g", Defaults.Nr2x.GlobalMidiChan, "Nord Rack 2X Global MIDI channel")
}

func SetNmG2Flags() {
	flag.UintVarP(&Defaults.NmG2.InPort, "in", "i", Defaults.NmG2.InPort, "Nord G2 MIDI in port")
	flag.UintVarP(&Defaults.NmG2.OutPort, "out", "o", Defaults.NmG2.OutPort, "Nord G2 MIDI out port")
	flag.StringVarP(&Defaults.NmG2.Voice, "voice", "c", Defaults.NmG2.Voice, "Nord G2 voice/slot [A, B, C, D]")
	flag.Uint8VarP(&Defaults.NmG2.GlobalMidiChan, "global", "g", Defaults.NmG2.GlobalMidiChan, "Nord G2 Global MIDI channel")
}

func SetNd2Flags() {
	flag.UintVarP(&Defaults.Nd2.InPort, "in", "i", Defaults.Nd2.InPort, "Nord Drum 2 MIDI in port")
	flag.UintVarP(&Defaults.Nd2.OutPort, "out", "o", Defaults.Nd2.InPort, "Nord Drum 2 MIDI in port")
	flag.Uint8VarP(&Defaults.Nd2.BaseMidiChannel, "base", "b", Defaults.Nd2.BaseMidiChannel, "MIDI channel for Nord Drum 2 voice 1")
	flag.Uint8VarP(&Defaults.Nd2.GlobalMidiChannel, "global", "g", Defaults.Nd2.GlobalMidiChannel, "Nord Drum 2 Global MIDI channel")
}

func SetEmdFlags() {
	flag.UintVarP(&Defaults.Emd.InPort, "in", "i", Defaults.Emd.InPort, "Elektron Machinedrum in port")
	flag.UintVarP(&Defaults.Emd.OutPort, "out", "o", Defaults.Emd.OutPort, "Elektron Machinedrum out port")
}

func ParseArgs(args []string, parseOpts ...util.ParseOpt) {
	// set common flags
	verbose := false
	flag.BoolVarP(&verbose, "verbose", "v", false, "verbose mode")

	util.ParseArgs(args, parseOpts...)

	if !verbose {
		log.SetOutput(io.Discard)
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
