// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package command

import (
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/creachadair/mds/value"
)

// Flags returns a function with the signature of the [C.SetFlags] callback,
// that calls bind(fs, v) for each v and the given flag set.
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
	for line := range strings.SplitSeq(c.Usage, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		} else if line == c.Name {
			lines = append(lines, "")
		} else {
			lines = append(lines, strings.TrimPrefix(line, prefix))
		}
	}
	if len(lines) == 0 {
		var tag string
		if c.hasFlagsDefined(flags.wantPrivateFlags()) {
			tag = "[flags]"
		}
		if len(c.Commands) != 0 {
			tag = joinSpace(tag, "<command>")
		}
		if tag != "" {
			lines = append(lines, tag)
		}
		if hc := c.FindSubcommand("help"); hc != nil && hc.Runnable() {
			lines = append(lines, "help")
		}
	}
	return lines
}

func joinSpace(a, b string) string {
	if a == "" {
		return b
	} else if b == "" {
		return a
	}
	return a + " " + b
}

// indent returns text indented as specified; first is prepended to the first
// line, and prefix to all subsequent lines.
func indent(first, prefix, text string) string {
	return first + strings.ReplaceAll(text, "\n", "\n"+prefix)
}

// FailWithUsage is a run function that logs a usage message for the command
// and returns [ErrRequestHelp].
func FailWithUsage(env *Env) error {
	env.Command.HelpInfo(0).WriteUsage(env)
	return ErrRequestHelp
}

// splitFlags constructs two slices from args, the first containing all flags
// and their arguments matched by fs, the second containing all the other free
// arguments.
//
// The arguments for flags mentioned in fs are checked for presence, but are
// not parsed.  An error is reported if a flag lacks its argument.
// Flag-shaped strings NOT matched by fs are treated as free arguments.
// We do not at this point have enough information to know whether
// such a flag-candidate requires an argument, as that requires the FlagSet.
func splitFlags(fs *flag.FlagSet, args []string) (flags, free []string, _ error) {
	var wantArg bool
	for i, s := range args {
		// Case 1: The previous argument is a flag that needs a value.
		if wantArg {
			flags = append(flags, s)
			wantArg = false
			continue
		}

		// Some shortcuts to simplify processing below:
		if s == "-" {
			// Bare "-" is flag-shaped, but treated as a non-flag argument for parsing.
			free = append(free, s)
			continue
		} else if s == "--" {
			// Bare "--" is consumed by the flag parser as a signal to stop parsing.
			// Seeing it when we have not yet observed any other free arguments,
			// we give up looking for flags belonging to this set.
			if len(free) == 0 {
				flags = append(flags, s)
				free = append(free, args[i+1:]...)
				break
			}
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

// CInfo represents metadata about a command and its subcommands, in a format
// suitable for JSON encoding.
type CInfo struct {
	Name  string   `json:"name"`
	Usage []string `json:"usage,omitempty"`
	Help  string   `json:"help,omitzero"`

	Runnable bool       `json:"runnable"`
	Flags    []FlagInfo `json:"flags,omitempty"`
	Unlisted bool       `json:"unlisted,omitzero"`
	Commands []*CInfo   `json:"commands,omitempty"`
}

// InfoCommand constructs a standardized info command with the specified name
// ("command-info" by default) that prints command structure metadata in JSON
// format. The caller is free to edit the resulting command, each call returns
// a separate value.
//
// Without arguments, the complete structure of the root command is printed.
// With arguments, only the named subcommand and its substructure are printed.
// Unlisted commands and private flags are omitted unless "-a" is given.
// Use "--root-only" to omit subcommands.
func InfoCommand(name string) *C {
	var doAll, doRootOnly bool
	return &C{
		Name:  cmp.Or(name, "command-info"),
		Usage: "[subcommand ... [--flag]]",
		Help:  "Write command structure to stdout as JSON.",
		SetFlags: func(env *Env, fs *flag.FlagSet) {
			fs.BoolVar(&doAll, "a", false, "Include unlisted commands and private flags")
			fs.BoolVar(&doRootOnly, "root-only", false, "Show only the root command, not subcommands")
		},
		Run: func(env *Env) error {
			defer func() { doAll = false; doRootOnly = false }()
			cur := env
			for cur.Parent != nil {
				cur = cur.Parent
			}
			// Include commands up front so we can walk the tree if we need to.
			opts := value.Cond(doAll, IncludeAll, IncludeCommands)
			info := cur.Command.Info(opts)
			for i, arg := range env.Args {
				if strings.HasPrefix(arg, "-") {
					if i+1 < len(env.Args) {
						return fmt.Errorf("extra arguments after flag %q: %q", arg, env.Args[i+1:])
					}
					clean := strings.TrimLeft(arg, "-")
					pos := slices.IndexFunc(info.Flags, func(f FlagInfo) bool {
						return f.Name == clean
					})
					if pos < 0 {
						return fmt.Errorf("command %q has no flag %q", info.Name, arg)
					}
					return json.NewEncoder(os.Stdout).Encode(info.Flags[pos])
				}
				pos := slices.IndexFunc(info.Commands, func(c *CInfo) bool {
					return c.Name == arg
				})
				if pos < 0 {
					return fmt.Errorf("command %q has no subcommand %q", info.Name, arg)
				}
				info = info.Commands[pos]
			}
			if doRootOnly {
				info.Commands = nil
			}
			return json.NewEncoder(os.Stdout).Encode(info)
		},
	}
}

// FlagInfo represents metadata about a flag defined by a command, in a format
// suitable for JSON encoding.
type FlagInfo struct {
	Name          string `json:"name"`
	Usage         string `json:"usage"`
	DefaultString string `json:"defaultString,omitzero"`
	IsBool        bool   `json:"isBool,omitzero"`
	Private       bool   `json:"private,omitzero"`
}

// Info constructs a [CInfo] record for c and its subcommands.  The provided
// help flags determine the visibility of unlisted and private flags.
func (c *C) Info(flags HelpFlags) *CInfo {
	c.setFlags(c.NewEnv(nil), &c.Flags)
	out := &CInfo{
		Name:     c.Name,
		Usage:    c.usageLines(flags),
		Help:     c.Help,
		Runnable: c.Runnable(),
		Unlisted: c.Unlisted,
	}
	c.Flags.VisitAll(func(f *flag.Flag) {
		u, ok := strings.CutPrefix(f.Usage, flagPrivatePrefix)
		if ok && !flags.wantPrivateFlags() {
			return // skip
		}
		dstring := f.DefValue
		if ok, err := isZeroValue(f, dstring); err == nil && ok {
			dstring = ""
		}
		out.Flags = append(out.Flags, FlagInfo{
			Name:          f.Name,
			Usage:         u,
			DefaultString: dstring,
			IsBool:        isBoolFlag(f),
			Private:       ok,
		})
	})
	if flags.wantCommands() {
		for _, sub := range c.Commands {
			if sub.Unlisted && !flags.wantUnlisted() {
				continue
			}
			out.Commands = append(out.Commands, sub.Info(flags))
		}
	}
	return out
}
