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

// Context is the environment passed to the Run function of a command.
// A Context implements the io.Writer interface, and should be used as
// the target of any diagnostic output the command wishes to emit.
// Primary command output should be sent to stdout.
type Context struct {
	Parent  *Context    // if this is a subcommand, its parent context (or nil)
	Command *C          // the C value that carries the Run function
	Config  interface{} // configuration data
	Log     io.Writer   // where to write diagnostic output (nil for os.Stderr)
}

// output returns the log writer for c.
func (c *Context) output() io.Writer {
	if c.Log != nil {
		return c.Log
	}
	return os.Stderr
}

func (c *Context) newChild(cmd *C) *Context {
	cp := *c // shallow copy
	cp.Command = cmd
	cp.Parent = c
	return &cp
}

// Write implements the io.Writer interface. Writing to a context writes to its
// designated output stream, allowing the context to be sent diagnostic output.
func (c *Context) Write(data []byte) (int, error) {
	return c.output().Write(data)
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
	Run func(ctx *Context, args []string) error

	// If set, this will be called after flags are parsed (if any) but before
	// any subcommands are processed. If it reports an error, execution stops
	// and that error is returned to the caller.
	Init func(ctx *Context) error

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

// NewContext returns a new root context for c with the optional config value.
func (c *C) NewContext(config interface{}) *Context {
	return &Context{Command: c, Config: config}
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
func Execute(ctx *Context, rawArgs []string) error {
	cmd := ctx.Command
	args := rawArgs

	// Unless this command does custom flag parsing, parse the arguments and
	// check for errors before passing control to the handler.
	if !cmd.CustomFlags {
		cmd.Flags.Usage = func() {}
		err := cmd.Flags.Parse(rawArgs)
		if err == flag.ErrHelp {
			return runShortHelp(ctx, args)
		} else if err != nil {
			return err
		}
		args = cmd.Flags.Args()
	}

	if cmd.Init != nil {
		if err := cmd.Init(ctx); err != nil {
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
			return Execute(ctx.newChild(sub), rest)
		} else if hasSub && len(rest) == 0 {
			// Show help for a topic subcommand with subcommands of its own.
			return runLongHelp(ctx.newChild(sub), nil)
		} else if cmd.Run == nil {
			fmt.Fprintf(ctx, "Error: %s command %q not understood\n", cmd.Name, args[0])
			return ErrUsage
		}
	}
	if cmd.Run == nil {
		return runLongHelp(ctx, args)
	}
	return cmd.Run(ctx, args)
}
