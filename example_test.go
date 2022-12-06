package command_test

import (
	"flag"
	"fmt"
	"strings"

	"github.com/creachadair/command"
)

func Example() {
	var noNewline bool

	root := &command.C{
		Name: "example",

		// Usage may have multiple lines, and can omit the command name.
		Usage: "command args...",

		// The first line of the help text is used as "short" help.
		// Any subsequent lines are include in "long" help.
		Help: `Do interesting things with arguments.

This program demonstrates the use of the command package.
This help text is printed by the "help" subcommand.`,

		// Note that the "example" command does not have a Run function.
		// Executing it without a subcommand will print a help message and exit
		// with error.

		Commands: []*command.C{
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

			// This is a typical user-defined command.
			{
				Name:  "echo",
				Usage: "text ...",
				Help:  "Concatenate the arguments with spaces and print to stdout.",

				SetFlags: func(env *command.Env, fs *flag.FlagSet) {
					fs.BoolVar(&noNewline, "n", false, "Do not print a trailing newline")
				},

				Run: func(env *command.Env, args []string) error {
					fmt.Print(strings.Join(args, " "))
					if !noNewline {
						fmt.Println()
					}
					return nil
				},
			},
		},
	}

	// Demonstrate help output.
	command.Run(root.NewEnv(nil), []string{"help"})

	command.RunOrFail(root.NewEnv(nil), []string{"echo", "foo", "bar"})
	command.RunOrFail(root.NewEnv(nil), []string{"echo", "-n", "baz"})
	// Output:
	// foo bar
	// baz
}
