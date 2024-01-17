// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package command_test

import (
	"errors"
	"flag"
	"strings"
	"testing"

	"github.com/creachadair/command"
	"github.com/google/go-cmp/cmp"
)

func TestRun_panic(t *testing.T) {
	const message = "omg the sky is falling"
	cmd := &command.C{
		Name: "freak-out",
		Run: func(*command.Env) error {
			panic(message)
		},
	}
	err := command.Run(cmd.NewEnv(nil), []string{"freak-out"})
	t.Logf("Error reported by run: %v", err)

	var got command.PanicError
	if !errors.As(err, &got) {
		t.Fatalf("Run: got error %[1]% %[1]v, want PanicError", err)
	}
	if !strings.Contains(err.Error(), message) {
		t.Error("Panic error does not contain the probe string")
	}
	if got := got.Value(); got != message {
		t.Errorf("Panic value: got %v, want %v", got, message)
	}
	if env := got.Env(); env.Command != cmd {
		t.Errorf("Panic env: got %+v, want %+v", env.Command, cmd)
	}
	t.Log("--- Captured panic stack (not a panic in the test, don't worry):\n", got.Stack())
}

func TestInfo(t *testing.T) {
	cmd := &command.C{
		Name:  "root",
		Usage: "root usage",
		Help:  "root help",
		SetFlags: func(_ *command.Env, fs *flag.FlagSet) {
			fs.String("s", "", "String flag")
			fs.Int("z", 25, "PRIVATE:Integer flag")
			fs.Bool("b", false, "Boolean flag")
		},
		Commands: []*command.C{
			{
				Name:     "unlisted",
				Usage:    "unlisted usage",
				Help:     "unlisted help",
				Unlisted: true,
				SetFlags: func(_ *command.Env, fs *flag.FlagSet) { fs.Float64("q", 0.1, "Float flag") },
				Run:      func(*command.Env) error { return nil }, // runnable
			},
			{
				Name:  "listed",
				Usage: "listed usage",
				Help:  "listed help",
				Run:   func(*command.Env) error { return nil }, // runnable
			},
		},
	}
	tests := []struct {
		name  string
		flags command.HelpFlags
		want  *command.CInfo
	}{
		{
			name:  "Default",
			flags: 0,
			want: &command.CInfo{
				Name:  "root",
				Usage: []string{"usage"}, // command name trimmed
				Help:  "root help",
				Flags: []command.FlagInfo{
					{Name: "b", Usage: "Boolean flag", DefaultString: "false"},
					{Name: "s", Usage: "String flag", DefaultString: ""},

					// Not z, as it is marked private.
				},
				// no subcommands, they are not requested
			},
		},
		{
			name:  "Commands",
			flags: command.IncludeCommands,
			want: &command.CInfo{
				Name:  "root",
				Usage: []string{"usage"}, // command name trimmed
				Help:  "root help",
				Flags: []command.FlagInfo{
					{Name: "b", Usage: "Boolean flag", DefaultString: "false"},
					{Name: "s", Usage: "String flag", DefaultString: ""},

					// Not z, as it is marked private.
				},
				Commands: []*command.CInfo{{
					Name:     "listed",
					Usage:    []string{"usage"},
					Help:     "listed help",
					Runnable: true,
				}},
			},
		},
		{
			name:  "Private",
			flags: command.IncludePrivateFlags,
			want: &command.CInfo{
				Name:  "root",
				Usage: []string{"usage"}, // command name trimmed
				Help:  "root help",
				Flags: []command.FlagInfo{
					{Name: "b", Usage: "Boolean flag", DefaultString: "false"},
					{Name: "s", Usage: "String flag", DefaultString: ""},
					{Name: "z", Usage: "Integer flag", DefaultString: "25"},
				},
				// no subcommands, they are not requested
			},
		},
		{
			name:  "All",
			flags: command.IncludeAll,
			want: &command.CInfo{
				Name:  "root",
				Usage: []string{"usage"}, // command name trimmed
				Help:  "root help",
				Flags: []command.FlagInfo{
					{Name: "b", Usage: "Boolean flag", DefaultString: "false"},
					{Name: "s", Usage: "String flag", DefaultString: ""},
					{Name: "z", Usage: "Integer flag", DefaultString: "25"},
				},
				Commands: []*command.CInfo{{
					Name:     "unlisted",
					Usage:    []string{"usage"},
					Help:     "unlisted help",
					Runnable: true,
					Unlisted: true,
					Flags:    []command.FlagInfo{{Name: "q", Usage: "Float flag", DefaultString: "0.1"}},
				}, {
					Name:     "listed",
					Usage:    []string{"usage"},
					Help:     "listed help",
					Runnable: true,
				}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(cmd.Info(tc.flags), tc.want); diff != "" {
				t.Errorf("Wrong output (-got, +want):\n%s", diff)
			}
		})
	}
}
