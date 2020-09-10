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

// runLongHelp is a run function that implements the "help" functionality.
func runLongHelp(ctx *Context, args []string) error {
	ctx.Command.HelpInfo(true).WriteLong(ctx)
	return ErrUsage
}

// runShortHelp is a run function that implements synopsis help.
func runShortHelp(ctx *Context, args []string) error {
	ctx.Command.HelpInfo(false).WriteSynopsis(ctx)
	return ErrUsage
}

// RunHelp is a run function that implements long help.  It displays the
// help for the enclosing command or subtopics of "help" itself.
func RunHelp(ctx *Context, args []string) error {
	// First check whether the arguments name a parent subcommand.
	if pt := walkArgs(ctx.Parent.Command, args); pt != nil {
		return runLongHelp(pt.NewContext(ctx.Config), args)
	}

	// Otherwise, check whether the arguments name a help subcommand.
	if ht := walkArgs(ctx.Command, args); ht != nil {
		return runLongHelp(ht.NewContext(ctx.Config), args)
	}

	// Otherwise this is an unknown topic.
	fmt.Fprintf(ctx, "Unknown help topic %q\n", strings.Join(args, " "))
	return ErrUsage
}

func walkArgs(cmd *C, args []string) *C {
	cur := cmd
	for _, arg := range args {
		cur = cur.FindSubcommand(arg)
		if cur == nil {
			return nil
		}
	}
	return cur
}
