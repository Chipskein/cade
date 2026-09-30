// Package imagepreview draws a cited image in the terminal with the chafa
// program (https://github.com/hpjansson/chafa). chafa is optional: without
// it, or for anything that is not an image, there is simply no preview.
package imagepreview

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/chipskein/cade/internal/imagefile"
)

// ChafaProgram is the executable looked up on PATH.
const ChafaProgram = "chafa"

// PreviewSize bounds the preview in terminal cells; chafa keeps the aspect
// ratio inside it.
type PreviewSize struct {
	Columns int
	Rows    int
}

// DefaultPreviewSize is a thumbnail that fits under a citation line.
var DefaultPreviewSize = PreviewSize{Columns: 40, Rows: 20}

// CommandRunner runs a program and returns its stdout; ExecRunner is the
// real one, tests replace it.
type CommandRunner interface {
	Output(ctx context.Context, program string, args ...string) ([]byte, error)
}

// ExecRunner runs programs with os/exec.
type ExecRunner struct{}

// Output runs program, including its stderr in the error on failure.
func (ExecRunner) Output(ctx context.Context, program string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, program, args...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("run %s %v: %w: %s", program, args, err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

// Previewer renders images with chafa when LookPath finds it.
type Previewer struct {
	Runner   CommandRunner
	LookPath func(program string) (string, error)
	Size     PreviewSize
}

// Render returns path drawn as terminal text, or "" when path is not an
// image cade describes, chafa is not installed or chafa fails: a missing
// preview must never break the citation it decorates.
//
//	preview := imagepreview.Previewer{Runner: imagepreview.ExecRunner{}, LookPath: exec.LookPath, Size: imagepreview.DefaultPreviewSize}
//	fmt.Println(preview.Render(ctx, "/home/ana/Downloads/erro.png"))
func (p Previewer) Render(ctx context.Context, path string) string {
	if !imagefile.IsDescribedImage(path) {
		return ""
	}
	if _, err := p.LookPath(ChafaProgram); err != nil {
		return ""
	}
	output, err := p.Runner.Output(ctx, ChafaProgram, p.sizeFlag(), "--", path)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(output), "\n")
}

func (p Previewer) sizeFlag() string {
	return fmt.Sprintf("--size=%dx%d", p.Size.Columns, p.Size.Rows)
}
