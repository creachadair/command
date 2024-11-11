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
	if !errors.Is(err, command.ErrRunPanicked) {
		t.Fatalf("Run: should panic, got error: %v", err)
	}
	if !strings.Contains(err.Error(), message) {
		t.Error("Panic error does not contain the probe string")
	}
}
