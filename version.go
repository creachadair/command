// Copyright (C) 2022 Michael J. Fromberger. All Rights Reserved.

package command

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
)

// VersionCommand constructs a standardized version command that prints version
// metadata from the running binary to stdout. The caller can safely modify the
// returned command to customize its behavior.
func VersionCommand() *C {
	return &C{
		Name: "version",
		Help: `Print build version information for this program and exit.`,
		Run:  runVersion,
	}
}

// runVersion implements the built-in "version" command.
func runVersion(env *Env, args []string) error {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return errors.New("no version information is available")
	}
	rev := "(unknown)"
	time := "(unknown)"
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			time = s.Value
		}
	}
	fmt.Printf("%s built by %s at time %s rev %s\n",
		filepath.Base(os.Args[0]), bi.GoVersion, time, rev)
	return ErrUsage
}
