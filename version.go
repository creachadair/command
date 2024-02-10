// Copyright (C) 2022 Michael J. Fromberger. All Rights Reserved.

package command

import (
	"encoding/hex"
	"encoding/json"
	"errors"
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
			bi, ok := debug.ReadBuildInfo()
			if !ok {
				return errors.New("no version information is available")
			}
			vi := versionInfo{
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
			if doJSON {
				json.NewEncoder(os.Stdout).Encode(vi)
				return nil
			}
			fmt.Println(vi)
			return ErrRequestHelp
		}),
	}
}

// versionInfo records the version information extracted from the build info
// record by the "version" command implementation.
type versionInfo struct {
	Name      string     `json:"name"`
	Path      string     `json:"path,omitempty"`
	Version   string     `json:"version,omitempty"`
	Commit    string     `json:"commit,omitempty"`
	Modified  bool       `json:"modified,omitempty"`
	Toolchain string     `json:"toolchain,omitempty"`
	OS        string     `json:"os,omitempty"`
	Arch      string     `json:"arch,omitempty"`
	Time      *time.Time `json:"time,omitempty"`
}

// String encodes the versionInfo in a single-line human-readable format.
// This is used for the plain "version" output.
func (v versionInfo) String() string {
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
func (v *versionInfo) parseModule(m *debug.Module) bool {
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
	v.Version = m.Version
	return m.Version != ""
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
