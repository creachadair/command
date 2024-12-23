// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package command_test

import (
	"flag"
	"testing"

	"github.com/creachadair/command"
	"github.com/creachadair/mds/mtest"
	"github.com/google/go-cmp/cmp"
)

func TestAdapt(t *testing.T) {
	zero := command.Adapt(func(*command.Env) error { return nil })
	two := command.Adapt(func(_ *command.Env, a, b string) error { return nil })
	twoVar := command.Adapt(func(_ *command.Env, a, b string, more ...string) error { return nil })
	twoRest := command.Adapt(func(_ *command.Env, a, b string, rest []string) error { return nil })

	tests := []struct {
		name string
		run  func(*command.Env) error
		args []string
		ok   bool
	}{
		{"zeroNil", zero, nil, true},
		{"zeroEmpty", zero, []string{}, true},
		{"zeroOne", zero, []string{"one"}, false},

		{"twoNil", two, nil, false},
		{"twoOne", two, []string{"one"}, false},
		{"twoTwo", two, []string{"one", "two"}, true},
		{"twoThree", two, []string{"one", "two", "three"}, false},

		{"twoVarNil", twoVar, nil, false},
		{"twoVarTwo", twoVar, []string{"one", "two"}, true},
		{"twoVarThree", twoVar, []string{"one", "two", "three"}, true},
		{"twoVarFour", twoVar, []string{"one", "two", "three", "four"}, true},

		{"twoRestNil", twoRest, nil, false},
		{"twoRestTwo", twoRest, []string{"one", "two"}, true},
		{"twoRestThree", twoRest, []string{"one", "two", "three"}, true},
		{"twoRestFour", twoRest, []string{"one", "two", "three", "four"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &command.C{Name: "test", Run: tc.run}
			err := command.Run(c.NewEnv(nil), tc.args)
			if err != nil && tc.ok {
				t.Errorf("On args %+q: unexpected error: %v", tc.args, err)
			} else if err == nil && !tc.ok {
				t.Errorf("On args %+q: unexpected success", tc.args)
			}
		})
	}
}

func TestAdaptErrors(t *testing.T) {
	tests := []struct {
		name string
		fn   any
	}{
		{"Nil", nil},
		{"NonFunction", "foo"},
		{"NoEnv", func(string) {}},
		{"NoResult", func(*command.Env) {}},
		{"NotError", func(*command.Env) bool { return true }},
		{"NotString", func(*command.Env, bool) error { return nil }},
		{"WrongVar", func(*command.Env, string, string, ...int) error { return nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mtest.MustPanic(t, func() { command.Adapt(tc.fn) })
		})
	}
}

func TestFlags(t *testing.T) {
	type pair struct {
		Name  string
		Value int
	}

	f1, f2 := pair{Name: "f1"}, pair{Name: "f2"}
	c := &command.C{
		SetFlags: command.Flags(func(fs *flag.FlagSet, v any) {
			p := v.(*pair)
			fs.IntVar(&p.Value, p.Name, 1, "Test flag "+p.Name)
		}, &f1, &f2),
		Run: func(*command.Env) error { return nil },
	}
	if err := command.Run(c.NewEnv(nil), []string{"-f1", "101", "-f2=102", "ok"}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if diff := cmp.Diff(f1, pair{Name: "f1", Value: 101}); diff != "" {
		t.Errorf("After Run f1 (-got, +want):\n%s", diff)
	}
	if diff := cmp.Diff(f2, pair{Name: "f2", Value: 102}); diff != "" {
		t.Errorf("After Run f2 (-got, +want):\n%s", diff)
	}
}
