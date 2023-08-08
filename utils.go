// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package command

import (
	"flag"
	"strings"
)

// usageLines parses and normalizes usage lines. The command name is stripped
// from the head of each line if it is present.
func (c *C) usageLines() []string {
	var lines []string
	prefix := c.Name + " "
	for _, line := range strings.Split(c.Usage, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		} else if line == c.Name {
			lines = append(lines, "")
		} else {
			lines = append(lines, strings.TrimPrefix(line, prefix))
		}
	}
	return lines
}

// indent returns text indented as specified; first is prepended to the first
// line, and prefix to all subsequent lines.
func indent(first, prefix, text string) string {
	return first + strings.ReplaceAll(text, "\n", "\n"+prefix)
}

// FailWithUsage is a run function that logs a usage message for the command
// and returns ErrRequestHelp.
func FailWithUsage(env *Env) error {
	env.Command.HelpInfo(false).WriteUsage(env)
	return ErrRequestHelp
}

// MergeFlags merges the flags from the parents of env into the flag set for
// the command of env itself. Flags are considered from the innermost to the
// outermost environment in order, and Only flags not already defined are
// merged into the ongoing set.
//
// Note: Because flags are parsed during argument traversal, the value seen by
// the init callback of a parent command will not reflect values set in the
// arguments of a subcommand.
func MergeFlags(env *Env) {
	seen := make(map[string]struct{})
	fs := &env.Command.Flags
	fs.VisitAll(func(f *flag.Flag) { seen[f.Name] = struct{}{} })

	for cur := env.Parent; cur != nil; cur = cur.Parent {
		pf := &cur.Command.Flags
		pf.VisitAll(func(f *flag.Flag) {
			if _, ok := seen[f.Name]; ok {
				return
			}
			fs.Var(f.Value, f.Name, f.Usage)
			seen[f.Name] = struct{}{}
		})
	}
}
