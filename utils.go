// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package command

import (
	"flag"
	"fmt"
	"strings"
)

// Flags returns a SetFlags function that calls bind(fs, v) with the flag set
// and the given value v.
func Flags(bind func(*flag.FlagSet, any), v any) func(*Env, *flag.FlagSet) {
	return func(_ *Env, fs *flag.FlagSet) { bind(fs, v) }
}

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
	env.Command.HelpInfo(0).WriteUsage(env)
	return ErrRequestHelp
}

// MergeFlags merges the flags from the parents of env into the flag set for
// the command of env itself. Flags are considered from the innermost to the
// outermost environment in order, and only flags not already defined are
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

// splitFlags constructs two slices from args, the first containing all flags
// and their arguments matched by fs, the second containing all the other free
// arguments. Flag values are not parsed. Flag-shaped strings not matched by fs
// are treated as free arguments.  An error is reported if a flag lacks its
// argument.
func splitFlags(fs *flag.FlagSet, args []string) (flags, free []string, _ error) {
	var wantArg bool
	for i, s := range args {
		// Terminate flag processing from an explicit "--"
		// Include the split point in the free argument list so that the caller
		// can distinguish unclaimed flags.
		if s == "--" {
			free = append(free, args[i:]...)
			return flags, free, nil
		}

		// Case 1: The previous argument is a flag that needs a value.
		if wantArg {
			flags = append(flags, s)
			wantArg = false
			continue
		}

		// Case 2: Flag-shaped arguments (-x, --x).
		if rest, ok := strings.CutPrefix(s, "-"); ok && rest != "" {
			rest = strings.TrimPrefix(rest, "-") // accept -name or --name

			// Some flags may carry their own values (e.g., --name=value).
			// Otherwise, anything that isn't a Boolean flag requires an argument.
			name, _, ok := strings.Cut(rest, "=")
			if f := fs.Lookup(name); f != nil {
				// This is a flag belonging to this flag set.
				flags = append(flags, s)
				if !isBoolFlag(f) && !ok {
					wantArg = true
				}
			} else {
				// This may be a flag for a downstream flag set; for now
				// treat it as a free argument.
				free = append(free, s)
			}
			continue
		}

		// Case 3: Free arguments (including "-").
		free = append(free, s)
	}
	if wantArg {
		return nil, nil, fmt.Errorf("missing value for flag %q", flags[len(flags)-1])
	}
	return flags, free, nil
}

func isBoolFlag(f *flag.Flag) bool {
	v, ok := f.Value.(interface {
		IsBoolFlag() bool
	})
	return ok && v.IsBoolFlag()
}

func joinArgs(a, b []string) []string {
	if len(a) == 0 {
		return b
	} else if len(b) == 0 {
		return a
	}
	return append(append(a, "--"), b...)
}
