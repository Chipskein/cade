package doctor

import (
	"bytes"
	"io"
	"io/fs"
	"path"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/gguf"
	"github.com/chipskein/cade/internal/rootfs"
)

// sizeLabelKey is the GGUF metadata key naming a model's parameter-count
// bucket (e.g. "4B"), set by the release's conversion for both a
// generation model and the mmproj released with it.
const sizeLabelKey = "general.size_label"

// File signatures: only these few bytes are read, never the content.
var (
	ggufMagic   = []byte("GGUF")
	sqliteMagic = []byte("SQLite format 3\x00")
)

// sourceCheck knows how to verify one source's configured targets.
type sourceCheck struct {
	source  string
	setting string
	targets []string
	verify  func(fsys fs.FS, target string) Problem
}

func checkConfigFile(fsys fs.FS, configPath string) Finding {
	finding := Finding{Subject: SubjectConfigFile, Path: configPath}
	if requireKind(fsys, configPath, false) != ProblemNone {
		finding.Problem = ProblemNoConfigFile
	}
	return finding
}

func checkModel(fsys fs.FS, subject Subject, setting, modelPath string) Finding {
	return Finding{Subject: subject, Setting: setting, Path: modelPath, Problem: requireHeader(fsys, modelPath, ggufMagic, ProblemNotGGUF)}
}

// checkVisionPairing compares generationPath's and projectorPath's GGUF
// "general.size_label" metadata; it stays quiet when either file lacks the
// key, since older conversions never set it.
func checkVisionPairing(fsys fs.FS, generationPath, projectorPath string) Finding {
	finding := Finding{Subject: SubjectVisionProjector, Setting: "vision.projector_path", Path: projectorPath}
	generationSize, ok := readSizeLabel(fsys, generationPath)
	if !ok {
		return finding
	}
	projectorSize, ok := readSizeLabel(fsys, projectorPath)
	if !ok || generationSize == projectorSize {
		return finding
	}
	finding.Problem = ProblemVisionModelMismatch
	finding.PairedModelPath, finding.PairedModelSize, finding.ProjectorSize = generationPath, generationSize, projectorSize
	return finding
}

// readSizeLabel reads target's "general.size_label" GGUF metadata; ok is
// false when the file can't be opened, isn't a GGUF, or lacks the key.
func readSizeLabel(fsys fs.FS, target string) (size string, ok bool) {
	file, err := fsys.Open(rootfs.Name(target))
	if err != nil {
		return "", false
	}
	defer file.Close()
	metadata, err := gguf.ReadStringMetadata(file, sizeLabelKey)
	if err != nil {
		return "", false
	}
	size, ok = metadata[sizeLabelKey]
	return size, ok
}

func checkSources(fsys fs.FS, sources config.SourcesConfig) []Finding {
	checks := []sourceCheck{
		{source: "git", setting: "sources.git_repositories", targets: sources.GitRepositories, verify: verifyGitRepository},
		{source: "browser", setting: "sources.browser_histories", targets: sources.BrowserHistories, verify: verifyBrowserHistory},
		{source: "file", setting: "sources.directories", targets: sources.Directories, verify: verifyDirectory},
		{source: "teams", setting: "sources.teams_indexeddb_dirs", targets: sources.TeamsIndexedDBDirs, verify: verifyLevelDB},
	}
	var findings []Finding
	for _, check := range checks {
		for _, target := range check.targets {
			findings = append(findings, Finding{Subject: SubjectSource, Source: check.source, Setting: check.setting, Path: target, Problem: check.verify(fsys, target)})
		}
	}
	if len(findings) == 0 {
		return []Finding{{Subject: SubjectSource, Problem: ProblemNoSources}}
	}
	return findings
}

func verifyGitRepository(fsys fs.FS, target string) Problem {
	return requireEntry(fsys, target, ".git", ProblemNotGitRepository)
}

func verifyBrowserHistory(fsys fs.FS, target string) Problem {
	return requireHeader(fsys, target, sqliteMagic, ProblemNotSQLite)
}

func verifyDirectory(fsys fs.FS, target string) Problem {
	return requireKind(fsys, target, true)
}

// verifyLevelDB looks for CURRENT, which every LevelDB directory has.
func verifyLevelDB(fsys fs.FS, target string) Problem {
	return requireEntry(fsys, target, "CURRENT", ProblemNotLevelDB)
}

// requireKind checks that target exists and is a directory (or a file).
func requireKind(fsys fs.FS, target string, directory bool) Problem {
	info, err := fs.Stat(fsys, rootfs.Name(target))
	switch {
	case err != nil:
		return ProblemMissing
	case directory && !info.IsDir():
		return ProblemNotADirectory
	case !directory && info.IsDir():
		return ProblemNotAFile
	}
	return ProblemNone
}

// requireEntry checks that the directory target holds entry.
func requireEntry(fsys fs.FS, target, entry string, absent Problem) Problem {
	if problem := requireKind(fsys, target, true); problem != ProblemNone {
		return problem
	}
	if _, err := fs.Stat(fsys, path.Join(rootfs.Name(target), entry)); err != nil {
		return absent
	}
	return ProblemNone
}

// requireHeader checks that the file target starts with magic.
func requireHeader(fsys fs.FS, target string, magic []byte, mismatch Problem) Problem {
	if problem := requireKind(fsys, target, false); problem != ProblemNone {
		return problem
	}
	file, err := fsys.Open(rootfs.Name(target))
	if err != nil {
		return ProblemMissing
	}
	defer file.Close()
	header := make([]byte, len(magic))
	if _, err := io.ReadFull(file, header); err != nil || !bytes.Equal(header, magic) {
		return mismatch
	}
	return ProblemNone
}
