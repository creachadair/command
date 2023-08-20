package command_test

import (
	"testing"

	"github.com/creachadair/command"
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
			defer func() {
				if v := recover(); v != nil {
					t.Logf("Recovered panic (OK): %v", v)
				}
			}()
			command.Adapt(tc.fn)
			t.Fatal("Adapt did not panic as it should")
		})
	}
}
