// Package buildinfo says which cade this is: `cade version`. `go tool mage build`
// sets the version, commit and date with -ldflags -X; a plain `go build`
// falls back to the VCS stamp the Go toolchain embeds.
package buildinfo

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// Set by `go tool mage build`: -X github.com/chipskein/cade/internal/buildinfo.version=...
var (
	version  = ""
	commit   = ""
	date     = ""
	llamaTag = ""
)

// Info describes the running binary.
type Info struct {
	Version string
	Commit  string
	Date    string
	// Accelerator is "CPU" or "CUDA", from the build tags.
	Accelerator string
	LlamaTag    string
}

// Read returns what this binary was built from.
//
//	fmt.Println(buildinfo.Read())
func Read() Info {
	info := Info{Version: version, Commit: commit, Date: date, Accelerator: accelerator, LlamaTag: llamaTag}
	if stamp, ok := debug.ReadBuildInfo(); ok {
		info = withVCSStamp(info, stamp.Settings)
	}
	if info.Version == "" {
		info.Version = "dev"
	}
	return info
}

// withVCSStamp fills what -ldflags did not set from the toolchain's stamp.
func withVCSStamp(info Info, settings []debug.BuildSetting) Info {
	for _, setting := range settings {
		switch {
		case setting.Key == "vcs.revision" && info.Commit == "":
			info.Commit = setting.Value[:min(len(setting.Value), 12)]
		case setting.Key == "vcs.time" && info.Date == "":
			info.Date = setting.Value[:min(len(setting.Value), 10)]
		}
	}
	return info
}

// String is the `cade version` line, e.g. "cade v0.0.0 (commit 8727192,
// 2026-09-27, CPU build, llama.cpp b11195)".
func (i Info) String() string {
	var details []string
	for _, detail := range []string{prefixed("commit ", i.Commit), i.Date, i.Accelerator + " build", prefixed("llama.cpp ", i.LlamaTag)} {
		if detail != "" {
			details = append(details, detail)
		}
	}
	return fmt.Sprintf("cade %s (%s)", i.Version, strings.Join(details, ", "))
}

func prefixed(label, value string) string {
	if value == "" {
		return ""
	}
	return label + value
}
