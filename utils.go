// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package command

import (
	"flag"
	"fmt"
	"strings"
)

// Flags returns a SetFlags function that calls bind(fs, v) for each v and the
// given flag set.
func Flags(bind func(*flag.FlagSet, any), vs ...any) func(*Env, *flag.FlagSet) {
	return func(_ *Env, fs *flag.FlagSet) {
		for _, v := range vs {
			bind(fs, v)
		}
	}
}

// usageLines parses and normalizes usage lines. The command name is stripped
// from the head of each line if it is present.
func (c *C) usageLines(flags HelpFlags) []string {
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
	if len(lines) == 0 && c.hasFlagsDefined(flags.wantPrivateFlags()) {
		return []string{"[flags]"}
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

// splitFlags constructs two slices from args, the first containing all flags
// and their arguments matched by fs, the second containing all the other free
// arguments. Flag values are not parsed. Flag-shaped strings not matched by fs
// are treated as free arguments.  An error is reported if a flag lacks its
// argument.
func splitFlags(fs *flag.FlagSet, args []string) (flags, free []string, _ error) {
	var wantArg bool
	for _, s := range args {
		// Case 1: The previous argument is a flag that needs a value.
		if wantArg {
			flags = append(flags, s)
			wantArg = false
			continue
		}

		// Treat "-" and "--" as free arguments to simplify the logic below.
		if s == "-" || s == "--" {
			free = append(free, s)
			continue
		}

		// Case 2: Flag-shaped arguments (-x, --x).
		if rest, ok := strings.CutPrefix(s, "-"); ok {
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

		// Case 3: Free arguments.
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

func joinArgs(a, b []string) []string { return append(a, b...) }
