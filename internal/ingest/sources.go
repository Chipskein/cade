package ingest

import (
	"fmt"

	"github.com/chipskein/cade/internal/event"
)

// SourceSpec registers one kind of source with the CLI. Adding a source
// (e.g. Teams, RF1.4) means appending a SourceSpec; nothing else changes.
type SourceSpec struct {
	// Name is what the user types: `cade ingest <Name> [target...]`.
	Name string
	// DefaultTargets are used when no target is given on the command line.
	DefaultTargets []string
	NewCollector   func(target string) (EventCollector, error)
}

// FindSource returns the spec named name.
func FindSource(specs []SourceSpec, name string) (SourceSpec, error) {
	for _, spec := range specs {
		if spec.Name == name {
			return spec, nil
		}
	}
	return SourceSpec{}, fmt.Errorf("unknown source %q, expected one of %v", name, SourceNames(specs))
}

// SourceNames lists the registered source names in order.
func SourceNames(specs []SourceSpec) []string {
	names := make([]string, len(specs))
	for i, spec := range specs {
		names[i] = spec.Name
	}
	return names
}

// ImageCaptions maps a file's absolute path to its image metadata, decided
// before the embedder loads (phase 19); a file absent from it is ingested
// as before.
type ImageCaptions map[string]event.Image
