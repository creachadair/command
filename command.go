// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

// Package command defines plumbing for command dispatch.
// It is based on and similar in design to the "go" command-line tool.
package command

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// Env is the environment passed to the Run function of a command.
// An Env implements the io.Writer interface, and should be used as
// the target of any diagnostic output the command wishes to emit.
// Primary command output should be sent to stdout.
type Env struct {
	Parent  *Env        // if this is a subcommand, its parent environment (or nil)
	Command *C          // the C value that carries the Run function
	Config  interface{} // configuration data
	Log     io.Writer   // where to write diagnostic output (nil for os.Stderr)
}

// output returns the log writer for c.
func (e *Env) output() io.Writer {
	if e.Log != nil {
		return e.Log
	}
	return os.Stderr
}

func (e *Env) newChild(cmd *C) *Env {
	cp := *e // shallow copy
	cp.Command = cmd
	cp.Parent = e
	return &cp
}

// Write implements the io.Writer interface. Writing to a context writes to its
// designated output stream, allowing the context to be sent diagnostic output.
func (e *Env) Write(data []byte) (int, error) {
	return e.output().Write(data)
}

// C carries the description and invocation function for a command.
type C struct {
	// The name of the command, preferably one word.
	Name string

	// A terse usage summary for the command. Multiple lines are allowed, but
	// each line should be self-contained for a particular usage sense.
	Usage string

	// A detailed description of the command. Multiple lines are allowed.
	// The first non-blank line of this text is used as a synopsis.
	Help string

	// Flags parsed from the raw argument list. This will be initialized before
	// Init or Run is called unless CustomFlags is true.
	Flags flag.FlagSet

	// If true, the command is responsible for flag parsing.
	CustomFlags bool

	// Execute the action of the command. If nil, calls FailWithUsage.
	Run func(env *Env, args []string) error

	// If set, this will be called before flags are parsed, to give the command
	// an opportunity to set flags.
	SetFlags func(env *Env, fs *flag.FlagSet)

	// If set, this will be called after flags are parsed (if any) but before
	// any subcommands are processed. If it reports an error, execution stops
	// and that error is returned to the caller.
	Init func(env *Env) error

	// Subcommands of this command.
	Commands []*C
}

// Runnable reports whether the command has any action defined.
func (c *C) Runnable() bool { return c != nil && (c.Run != nil || c.Init != nil) }

// HasRunnableSubcommands reports whether c has any runnable subcommands.
func (c *C) HasRunnableSubcommands() bool {
	if c != nil {
		for _, cmd := range c.Commands {
			if cmd.Runnable() {
				return true
			}
		}
	}
	return false
}

// NewEnv returns a new root context for c with the optional config value.
func (c *C) NewEnv(config interface{}) *Env {
	return &Env{Command: c, Config: config}
}

// FindSubcommand returns the subcommand of c matching name, or nil.
func (c *C) FindSubcommand(name string) *C {
	for _, cmd := range c.Commands {
		if cmd.Name == name {
			return cmd
		}
	}
	return nil
}

// ErrUsage is returned from Execute if the user requested help.
var ErrUsage = errors.New("help requested")

// Execute runs the command given unprocessed arguments. If the command has
// flags they are parsed and errors are handled before invoking the handler.
//
// Execute writes usage information to ctx and returns ErrUsage if the
// command-line usage was incorrect or the user requested -help via flags.
func Execute(env *Env, rawArgs []string) error {
	cmd := env.Command
	args := rawArgs

	// If the command defines a flag setter, invoke it.
	if cmd.SetFlags != nil {
		cmd.SetFlags(env, &cmd.Flags)
	}

	// Unless this command does custom flag parsing, parse the arguments and
	// check for errors before passing control to the handler.
	if !cmd.CustomFlags {
		cmd.Flags.Usage = func() {}
		err := cmd.Flags.Parse(rawArgs)
		if err == flag.ErrHelp {
			return runShortHelp(env, args)
		} else if err != nil {
			return err
		}
		args = cmd.Flags.Args()
	}

	if cmd.Init != nil {
		if err := cmd.Init(env); err != nil {
			return fmt.Errorf("initializing %q: %v", cmd.Name, err)
		}
	}

	// Unclaimed (non-flag) arguments may be free arguments for this command, or
	// may belong to a subcommand.
	if len(args) != 0 {
		sub, rest := cmd.FindSubcommand(args[0]), args[1:]
		hasSub := sub.HasRunnableSubcommands()

		if sub.Runnable() || (hasSub && len(rest) != 0) {
			// A runnable subcommand takes precedence.
			return Execute(env.newChild(sub), rest)
		} else if hasSub && len(rest) == 0 {
			// Show help for a topic subcommand with subcommands of its own.
			return runLongHelp(env.newChild(sub), nil)
		} else if cmd.Run == nil {
			fmt.Fprintf(env, "Error: %s command %q not understood\n", cmd.Name, args[0])
			return ErrUsage
		}
	}
	if cmd.Run == nil {
		return runLongHelp(env, args)
	}
	return cmd.Run(env, args)
}
