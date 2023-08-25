// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package command

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"reflect"
	"strings"
	"text/tabwriter"
)

// HelpCommand constructs a standardized help command with optional topics.
// The caller is free to edit the resulting command, each call returns a
// separate value.
func HelpCommand(topics []HelpTopic) *C {
	cmd := &C{
		Name:  "help",
		Usage: "[topic/command]",
		Help:  `Print help for the specified command or topic.`,

		CustomFlags: true,
		Run:         RunHelp,
	}
	for _, topic := range topics {
		cmd.Commands = append(cmd.Commands, topic.command())
	}
	return cmd
}

// A HelpTopic specifies a name and some help text for use in constructing help
// topic commands.
type HelpTopic struct {
	Name string
	Help string
}

func (h HelpTopic) command() *C { return &C{Name: h.Name, Help: h.Help} }

// HelpInfo records synthesized help details for a command.
type HelpInfo struct {
	Name     string
	Synopsis string
	Usage    string
	Help     string
	Flags    string

	// Help for subcommands (populated if requested)
	Commands []HelpInfo

	// Help for subtopics (populated if requested)
	Topics []HelpInfo
}

// HelpFlags is a bit mask of flags for the HelpInfo method.
type HelpFlags int

func (h HelpFlags) wantCommands() bool     { return h&IncludeCommands != 0 }
func (h HelpFlags) wantUnlisted() bool     { return h&IncludeUnlisted != 0 }
func (h HelpFlags) wantPrivateFlags() bool { return h&IncludePrivateFlags != 0 }

const (
	IncludeCommands     HelpFlags = 1 << iota // include subcommands and help topics
	IncludeUnlisted                           // include unlisted subcommands
	IncludePrivateFlags                       // include private (hidden) flags
)

// HelpInfo returns help details for c.
//
// A command or subcommand with no Run function and no subcommands of its own
// is considered a help topic, and listed separately.
//
// Flags whose usage message has the case-sensitive prefix "PRIVATE:" are
// omitted from help listings.
func (c *C) HelpInfo(flags HelpFlags) HelpInfo {
	help := strings.TrimSpace(c.Help)
	prefix := "  " + c.Name + " "
	h := HelpInfo{
		Name:     c.Name,
		Synopsis: strings.SplitN(help, "\n", 2)[0],
		Help:     help,
	}
	if u := c.usageLines(); len(u) != 0 {
		h.Usage = "Usage:\n\n" + indent(prefix, prefix, strings.Join(u, "\n"))
	}
	if c.hasFlagsDefined(flags.wantPrivateFlags()) {
		var buf bytes.Buffer
		fmt.Fprintln(&buf, "Flags:")
		writeFlagHelp(&buf, &c.Flags, flags.wantPrivateFlags())
		h.Flags = strings.TrimSpace(buf.String())
	}
	if flags.wantCommands() {
		for _, cmd := range c.Commands {
			if cmd.Unlisted && !flags.wantUnlisted() {
				continue
			}
			sh := cmd.HelpInfo(flags &^ IncludeCommands) // don't recur
			if cmd.Runnable() || len(cmd.Commands) != 0 {
				h.Commands = append(h.Commands, sh)
			} else {
				h.Topics = append(h.Topics, sh)
			}
		}
	}
	return h
}

func (c *C) hasFlagsDefined(wantPrivate bool) (ok bool) {
	if !c.CustomFlags {
		c.Flags.VisitAll(func(f *flag.Flag) {
			if !strings.HasPrefix(f.Usage, flagPrivatePrefix) || wantPrivate {
				ok = true
			}
		})
	}
	return
}

func (c *C) setFlags(env *Env, fs *flag.FlagSet) {
	if c != nil && c.SetFlags != nil && !c.isFlagSet {
		c.SetFlags(env, fs)
		c.isFlagSet = true
	}
}

// WriteUsage writes a usage summary to w.
func (h HelpInfo) WriteUsage(w io.Writer) {
	if h.Usage != "" {
		fmt.Fprint(w, h.Usage, "\n\n")
	}
}

// WriteSynopsis writes a usage summary and command synopsis to w.
// If the command defines flags, the flag summary is also written.
func (h HelpInfo) WriteSynopsis(w io.Writer) {
	h.WriteUsage(w)
	if h.Synopsis == "" {
		fmt.Fprint(w, "(no description available)\n\n")
	} else {
		fmt.Fprint(w, h.Synopsis+"\n\n")
	}
	if h.Flags != "" {
		fmt.Fprint(w, h.Flags, "\n\n")
	}
}

// WriteLong writes a complete help description to w, including a usage
// summary, full help text, flag summary, and subcommands.
func (h HelpInfo) WriteLong(w io.Writer) {
	h.WriteUsage(w)
	if h.Help == "" {
		fmt.Fprint(w, "(no description available)\n\n")
	} else {
		fmt.Fprint(w, h.Help, "\n\n")
	}
	if h.Flags != "" {
		fmt.Fprint(w, h.Flags, "\n\n")
	}
	if len(h.Commands) != 0 {
		writeTopics(w, h.Name+" ", "Subcommands:", h.Commands)
	}
	if len(h.Topics) != 0 {
		writeTopics(w, "", "Help topics:", h.Topics)
	}
}

func writeTopics(w io.Writer, base, label string, topics []HelpInfo) {
	fmt.Fprintln(w, label)
	tw := tabwriter.NewWriter(w, 4, 8, 1, ' ', 0)
	for _, cmd := range topics {
		syn := cmd.Synopsis
		if syn == "" {
			syn = "(no description available)"
		}
		fmt.Fprint(tw, "  ", base+cmd.Name, "\t:\t", syn, "\n")
	}
	tw.Flush()
	fmt.Fprintln(w)
}

// runLongHelp is a run function that prints long-form help.
// The topics are additional help topics to include in the output.
func printLongHelp(env *Env, topics []HelpInfo) error {
	ht := env.Command.HelpInfo(IncludeCommands)
	ht.Topics = append(ht.Topics, topics...)
	ht.WriteLong(env)
	return ErrRequestHelp
}

// runShortHelp is a run function that prints synopsis help.
func printShortHelp(env *Env) error {
	env.Command.HelpInfo(0).WriteSynopsis(env)
	return ErrRequestHelp
}

// RunHelp is a run function that implements long help.  It displays the
// help for the enclosing command or subtopics of "help" itself.
func RunHelp(env *Env) error {
	// Check whether the arguments describe the parent or one of its subcommands.
	target := walkArgs(env.Parent, env.Args)
	if target == env.Parent {
		// For the parent, include the help command's own topics.
		return printLongHelp(target, env.Command.HelpInfo(IncludeCommands).Topics)
	} else if target != nil {
		return printLongHelp(target, nil)
	}

	// Otherwise, check whether the arguments name a help subcommand.
	if ht := walkArgs(env, env.Args); ht != nil {
		return printLongHelp(ht, nil)
	}

	// Otherwise the arguments request an unknown topic.
	fmt.Fprintf(env, "Unknown help topic %q\n", strings.Join(env.Args, " "))
	return ErrRequestHelp
}

func walkArgs(env *Env, args []string) *Env {
	cur := env

	for _, arg := range args {
		// If no corresponding subcommand is found, or if the subtree starting
		// with that command is unlisted, report no match.
		next := cur.Command.FindSubcommand(arg)
		if next == nil || next.Unlisted {
			return nil
		}
		// Populate flags so that the help text will include them.
		next.setFlags(cur, &next.Flags)
		cur = cur.newChild(next, nil)
	}
	return cur
}

const flagPrivatePrefix = "PRIVATE:"

// writeFlagHelp writes descriptive help about the flags defined in fs to w.
//
// This is essentially a copy of flag.FlagSet.PrintDefault, with changes:
//
// - Long flag names (> 1 character) are prefixed by "--" instead of "-".
// - Flags whose usage begins with "PRIVATE:" are omitted.
func writeFlagHelp(w *bytes.Buffer, fs *flag.FlagSet, wantPrivate bool) {
	var errs []error
	fs.VisitAll(func(f *flag.Flag) {
		if u, ok := strings.CutPrefix(f.Usage, flagPrivatePrefix); ok {
			if !wantPrivate {
				return // don't display this flag
			}
			f.Usage = strings.TrimPrefix(u, " ")
		}
		tag := "  -"
		if len(f.Name) > 1 {
			tag = " --"
		}
		fmt.Fprint(w, tag, f.Name)
		name, usage := flag.UnquoteUsage(f)
		if name != "" {
			fmt.Fprint(w, " ", name)
		}
		if len(f.Name) == 1 && name == "" {
			w.WriteString("\t")
		} else {
			w.WriteString("\n    \t")
		}
		w.WriteString(strings.ReplaceAll(usage, "\n", "\n    \t"))

		if ok, err := isZeroValue(f, f.DefValue); err != nil {
			errs = append(errs, err)
		} else if !ok {
			if isStringish(f) {
				fmt.Fprintf(w, " (default %q)", f.DefValue)
			} else {
				fmt.Fprintf(w, " (default %v)", f.DefValue)
			}
		}
		w.WriteString("\n")
	})
	if len(errs) != 0 {
		for _, err := range errs {
			fmt.Fprint(w, "\n", err)
		}
	}
}

// isStringish reports whether v has underlying string type.
func isStringish(f *flag.Flag) bool {
	t := reflect.TypeOf(f.Value)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.String
}

// isZeroValue reports whether the string represents the zero value for a
// flag. Copied with minor changes frpm src/flag/flag.go.
func isZeroValue(f *flag.Flag, value string) (ok bool, err error) {
	// Build a zero value of the flag's Value type, and see if the result of
	// calling its String method equals the value passed in.  This works unless
	// the Value type is itself an interface type.
	typ := reflect.TypeOf(f.Value)
	var z reflect.Value
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
		z = reflect.New(typ)
	} else {
		z = reflect.Zero(typ)
	}
	// Catch panics calling the String method, which shouldn't prevent the
	// usage message from being printed, but that we should report to the
	// user so that they know to fix their code.
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("panic calling String method on zero %v for flag %s: %v", typ, f.Name, e)
		}
	}()
	return value == z.Interface().(flag.Value).String(), nil
}
