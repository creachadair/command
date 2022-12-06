// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

// Package command defines plumbing for command dispatch.
// It is based on and similar in design to the "go" command-line tool.
//
// # Overview
//
// The command package allows a program to easily process a simple language of
// named commands, each of which may have its own flags, arguments, and nested
// subcommands.  A command is represented by a *command.C value carrying help
// text, usage summaries, and a function to execute its behavior.
//
// The Run and RunOrFail functions parse the raw argument list of a program
// against a tree of *command.C values, parsing flags as needed and executing
// the selected command or printing appropriate diagnostics. Flags are parsed
// using the standard "flag" package by default.
package command

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
)

// Env is the environment passed to the Run function of a command.
// An Env implements the io.Writer interface, and should be used as
// the target of any diagnostic output the command wishes to emit.
// Primary command output should be sent to stdout.
type Env struct {
	Parent  *Env      // if this is a subcommand, its parent environment (or nil)
	Command *C        // the C value that carries the Run function
	Config  any       // configuration data
	Log     io.Writer // where to write diagnostic output (nil for os.Stderr)
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
//
// When a command is first discovered during argument traversal, its SetFlags
// hook is executed (if defined) to prepare its flag set.  Then, unless the
// CustomFlags option is true, the rest of the argument list is parsed by the
// FlagSet to separate command-specific flags from further arguments and/or
// subcommands.
//
// After flag processing and before attempting to explore subcommands, an Init
// hook is called if one is defined. If Init reports an error it terminates
// argument traversal, and that error is reported back to the user.
//
// After initialization, if there are any remaining non-flag arguments, we
// check for a matching subcommand.  If one is found, argument traversal recurs
// to that subcommand to process the rest of the command-line.
//
// Otherwise, if the command defines a Run hook, that hook is executed with the
// remaining unconsumed arguments. If no Run hook is defined, the traversal
// stops, logs a help message, and reports an error.
type C struct {
	// The name of the command, preferably one word. The name is during argument
	// processing to choose which command or subcommand to execute.
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

	// Perform the action of the command. If nil, calls FailWithUsage.
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

	isFlagSet bool // true if SetFlags was invoked
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
func (c *C) NewEnv(config any) *Env {
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

// ErrUsage is returned from Run if the user requested help.
var ErrUsage = errors.New("help requested")

type usageErr struct {
	env *Env
	msg string
}

func (u usageErr) Error() string { return string(u.msg) }

// Usagef returns a formatted error that describes a usage error for the
// command whose environment is e.
func (e *Env) Usagef(msg string, args ...any) error {
	return usageErr{env: e, msg: fmt.Sprintf(msg, args...)}
}

// RunOrFail behaves as Run, but prints a log message and calls os.Exit if the
// command reports an error. If the command succeeds, RunOrFail returns.
func RunOrFail(env *Env, rawArgs []string) {
	if err := Run(env, rawArgs); err != nil {
		if u, ok := err.(usageErr); ok {
			log.Printf("Error: %s", u.msg)
			u.env.Command.HelpInfo(false).WriteUsage(env)
		} else if !errors.Is(err, ErrUsage) {
			log.Printf("Error: %v", err)
			os.Exit(1)
		}
		os.Exit(2)
	}
}

// Run runs the command given unprocessed arguments. If the command has flags
// they are parsed and errors are handled before invoking the handler.
//
// Run writes usage information to ctx and returns ErrUsage if the command-line
// usage was incorrect or the user requested -help via flags.
func Run(env *Env, rawArgs []string) error {
	cmd := env.Command
	args := rawArgs

	// If the command defines a flag setter, invoke it.
	if cmd.SetFlags != nil && !cmd.isFlagSet {
		cmd.SetFlags(env, &cmd.Flags)
		cmd.isFlagSet = true
	}

	// Unless this command does custom flag parsing, parse the arguments and
	// check for errors before passing control to the handler.
	if !cmd.CustomFlags {
		cmd.Flags.Usage = func() {}
		err := cmd.Flags.Parse(rawArgs)
		if err == flag.ErrHelp {
			return printLongHelp(env, args, nil)
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
			return Run(env.newChild(sub), rest)
		} else if hasSub && len(rest) == 0 {
			// Show help for a topic subcommand with subcommands of its own.
			return printLongHelp(env.newChild(sub), nil, nil)
		} else if cmd.Run == nil {
			fmt.Fprintf(env, "Error: %s command %q not understood\n", cmd.Name, args[0])
			return ErrUsage
		}
	}
	if cmd.Run == nil {
		return printShortHelp(env, args)
	}
	return cmd.Run(env, args)
}
