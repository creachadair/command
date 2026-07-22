// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package command

import (
	"bytes"
	"cmp"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/creachadair/mds/mstr"
	"github.com/creachadair/mds/slice"
)

// HelpCommand constructs a standardized help command with optional topics.
// The caller is free to edit the resulting command, each call returns a
// separate value.
//
// As a special case, if there are arguments after the help command and the
// first is one of "-a", "-all", or "--all", that argument is discarded and the
// rendered help text includes unlisted commands and private flags.
func HelpCommand(topics []HelpTopic) *C {
	cmd := &C{
		Name:  "help",
		Usage: "[-a|--all] [topic/command]",
		Help: `Print help for the specified command or topic.

With -a or --all, also show help for unlisted commands and private flags.`,

		CustomFlags: true,

		Run: func(env *Env) error {
			if len(env.Args) >= 1 { // maybe: help -a foo
				switch env.Args[0] {
				case "-a", "-all", "--all":
					env.HelpFlags(IncludeUnlisted | IncludePrivateFlags)
					env.Args = env.Args[1:]
				}
			}
			return RunHelp(env)
		},
	}
	for _, topic := range topics {
		cmd.Commands = append(cmd.Commands, topic.command())
	}
	return cmd
}

// A HelpTopic specifies a name and some help text for use in constructing help
// topic commands. See [HelpCommand].
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

	IncludeAll HelpFlags = ^0 // include all available help types
)

// HelpInfo returns help details for c.
//
// A command or subcommand with no Run function and no subcommands of its own
// is considered a help topic, and listed separately.
//
// Flags whose usage message has the case-sensitive prefix "PRIVATE:" are
// omitted from help listings unless [IncludePrivateFlags] is set.
// Subcommands marked as unlisted are omitted from help listings unless
// [IncludeUnlisted] is set.
func (c *C) HelpInfo(flags HelpFlags) HelpInfo {
	help := strings.TrimSpace(c.Help)
	synopsis, _, _ := strings.Cut(help, "\n")
	prefix := "  " + c.Name + " "
	h := HelpInfo{
		Name:     c.Name,
		Synopsis: synopsis,
		Help:     help,
	}
	if u := c.usageLines(flags); len(u) != 0 {
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
	ht := env.Command.HelpInfo(env.hflag | IncludeCommands)
	ht.Topics = append(ht.Topics, topics...)
	ht.WriteLong(env)
	return ErrRequestHelp
}

// runShortHelp is a run function that prints synopsis help.
func printShortHelp(env *Env) error {
	env.Command.HelpInfo(env.hflag).WriteSynopsis(env)
	return ErrRequestHelp
}

// toStdout returns a copy of e in which output goes to stdout instead of
// whatever it is set to (stderr by default).
func (e *Env) toStdout() *Env {
	cenv := *e // shallow copy
	cenv.Log = os.Stdout
	return &cenv
}

// RunHelp is a run function that implements long help.  It displays the
// help for the enclosing command or subtopics of "help" itself.
func RunHelp(env *Env) error {
	// Check whether the arguments describe the parent (assuming there is one)
	// or one of its subcommands.
	var res walkResult
	if env.Parent != nil {
		res = walkArgs(env.Parent.HelpFlags(env.hflag), env.Args)
		if res.unique == env.Parent {
			// For the parent, include the help command's own topics.
			return printLongHelp(res.unique.toStdout(), env.Command.HelpInfo(env.hflag|IncludeCommands).Topics)
		} else if res.unique != nil {
			return printLongHelp(res.unique.toStdout(), nil)
		}
	}

	// Otherwise, check whether the arguments name a help subcommand.
	hr := walkArgs(env, env.Args)
	if hr.unique != nil {
		return printLongHelp(hr.unique.toStdout(), nil)
	}

	// Otherwise the arguments request an unknown topic.
	return res.merge(hr).fail(env)
}

// walkResult is the result of searching a command tree for a help topic.
type walkResult struct {
	unique  *Env     // if a unique match was found, its env
	last    string   // the last argument uniquely resolved
	rest    []string // remaining argments after last
	options []string // candidates
}

// fail reports a diagnostic message to env, and returns [ErrRequestHelp].
func (w walkResult) fail(env *Env) error {
	fmt.Fprintf(env, "Unknown help topic %q", strings.Join(w.rest, " "))
	if w.last != "" {
		fmt.Fprintf(env, " under %q", w.last)
	}
	if opts := w.options; len(opts) != 0 {
		fmt.Fprintf(env, ". Did you mean: %s?", joinOptions(opts))
	}
	fmt.Fprintln(env)
	return ErrRequestHelp
}

// merge returns a copy of w with o merged into it.
func (w walkResult) merge(o walkResult) walkResult {
	if w.last == "" {
		w.last = o.last
	}
	if len(w.rest) == 0 {
		w.rest = o.rest
	}
	w.options = append(w.options, o.options...)
	return w
}

func walkArgs(env *Env, args []string) (out walkResult) {
	cur := env

	for i, arg := range args {
		// If no corresponding subcommand is found, or if the subtree starting
		// with that command is unlisted and we weren't asked to show unlisted
		// things, report no match.
		next := cur.Command.FindSubcommand(arg)
		if next == nil {
			out.rest = args[i:] // including arg (which is unresolved)
			out.options = findCandidates(cur.Command, arg)
			return
		}
		// Note: If the caller explicitly names an unlisted command, we
		// will still report help for it, even if -a / --all is not set.

		// Populate flags so that the help text will include them.
		next.setFlags(cur, &next.Flags)
		cur = cur.newChild(next, nil)
		out.last = arg
	}
	out.unique = cur
	return
}

func findCandidates(cmd *C, arg string) []string {
	m := slice.ToMap(cmd.Commands, func(c *C) (string, float64) {
		return c.Name, mstr.Similarity(c.Name, arg)
	})
	maps.DeleteFunc(m, func(_ string, sim float64) bool {
		return sim < 0.75
	})
	names := slice.MapKeys(m)
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Compare(m[b], m[a])
	})
	return names
}

func joinOptions(opts []string) string {
	if len(opts) == 0 {
		return ""
	} else if len(opts) == 1 {
		return strconv.Quote(opts[0])
	}
	for i, opt := range opts {
		opts[i] = strconv.Quote(opt)
	}
	return strings.Join(opts[:len(opts)-1], ", ") + " or " + opts[len(opts)-1]
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
	if t.Kind() == reflect.Pointer {
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
