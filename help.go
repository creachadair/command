// Copyright (C) 2020 Michael J. Fromberger. All Rights Reserved.

package command

import (
	"bytes"
	"flag"
	"fmt"
	"io"
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

// HelpInfo returns help details for c. If includeCommands is true and c has
// subcommands, their help is also generated.
//
// A command or subcommand with no Run function and no subcommands of its own
// is considered a help topic, and listed separately.
func (c *C) HelpInfo(includeCommands bool) HelpInfo {
	help := strings.TrimSpace(c.Help)
	prefix := "  " + c.Name + " "
	h := HelpInfo{
		Name:     c.Name,
		Synopsis: strings.SplitN(help, "\n", 2)[0],
		Help:     help,
	}
	if c.Runnable() {
		h.Usage = "Usage:\n\n" + indent(prefix, prefix, strings.Join(c.usageLines(), "\n"))
	}
	if c.hasFlagsDefined() {
		var buf bytes.Buffer
		fmt.Fprintln(&buf, "\nOptions:")
		c.Flags.SetOutput(&buf)
		c.Flags.PrintDefaults()
		h.Flags = strings.TrimSpace(buf.String())
	}
	if includeCommands {
		for _, cmd := range c.Commands {
			sh := cmd.HelpInfo(false) // don't recur
			if cmd.Runnable() || len(cmd.Commands) != 0 {
				h.Commands = append(h.Commands, sh)
			} else {
				h.Topics = append(h.Topics, sh)
			}
		}
	}
	return h
}

func (c *C) hasFlagsDefined() (ok bool) {
	if !c.CustomFlags {
		c.Flags.VisitAll(func(*flag.Flag) {
			ok = true
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
	ht := env.Command.HelpInfo(true)
	ht.Topics = append(ht.Topics, topics...)
	ht.WriteLong(env)
	return ErrUsage
}

// runShortHelp is a run function that prints synopsis help.
func printShortHelp(env *Env) error {
	env.Command.HelpInfo(false).WriteSynopsis(env)
	return ErrUsage
}

// RunHelp is a run function that implements long help.  It displays the
// help for the enclosing command or subtopics of "help" itself.
func RunHelp(env *Env) error {
	// Check whether the arguments describe the parent or one of its subcommands.
	target := walkArgs(env.Parent, env.Args)
	if target == env.Parent {
		// For the parent, include the help command's own topics.
		return printLongHelp(target, env.Command.HelpInfo(true).Topics)
	} else if target != nil {
		return printLongHelp(target, nil)
	}

	// Otherwise, check whether the arguments name a help subcommand.
	if ht := walkArgs(env, env.Args); ht != nil {
		return printLongHelp(ht, nil)
	}

	// Otherwise the arguments request an unknown topic.
	fmt.Fprintf(env, "Unknown help topic %q\n", strings.Join(env.Args, " "))
	return ErrUsage
}

func walkArgs(env *Env, args []string) *Env {
	cur := env

	// Populate flags so that the help text will include them.
	for _, arg := range args {
		next := cur.Command.FindSubcommand(arg)
		if next == nil {
			return nil
		}
		next.setFlags(cur, &next.Flags)
		cur = cur.newChild(next, nil)
	}
	return cur
}
