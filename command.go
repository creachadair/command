// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

// Package command defines plumbing for command dispatch.
// It is based on and similar in design to the "go" command-line tool.
//
// # Overview
//
// The command package helps a program to process a language of named
// commands, each of which may have its own flags, arguments, and nested
// subcommands.  A command is represented by a *command.C value carrying help
// text, usage summaries, and a function to execute its behavior.
//
// The Run and RunOrFail functions parse the raw argument list of a program
// against a tree of *command.C values, parsing flags as needed and executing
// the selected command or printing appropriate diagnostics. Flags are parsed
// using the standard "flag" package by default.
package command

import (
	"context"
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
	Args    []string  // the unclaimed command-line arguments
	Log     io.Writer // where to write diagnostic output (nil for os.Stderr)

	ctx    context.Context
	cancel context.CancelCauseFunc
	merge  bool
	hflag  HelpFlags // default: no unlisted commands, no private flags
}

// Context returns the context associated with e. If e does not have its own
// context, it returns the context of its parent, or if e has no parent it
// returns a new background context.
func (e *Env) Context() context.Context {
	if e.ctx != nil {
		return e.ctx
	} else if e.Parent == nil {
		return context.Background()
	}
	return e.Parent.Context()
}

// Cancel cancels the context associated with e with the given cause.
// If e does not have its own context, the cancellation is propagated to its
// parent if one exists. If e has no parent and no context, Cancel does nothing
// without error.
func (e *Env) Cancel(cause error) {
	if e.cancel != nil {
		e.cancel(cause)
	} else if e.Parent != nil {
		e.Parent.Cancel(cause)
	}
}

// SetContext sets the context of e to ctx and returns e.  If ctx == nil it
// clears the context of e so that it defaults to its parent (see Context).
func (e *Env) SetContext(ctx context.Context) *Env {
	if ctx == nil {
		e.ctx = nil
		e.cancel = nil
	} else {
		e.ctx, e.cancel = context.WithCancelCause(ctx)
	}
	return e
}

// MergeFlags sets the flag merge option for e and returns e.
//
// Setting this option true modifies the flag parsing algorithm for commands
// dispatched through e to "merge" flags matching the current command from the
// remainder of the argument list. The default is false.
//
// Merging allows flags for a command to be defined later in the command-line,
// after subcommands and their own flags.  For example, given a command "one"
// with flag -A and a subcommand "two" with flag -B: With merging false, the
// following arguments report an error.
//
//	one two -B 2 -A 1
//
// This is because the default parsing algorithm (without merge) stops parsing
// flags for "one" at the first non-flag argument, and "two" does not recognize
// the flag -A. With merging enabled the argument list succeeds, because the
// parser "looks ahead", treating it as if the caller had written:
//
//	one -A 1 -- two -B 2
//
// Setting the MergeFlags option also applies to all the descendants of e
// unless the command's Init callback changes the setting.  Note that if a
// subcommand defines a flag with the same name as its ancestor, the ancstor
// will shadow the flag for the descendant.
func (e *Env) MergeFlags(merge bool) *Env { e.merge = merge; return e }

// HelpFlags sets the base help flags for e and returns e.
//
// By default, help listings do not include unlisted commands or private flags.
// This permits the caller to override the default help printing rules.
func (e *Env) HelpFlags(f HelpFlags) *Env { e.hflag = (f &^ IncludeCommands); return e }

// output returns the log writer for c.
func (e *Env) output() io.Writer {
	if e.Log != nil {
		return e.Log
	}
	return os.Stderr
}

func (e *Env) newChild(cmd *C, cargs []string) *Env {
	cp := *e // shallow copy
	cp.Command = cmd
	cp.Parent = e
	cp.Args = cargs
	return &cp
}

// Write implements the io.Writer interface. Writing to a context writes to its
// designated output stream, allowing the context to be sent diagnostic output.
func (e *Env) Write(data []byte) (int, error) {
	return e.output().Write(data)
}

// parseFlags parses flags from rawArgs using the flag set from env.Command.
// If parsing succeeds, it updates env.Args.
// If the command specifies custom flags, this is a no-op without error.
func (e *Env) parseFlags(rawArgs []string) error {
	if e.Command.CustomFlags {
		return nil
	}
	e.Command.Flags.Usage = func() {}
	e.Command.Flags.SetOutput(io.Discard)
	toParse := rawArgs
	if e.merge {
		flags, free, err := splitFlags(&e.Command.Flags, rawArgs)
		if err != nil {
			return err
		}
		toParse = joinArgs(flags, free)
	}
	err := e.Command.Flags.Parse(toParse)
	if errors.Is(err, flag.ErrHelp) {
		return printLongHelp(e, nil)
	} else if err != nil {
		return err
	}
	e.Args = e.Command.Flags.Args()
	return nil
}

// C carries the description and invocation function for a command.
//
// To process a command-line, the Run function walks through the arguments
// starting from a root command to discover which command should be run and
// what flags it requires. This argument traversal proceeds in phases:
//
// When a command is first discovered during argument traversal, its SetFlags
// hook is executed (if defined) to prepare its flag set.  Then, unless the
// CustomFlags option is true, the rest of the argument list is parsed using
// the FlagSet to separate command-specific flags from further arguments and/or
// subcommands.
//
// After flags are parsed and before attempting to explore subcommands, the
// current command's Init hook is called (if defined). If Init reports an error
// it terminates argument traversal, and that error is reported back to the
// user.
//
// Next, if there are any remaining non-flag arguments, Run checks whether the
// current command has a subcommand matching the first argument.  If so
// argument traversal recurs into that subcommand to process the rest of the
// command-line.
//
// Otherwise, if the command defines a Run hook, that hook is executed with the
// remaining unconsumed arguments. If no Run hook is defined, the traversal
// stops, logs a help message, and reports an error.
type C struct {
	// The name of the command, preferably one word. The name is during argument
	// processing to choose which command or subcommand to execute.
	Name string

	// A terse usage summary for the command. Multiple lines are allowed.
	// Each line should be self-contained for a particular usage sense.
	//
	// The name of the command will be automatically inserted at the front of
	// each usage line if it is not present. If no usage is defined, the help
	// mechanism will generate a default based on the presence of flags and
	// subcommands.
	Usage string

	// A detailed description of the command. Multiple lines are allowed.
	// The first non-blank line of this text is used as a synopsis; the whole
	// string is printed for long help.
	Help string

	// Flags parsed from the raw argument list. This will be initialized before
	// Init or Run is called.
	Flags flag.FlagSet

	// If false, Flags is used to parse the argument list.  Otherwise, the Init
	// function is responsible for parsing flags from the argument list.
	CustomFlags bool

	// If true, exclude this command from help listings unless it is explicitly
	// named and requested.
	Unlisted bool

	// Perform the action of the command. If nil, calls FailWithUsage.
	Run func(env *Env) error

	// If set, this will be called before flags are parsed, to give the command
	// an opportunity to set flags.
	SetFlags func(env *Env, fs *flag.FlagSet)

	// If set, this will be called after flags are parsed (if any) but before
	// any subcommands are processed. If it reports an error, execution stops
	// and that error is returned to the caller.
	//
	// The Init callback is permitted to modify env, and any such modifications
	// will persist through the rest of the invocation.
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
func (c *C) NewEnv(config any) *Env { return &Env{Command: c, Config: config} }

// FindSubcommand returns the subcommand of c matching name, or nil.
func (c *C) FindSubcommand(name string) *C {
	for _, cmd := range c.Commands {
		if cmd.Name == name {
			return cmd
		}
	}
	return nil
}

// ErrRequestHelp is returned from Run if the user requested help.
var ErrRequestHelp = errors.New("help requested")

// UsageError is the concrete type of errors reported by the Usagef function,
// indicating an error in the usage of a command.
type UsageError struct {
	Env     *Env
	Message string
}

func (u UsageError) Error() string { return string(u.Message) }

// Usagef returns a formatted error that describes a usage error for the
// command whose environment is e. The result has concrete type UsageError.
func (e *Env) Usagef(msg string, args ...any) error {
	return UsageError{Env: e, Message: fmt.Sprintf(msg, args...)}
}

// RunOrFail behaves as Run, but prints a log message and calls os.Exit if the
// command reports an error. If the command succeeds, RunOrFail returns.
//
// If a command reports a UsageError or ErrRequestHelp, the exit code is 2.
// For any other error the exit code is 1.
func RunOrFail(env *Env, rawArgs []string) {
	if err := Run(env, rawArgs); err != nil {
		var uerr UsageError
		if errors.As(err, &uerr) {
			log.Printf("Error: %s", uerr.Message)
			uerr.Env.Command.HelpInfo(env.hflag).WriteUsage(uerr.Env)
		} else if !errors.Is(err, ErrRequestHelp) {
			log.Printf("Error: %v", err)
			os.Exit(1)
		}
		os.Exit(2)
	}
}

// Run traverses the given unprocessed arguments starting from env.
// See the documentation for type C for a description of argument traversal.
//
// Run writes usage information to env and returns a UsageError if the
// command-line usage was incorrect, or ErrRequestHelp if the user requested
// help via the --help flag.
func Run(env *Env, rawArgs []string) (err error) {
	defer env.Cancel(err)
	cmd := env.Command
	env.Args = rawArgs

	// If the command defines a flag setter, invoke it.
	cmd.setFlags(env, &cmd.Flags)

	// Unless this command does custom flag parsing, parse the arguments and
	// check for errors before passing control to the handler.
	if err := env.parseFlags(rawArgs); err != nil {
		return err
	}

	if cmd.Init != nil {
		if err := cmd.Init(env); err != nil {
			return fmt.Errorf("initializing %q: %v", cmd.Name, err)
		}
	}

	// Unclaimed (non-flag) arguments may be free arguments for this command, or
	// may belong to a subcommand.
	if len(env.Args) != 0 {
		sub, rest := cmd.FindSubcommand(env.Args[0]), env.Args[1:]
		hasSub := sub.HasRunnableSubcommands()

		if sub.Runnable() || (hasSub && len(rest) != 0) {
			// A runnable subcommand takes precedence.
			return Run(env.newChild(sub, rest), rest)
		} else if hasSub && len(rest) == 0 {
			// Show help for a topic subcommand with subcommands of its own.
			return printLongHelp(env.newChild(sub, rest), nil)
		} else if cmd.Run == nil {
			fmt.Fprintf(env, "Error: %s command %q not understood\n", cmd.Name, env.Args[0])
			return ErrRequestHelp
		}
	}
	if cmd.Run == nil {
		return printShortHelp(env)
	}
	return cmd.Run(env)
}
