// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package command_test

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/creachadair/command"
	"github.com/google/go-cmp/cmp"
)

var flags struct {
	A string
	B string
	C string
	D string
	E string
	F string
}

func setFlag(name string, s *string) func(*command.Env, *flag.FlagSet) {
	return func(_ *command.Env, fs *flag.FlagSet) {
		fs.StringVar(s, name, "", name)
	}
}

func newTestRoot(run func(*command.Env) error) *command.C {
	return &command.C{
		Name:     "root",
		SetFlags: setFlag("A", &flags.A),
		Run:      run,
		Commands: []*command.C{{
			Name:     "one",
			SetFlags: setFlag("B", &flags.B),
			Run:      run,
			Commands: []*command.C{{
				Name:     "two",
				SetFlags: setFlag("C", &flags.C),
				Run:      run,
				Commands: []*command.C{{
					Name:     "four",
					SetFlags: setFlag("E", &flags.E),
					Run:      run,
				}},
			}},
		}, {
			Name:     "three",
			SetFlags: setFlag("D", &flags.D),
			Run:      run,
		}, {
			Name:     "five",
			SetFlags: setFlag("A", &flags.F),
			Run: func(env *command.Env) error {
				if err := run(env); err != nil {
					return err
				} else if flags.F == "" {
					return errors.New("flag -F is empty")
				}
				return nil
			},
		}},
	}
}

func TestEnv_IsFlagSet(t *testing.T) {
	checkFlagSet := func(t *testing.T, name string, want bool) func(*command.Env) error {
		return func(env *command.Env) error {
			if ok := env.IsFlagSet(name); ok != want {
				t.Errorf("IsFlagSet(%q): got %v, want %v (args=%+q)", name, ok, want, env.Args)
			}
			return nil
		}
	}

	var stringFlag string
	var boolFlag bool
	tests := []struct {
		flagName string
		args     string
		wantSet  bool
	}{
		{"string", "", false},
		{"bool", "", false},
		{"string", "a b c", false},
		{"bool", "a b c", false},
		{"string", "--string new", true},
		{"string", "--string new a b c", true},
		{"string", "a b -string new c", true}, // merged is default
		{"bool", "--bool", true},
		{"bool", "-bool a b c", true},
		{"bool", "a -bool b c", true}, // merged is default
		{"string", "-string x -bool a b c", true},
		{"string", "-bool -string x a", true},
		{"bool", "-string x -bool b", true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s_%v", tc.flagName, tc.wantSet), func(t *testing.T) {
			root := &command.C{
				Name: "root",
				SetFlags: func(_ *command.Env, fs *flag.FlagSet) {
					fs.StringVar(&stringFlag, "string", "old", "String value")
					fs.BoolVar(&boolFlag, "bool", false, "Boolean value")
				},
				Run: checkFlagSet(t, tc.flagName, tc.wantSet),
			}
			args := strings.Fields(tc.args)
			if err := command.Run(root.NewEnv(nil), args); err != nil {
				t.Errorf("Command failed; %v", err)
			}
		})
	}

}

func TestParse(t *testing.T) {
	checkRun := func(wantArgs string) func(env *command.Env) error {
		return func(env *command.Env) error {
			if diff := cmp.Diff(env.Args, strings.Fields(wantArgs)); diff != "" {
				return fmt.Errorf("wrong args (-got, +want):\n%s", diff)
			}
			return nil
		}
	}
	const noError = ""
	const noSuchFlag = "flag provided but not defined"
	const missingArg = "flag needs an argument"
	const wrongArgs = "wrong args"
	tests := []struct {
		name     string
		doMerge  bool
		args     string
		wantArgs string
		wantErr  string
	}{
		{"rootNoFlags", false, "x y", "x y", noError},
		{"rootBadFlag", false, "--nonesuch x y", "", noSuchFlag},
		{"rootBadFlagMerged", true, "-nonesuch x", "", noSuchFlag},
		{"rootBadFlagStop", false, "-- --nonesuch x y", "--nonesuch x y", noError},
		{"rootA", false, "--A=1 x y", "x y", noError},
		{"rootADash", false, "-A - x y", "x y", noError},
		{"rootStop", false, "-- -A v x y", "-A v x y", noError},

		{"rootAMergedFront", true, "--A 1 x y", "x y", noError},
		{"rootAMergedMid", true, "x -A 1 y", "x y", noError},
		{"rootAMergedBack", true, "x y -A=1", "x y", noError},
		{"rootStopAtEnd", true, "-A 1 --", "", noError},
		{"oneStopAtEnd", true, "one -A 1 -B 2 --", "", noError},

		{"oneNoFlags", false, "one x y", "x y", noError},
		{"oneB", false, "one -B w x y", "x y", noError},
		{"oneBLast", false, "one x y -B w", "x y -B w", noError},
		{"oneBLastMerged", true, "one x y -B w", "x y", noError},
		{"oneMissing", false, "one -B", "x y", missingArg},

		{"oneABCDash", true, "-A - one -B - two x -C - y", "x y", noError},
		{"twoNoFlags", false, "one two x y", "x y", noError},
		{"twoMixed", false, "one two -C 2 -B 1 x y", "x y", noSuchFlag},
		{"twoMixedMerged", true, "one two -C 2 -B -1 x y", "x y", noError},
		{"twoAllFlags", false, "-A 1 one -B 2 two -C 3 x y", "x y", noError},
		{"twoMergedFlags", true, "one two -C 3 x --A=1 -B 2 y", "x y", noError},
		{"twoMergedFlagsNoStop", true, "one two -C 3 x --A=1 -B 2 y", "x y", noError},
		{"twoMergedFlagsStop0", true, "-- one two -C 3 x --A=1 -B 2 y", "x --A=1 y", noError},
		{"twoMergedFlagsStopA", true, "one -- two -C 3 x --A=1 -B 2 y", "x -B 2 y", noError},
		{"twoMergedFlagsStopB", true, "one two -- -C 3 x --A=1 -B 2 y", "-C 3 x y", noError},
		{"twoMergedFlagsStopC", true, "one two -C 3 -- x --A=1 -B 2 y", "x y", noError},

		{"threeCDMerged", true, "three -D 4 -C 3", "x y", noSuchFlag},
		{"threeBadFlagMerged", true, "three -D=1 -other x y", "x y", noSuchFlag},
		{"threeOtherArgMerged", true, "three -D=1 -- -other x y", "x y", wrongArgs},

		{"fourAllMixedMerged", true, "one -A=4 two -B=3 -C=2 four x --E 1 y", "x y", noError},
		{"fourAllFrontMerged", true, "one two four x y -E=1 -C 2 --B=3 --A 4", "x y", noError},
		{"fourBadFlagMerged", true, "-A 1 one two -Q ? four x y", "x y", noSuchFlag},

		// Verify that if a subcommand defines the same flag, we can prevent
		// it from being "poached" by the enclosing command by using "--".
		{"fiveFlagPoachedEarly", true, "-A ok five x y -A alt", "x y", "flag -F is empty"},
		{"fiveFlagPoachedLate", true, "five x y -A alt", "x y", "flag -F is empty"},
		{"fiveFlagUnpoachedEarly", true, "-A ok -- five x y -A alt", "x y", noError},
		{"fiveFlagUnpoachedLate", true, "-- five x y -A alt", "x y", noError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestRoot(checkRun(tc.wantArgs)).NewEnv(nil).MergeFlags(tc.doMerge)
			args := strings.Fields(tc.args)
			err := command.Run(env, args)
			if tc.wantErr != "" {
				if err == nil {
					t.Error("Run did not report an error as it should")
				} else if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Got error %v, want %q", err, tc.wantErr)
				} else {
					t.Logf("Got expected error: %v", err)
				}
			} else if err != nil {
				t.Errorf("Run: unexpected error: %v", err)
			}
		})
	}
}

func TestHelpFlag(t *testing.T) {
	// A --help flag should be recognized even if it is not defined by the flag
	// set, as long as it occurs before the non-flag arguments.
	root := &command.C{
		Name: "cmd",
		Commands: []*command.C{{
			Name: "sub",
			SetFlags: func(_ *command.Env, fs *flag.FlagSet) {
				fs.Bool("foo", false, "A flag for testing")
			},
			Run: func(env *command.Env) error { return nil },
		}},
	}
	tests := []struct {
		args    string
		wantErr string
	}{
		{"sub", ""},
		{"sub - --help", ""},  // free args: - --help
		{"sub -- --help", ""}, // free args: --help
		{"sub --foo -help", "help requested"},
		{"sub --help", "help requested"},
		{"sub -foo --help x y -bar", "help requested"},
		{"sub -foo --help", "help requested"},
		{"sub -foo -bar", "not defined"},
		{"sub -foo -help -bar", "help requested"},
		{"sub -help", "help requested"},
		{"sub a b -help", "help requested"},
		{"sub -foo -- -bar", ""}, // free args: -bar
	}
	for _, tc := range tests {
		for _, ok := range []bool{false, true} {
			env := root.NewEnv(nil).MergeFlags(ok)
			env.Log = io.Discard
			args := strings.Fields(tc.args)
			err := command.Run(env, args)
			if tc.wantErr == "" && err != nil {
				t.Errorf("Run merge=%v %q: unexpected error: %v", ok, tc.args, err)
			} else if err != nil && !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Run merge=%v %q: got error %v, want %q", ok, tc.args, err, tc.wantErr)
			}
		}
	}
}
