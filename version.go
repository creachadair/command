// Copyright (C) 2022 Michael J. Fromberger. All Rights Reserved.

package command

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
)

// VersionCommand constructs a standardized version command that prints version
// metadata from the running binary to stdout. The caller can safely modify the
// returned command to customize its behavior.
func VersionCommand() *C {
	var doJSON bool
	return &C{
		Name: "version",
		Help: `Print build version information for this program and exit.`,
		SetFlags: func(_ *Env, fs *flag.FlagSet) {
			fs.BoolVar(&doJSON, "json", false, "Write version information as JSON")
		},
		Run: Adapt(func(env *Env) error {
			vi := GetVersionInfo()
			if doJSON {
				json.NewEncoder(os.Stdout).Encode(vi)
				return nil
			}
			fmt.Println(vi)
			return ErrRequestHelp
		}),
	}
}

// VersionInfo records version information extracted from the build info record
// for the running program.
type VersionInfo struct {
	// Name is the base name of the running binary from os.Args.
	Name string `json:"name"`

	// Path is the import path of the main package.
	Path string `json:"path"`

	// Version, if available, is the version tag at which the binary was built.
	// This is empty if no version label is available, e.g. an untagged commit.
	Version string `json:"version,omitempty"`

	// Commit, if available, is the commit hash at which the binary was built.
	// Typically this will be a hex string, but the format is not guaranteed.
	Commit string `json:"commit,omitempty"`

	// Modified reports whether the contents of the build environment were
	// modified from a clean state. This may indicate the presence of extra
	// files in the working directory, even if the repository is up-to-date.
	Modified bool `json:"modified,omitempty"`

	// Toolchain gives the Go toolchain version that built the binary.
	Toolchain string `json:"toolchain"`

	// OS gives the GOOS value used by the compiler.
	OS string `json:"os,omitempty"`

	// Arch gives the GOARCH value used by the compiler.
	Arch string `json:"arch,omitempty"`

	// Time, if non-nil, gives the timestamp corresponding to the commit at
	// which the binary was built.  It is nil if the commit time is not
	// recorded; otherwise the value is a non-zero time in UTC.
	Time *time.Time `json:"time,omitempty"`
}

// GetVersionInfo returns a VersionInfo record extracted from the build
// metadata in the currently running process. If no build information is
// available, only the Name field will be populated.
func GetVersionInfo() VersionInfo {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return VersionInfo{Name: filepath.Base(os.Args[0])}
	}
	vi := VersionInfo{
		Name:      filepath.Base(os.Args[0]),
		Path:      bi.Path,
		Toolchain: bi.GoVersion,
	}
	vi.parseModule(&bi.Main)

	// Check for build settings. These may not be present if the build was done
	// without the repository available, e.g., via go install outside a module.
	// If these settings are present they are preferred.
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			vi.Commit = s.Value
		case "vcs.time":
			ts, err := time.Parse(time.RFC3339, s.Value)
			if err == nil {
				vi.Time = &ts
			}
		case "vcs.modified":
			vi.Modified = s.Value == "true"
		case "GOOS":
			vi.OS = s.Value
		case "GOARCH":
			vi.Arch = s.Value
		}
	}
	return vi
}

// String encodes v in a single-line human-readable format.  This is the format
// used for plain text output by the "version" command implementation.
func (v VersionInfo) String() string {
	var sb strings.Builder
	sb.WriteString(v.Name)
	if v.Version != "" {
		fmt.Fprint(&sb, " version ", v.Version)
	}
	if v.Path != "" {
		fmt.Fprint(&sb, " path ", v.Path)
	}
	if v.Commit != "" {
		fmt.Fprint(&sb, " commit ", v.Commit)
	}
	if v.Toolchain != "" {
		fmt.Fprint(&sb, " with ", v.Toolchain)
	}
	if v.Time != nil {
		fmt.Fprint(&sb, " at ", v.Time.Format(time.RFC3339))
	}
	if v.OS != "" && v.Arch != "" {
		fmt.Fprint(&sb, " for ", v.OS, "/", v.Arch)
	}
	return sb.String()
}

// parseModule reports whether m contains version and commit information and,
// if so, populates the corresopnding fields of v.  If the module has a replace
// directive, the replacement is preferred.
func (v *VersionInfo) parseModule(m *debug.Module) bool {
	if m.Replace != nil && v.parseModule(m.Replace) {
		return true
	}

	// A module version may be a tag, e.g. v1.2.3, or a pseudo-version assigned
	// by the module plumbing, e.g., v1.2.3-{date}-{commit}.
	if ts, commit, ok := parsePseudoVersion(m.Version); ok {
		v.Commit = commit
		v.Time = &ts
		return true
	}
	if m.Version != "(devel)" {
		v.Version = m.Version
	}
	return v.Version != ""
}

// parsePseudoVersion reports whether s appears to be a Go module pseudoversion
// marker, and if so returns the timestamp and commit digest extracted from it.
func parsePseudoVersion(s string) (time.Time, string, bool) {
	ps := strings.Split(s, "-")
	if len(ps) == 3 {
		// A valid pseudo-version has a timestamp and hex commit.
		ts, terr := time.Parse(time.RFC3339, ps[1])
		_, herr := hex.DecodeString(ps[2])
		return ts, ps[2], terr == nil && herr == nil
	}
	return time.Time{}, "", false
}
