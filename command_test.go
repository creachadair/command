// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package command_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/creachadair/command"
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
