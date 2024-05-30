package util

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

type CommandTree struct {
	Command     Command
	SubCommands map[string]*CommandTree
}

type Command func(commandName, args []string) error

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

func GetCommand(ct *CommandTree, args []string) (string, func() error, error) {
	commandName := args[:1]
	args = args[1:]
	for ct.Command == nil {
		if len(args) == 0 {
			return "", nil, errors.Errorf("subcommand expected. %s", FormatSubtreeOptions(ct))
		}

		subTree, ok := ct.SubCommands[args[0]]
		if !ok {
			return "", nil, errors.Errorf("unknown subcommand '%s'. %s", args[0], FormatSubtreeOptions(ct))
		}

		commandName = append(commandName, args[0])
		args = args[1:]
		ct = subTree
	}

	return strings.Join(commandName, " "), func() error {
		return ct.Command(commandName, args)
	}, nil
}

func FormatSubtreeOptions(tree *CommandTree) string {
	subcommands := []string{}
	for commandName := range tree.SubCommands {
		subcommands = append(subcommands, commandName)
	}
	return fmt.Sprintf("Options:\n  %s", strings.Join(subcommands, "\n  "))
}
