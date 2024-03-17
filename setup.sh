#!/usr/bin/env bash
#
# Usage: setup.sh path/to/main.go
#
# Generate a basic skeleton for a command-line tool using the command package.
# The resulting program includes the built-in "help" and "version" commands,
# and a minimal Run function.
#
set -euo pipefail

OUTPUT="${1:?missing output path}"
mkdir -p "$(dirname "$OUTPUT")"
if [[ -f "$OUTPUT" ]] ; then
    echo "Output $OUTPUT already exists" 1>&2
    exit 1
fi
go run golang.org/x/tools/cmd/goimports@latest > "$OUTPUT" <<"EOF"
package main

import "github.com/creachadair/command"

func main() {
   root := &command.C{
      Name: command.ProgramName(),
      Help: "Skeleton of a CLI tool.",

      Run: command.Adapt(runMain),

      Commands: []*command.C{
         command.HelpCommand(nil),
         command.VersionCommand(),
      },
   }
   command.RunOrFail(root.NewEnv(nil).MergeFlags(true), os.Args[1:])
}

func runMain(env *command.Env) error {
   fmt.Fprintln(env, "TODO: implement this")
   return nil
}
EOF
echo "Created $OUTPUT." 1>&2

