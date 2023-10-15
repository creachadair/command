package command_test

import (
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
		}},
	}
}

func TestParse(t *testing.T) {
	wantArgs := []string{"x", "y"}
	root := newTestRoot(func(env *command.Env) error {
		if diff := cmp.Diff(env.Args, wantArgs); diff != "" {
			return fmt.Errorf("wrong args (-got, +want):\n%s", diff)
		}
		return nil
	})
	env := func(merge bool) *command.Env { return root.NewEnv(nil).MergeFlags(merge) }

	const noError = ""
	const noSuchFlag = "flag provided but not defined"
	const missingArg = "flag needs an argument"
	const wrongArgs = "wrong args"
	tests := []struct {
		name    string
		env     *command.Env
		args    string
		wantErr string
	}{
		{"rootNoFlags", env(false), "x y", noError},
		{"rootBadFlag", env(false), "--nonesuch x y", noSuchFlag},
		{"rootBadFlagMerged", env(true), "-nonesuch x", noSuchFlag},
		{"rootA", env(false), "--A=1 x y", noError},
		{"rootADash", env(false), "-A - x y", noError},

		{"rootAMergedFront", env(true), "--A 1 x y", noError},
		{"rootAMergedMid", env(true), "x -A 1 y", noError},
		{"rootAMergedBack", env(true), "x y -A=1", noError},

		{"oneNoFlags", env(false), "one x y", noError},
		{"oneB", env(false), "one -B w x y", noError},
		{"oneBLast", env(false), "one x y -B w", wrongArgs},
		{"oneBLastMerged", env(true), "one x y -B w", noError},
		{"oneMissing", env(false), "one -B", missingArg},

		{"twoNoFlags", env(false), "one two x y", noError},
		{"twoMixed", env(false), "one two -C 2 -B 1 x y", noSuchFlag},
		{"twoMixedMerged", env(true), "one two -C 2 -B -1 x y", noError},
		{"twoAllFlags", env(false), "-A 1 one -B 2 two -C 3 x y", noError},
		{"twoMergedFlags", env(true), "one two -C 3 x --A=1 -B 2 y", noError},
		{"oneABCDash", env(true), "-A - one -B - two x -C - y", noError},

		{"threeCDMerged", env(true), "three -D 4 -C 3", noSuchFlag},
		{"threeBadFlagMerged", env(true), "three -D=1 -other x y", noSuchFlag},
		{"threeOtherArgMerged", env(true), "three -D=1 -- -other x y", wrongArgs},

		{"fourAllMixedMerged", env(true), "one -A=4 two -B=3 -C=2 four x --E 1 y", noError},
		{"fourAllFrontMerged", env(true), "one two four x y -E=1 -C 2 --B=3 --A 4", noError},
		{"fourBadFlagMerged", env(true), "-A 1 one two -Q ? four x y", noSuchFlag},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := strings.Fields(tc.args)
			err := command.Run(tc.env, args)
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
