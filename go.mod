module github.com/creachadair/command

go 1.25.0

require github.com/google/go-cmp v0.7.0

require github.com/creachadair/mds v0.27.1

// Bug in flag parsing on nested subcommands.
retract v0.2.3
