package util

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	flag "github.com/spf13/pflag"
)

type Command func(args []string) error

type CommandTree struct {
	Command     Command
	SubCommands map[string]*CommandTree
}

type CommandTreeOpt func(*CommandTree) error

func WithCommand(c Command) CommandTreeOpt {
	return func(ct *CommandTree) error {
		if ct.Command != nil {
			return errors.New("command already set")
		}
		if ct.SubCommands != nil {
			return errors.New("command tree node cannot have both a command and sub-commands")
		}
		ct.Command = c
		return nil
	}
}

func WithSubCommand(name string, c Command) CommandTreeOpt {
	return WithSubCommandTree(name, &CommandTree{Command: c})
}

func WithSubCommandTree(name string, subCommand *CommandTree) CommandTreeOpt {
	return func(ct *CommandTree) error {
		if ct.SubCommands[name] != nil {
			return errors.Errorf("subcommand '%s' already set", name)
		}
		if ct.Command != nil {
			return errors.New("command tree node cannot have both a command and sub-commands")
		}
		if ct.SubCommands == nil {
			ct.SubCommands = map[string]*CommandTree{}
		}
		ct.SubCommands[name] = subCommand
		return nil
	}
}

func NewCommandTree(opts ...CommandTreeOpt) (*CommandTree, error) {
	ct := &CommandTree{}
	for _, opt := range opts {
		if err := opt(ct); err != nil {
			return nil, errors.Wrap(err, "cannot intialise command tree")
		}
	}

	if ct.Command == nil && len(ct.SubCommands) == 0 {
		return nil, errors.New("either command or subcommands must be set")
	}

	return ct, nil
}

func NewCommandFuncNode(c Command) *CommandTree {
	return &CommandTree{
		Command: c,
	}
}

func NewSubCommandNode(subCommands map[string]*CommandTree) *CommandTree {
	return &CommandTree{
		SubCommands: subCommands,
	}
}

func GetCommand(ct *CommandTree, args []string) (Command, []string, error) {
	name := args[0]
	args = args[1:]
	for ct.Command == nil {
		if len(args) == 0 {
			return nil, nil, errors.Errorf("subcommand expected. %s", FormatSubtreeOptions(ct))
		}

		subTree, ok := ct.SubCommands[args[0]]
		if !ok {
			return nil, nil, errors.Errorf("unknown subcommand '%s'. %s", args[0], FormatSubtreeOptions(ct))
		}

		name = fmt.Sprintf("%s %s", name, args[0])
		args = args[1:]
		ct = subTree
	}

	return ct.Command, append([]string{name}, args...), nil
}

func FormatSubtreeOptions(tree *CommandTree) string {
	subcommands := []string{}
	for commandName := range tree.SubCommands {
		subcommands = append(subcommands, commandName)
	}
	return fmt.Sprintf("Options:\n  %s", strings.Join(subcommands, "\n  "))
}

type OptionDef struct {
	Name      string
	ShortName string
}

type ParseOptions struct {
	RequiredArguments []string
	RequiredOptions   []*OptionDef
}

type ParseOpt func(*ParseOptions)

func WithRequiredArg(arg string) ParseOpt {
	return func(po *ParseOptions) {
		po.RequiredArguments = append(po.RequiredArguments, arg)
	}
}

func WithRequiredOpt(name, shortName string) ParseOpt {
	return func(ro *ParseOptions) {
		ro.RequiredOptions = append(ro.RequiredOptions, &OptionDef{Name: name, ShortName: shortName})
	}
}

func ParseArgs(args []string, parseOpts ...ParseOpt) {
	reqOpts := &ParseOptions{
		RequiredArguments: []string{},
		RequiredOptions:   []*OptionDef{},
	}
	for _, opt := range parseOpts {
		opt(reqOpts)
	}

	flag.Usage = func() {
		fmt.Printf("Usage: %s [OPTIONS] %s\n", args[0], strings.Join(reqOpts.RequiredArguments, " "))
		flag.PrintDefaults()
	}

	flag.CommandLine.Parse(args[1:])

	if flag.NArg() != len(reqOpts.RequiredArguments) {
		if flag.NArg() < len(reqOpts.RequiredArguments) {
			fmt.Printf("argument(s) required: %s\n", strings.Join(reqOpts.RequiredArguments[flag.NArg():], " "))
		} else {
			fmt.Printf("unexpected argument(s): %s\n", strings.Join(flag.Args()[len(reqOpts.RequiredArguments):], " "))
		}
		flag.Usage()
		os.Exit(1)
	}

	seen := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { seen[f.Name] = true })
	for _, req := range reqOpts.RequiredOptions {
		if !seen[req.Name] && !seen[req.ShortName] {
			fmt.Printf("option is required: %s\n", req.Name)
			flag.Usage()
			os.Exit(1)
		}
	}
}
