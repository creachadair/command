package command_test

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/creachadair/command"
)

func Example() {
	// The environment passed to a command can carry an arbitrary config value.
	// Here we use a struct carrying information about options.
	type options struct {
		noNewline bool
		label     string
		private   int
		confirm   bool
	}

	root := &command.C{
		Name: "example",

		// Usage may have multiple lines, and can omit the command name.
		Usage: "command args...",

		// The first line of the help text is used as "short" help.
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
		},
	}

	// For purposes of the test output, discard help output.
	null, err := os.Create("/dev/null")
	if err != nil {
		log.Fatal(err)
	}
	save := os.Stdout
	os.Stdout = null

	// Demonstrate help output.
	//
	// Note that the argument to NewEnv is plumbed via the Config field of Env.

	var opt options
	env := root.NewEnv(&opt)
	env.Log = io.Discard

	command.Run(env, []string{"help"})
	opt = options{}  // reset settings
	os.Stdout = save // restore stdout

	// Requesting help for an unlisted subcommand reports an error.
	command.Run(env, []string{"help", "secret"})
	opt = options{}

	// But if you name the command explicitly with -help, you get help.
	command.Run(env, []string{"secret", "-help"})
	opt = options{}

	// Execute a command with some arguments.
	command.RunOrFail(env, []string{"echo", "foo", "bar"})
	opt = options{}

	// Execute a command with some flags and argujments.
	command.RunOrFail(env, []string{"-label", "xyzzy", "echo", "bar"})
	opt = options{}

	// Merged flags can be used anywhere in their scope.
	command.RunOrFail(env, []string{"echo", "-label", "foo", "bar"})
	opt = options{}

	// Private-marked flags still work as expected.
	command.RunOrFail(env, []string{"echo", "-p", "25", "-label", "ok", "bar"})
	opt = options{}

	// Executing an unlisted command works.
	command.RunOrFail(env, []string{"secret", "fort"})
	opt = options{}

	// An unmerged flag.
	command.RunOrFail(env, []string{"echo", "-n", "baz"})
	// Output:
	// foo bar
	// [xyzzy] bar
	// [foo] bar
	// [ok] <25> bar
	// easter-egg fort
	// baz
}
