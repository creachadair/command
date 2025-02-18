package command_test

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/creachadair/command"
)

// The environment passed to a command can carry an arbitrary config value.
// Here we use a struct carrying information about options.
type options struct {
	noNewline bool
	label     string
	private   int
	confirm   bool
}

func Example() {
	root := &command.C{
		Name: "example",

		// Usage may have multiple lines, each describing a different mode of
		// operation. The command name can be omitted; the help printer will add
		// it if it is not present.
		Usage: "command args...\nhelp",

		// The first line of help text is used as a synopsis in help listings.
		// Any subsequent lines are include in "long" help.
		Help: `Do interesting things with arguments.

This program demonstrates the use of the command package.
This help text is printed by the "help" subcommand.`,

		// This hook is called (when defined) to set up flags.
		SetFlags: func(env *command.Env, fs *flag.FlagSet) {
			opt := env.Config.(*options)
			fs.StringVar(&opt.label, "label", "", "Label text")
			fs.IntVar(&opt.private, "p", 0, "PRIVATE: Unadvertised flag")
			fs.BoolVar(&opt.confirm, "y", false, "Confirm activity")
		},

		// Note that the "example" command does not have a Run function.
		// Executing it without a subcommand will print a help message and exit
		// with error.

		// Subcommands.
		Commands: []*command.C{
			// This is a typical user-defined command.
			{
				Name:  "echo",
				Usage: "text ...",
				Help:  "Concatenate the arguments with spaces and print to stdout.",

				SetFlags: func(env *command.Env, fs *flag.FlagSet) {
					// Pull the config value out of the environment and attach a flag to it.
					opt := env.Config.(*options)
					fs.BoolVar(&opt.noNewline, "n", false, "Do not print a trailing newline")
				},

				Run: func(env *command.Env) error {
					opt := env.Config.(*options)
					if opt.label != "" {
						fmt.Printf("[%s] ", opt.label)
					}
					if opt.private > 0 {
						fmt.Printf("<%d> ", opt.private)
					}
					fmt.Print(strings.Join(env.Args, " "))
					if !opt.noNewline {
						fmt.Println()
					}
					return nil
				},
			},

			{
				Name:  "secret",
				Usage: "args ...",
				Help:  "A command that is hidden from help listings.",

				// Exclude this command when listing subcommands.
				Unlisted: true,

				Run: func(env *command.Env) error {
					fmt.Printf("easter-egg %s\n", strings.Join(env.Args, ", "))
					return nil
				},
			},

			{
				Name: "fatal",

				// Demonstrate that a command which panics is properly handled.
				Run: func(env *command.Env) error { panic("ouch") },
			},

			// This function creates a basic "help" command that prints out
			// command help based on usage and help strings.
			//
			// This command can also have "help topics", which are stripped-down
			// commands with only help text (no flags, usage, or other behavior).
			command.HelpCommand([]command.HelpTopic{{
				Name: "special",
				Help: "This is some useful information a user might care about.",
			}, {
				Name: "magic",
				Help: `The user can write "command help <topic>" to get this text.`,
			}}),
		},
	}

	// To run a command, we need to construct an environment for it.
	// The environment can carry an arbitrary config value, which in this case
	// is our custom options type.
	var opts options
	command.RunOrFail(root.NewEnv(&opts), os.Args[1:])
}
