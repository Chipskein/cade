// Package devtasks holds the logic behind the Mage targets in magefiles/:
// building llama.cpp and cade, tests, evaluations, downloads and the
// release archive. Nothing here is linked into the cade binary.
package devtasks

import (
	"os"
	"path/filepath"
	"strings"
)

// EnvVar names an environment variable that overrides a build setting,
// e.g. `GO_TAGS= go tool mage bench`.
type EnvVar string

const (
	EnvCudaArch    EnvVar = "CUDA_ARCH"
	EnvCudaHome    EnvVar = "CUDA_HOME"
	EnvNvccCCBin   EnvVar = "NVCC_CCBIN"
	EnvLlamaNative EnvVar = "LLAMA_NATIVE"
	EnvModelsDir   EnvVar = "MODELS_DIR"
	EnvPrefix      EnvVar = "PREFIX"
	EnvDestDir     EnvVar = "DESTDIR"
	EnvEvalTimeout EnvVar = "EVAL_TIMEOUT"
	EnvGoTags      EnvVar = "GO_TAGS"
	EnvFuzzTime    EnvVar = "FUZZTIME"
	EnvScale       EnvVar = "SCALE"
	EnvScaleTopK   EnvVar = "SCALE_TOP_K"
	EnvScaleReport EnvVar = "SCALE_REPORT"
	EnvEvalMode    EnvVar = "MODE"
	EnvVersion     EnvVar = "VERSION"
	EnvCommit      EnvVar = "COMMIT"
	EnvBuildDate   EnvVar = "BUILD_DATE"
	EnvHome        EnvVar = "HOME"

	EnvEmbeddingModel  EnvVar = "EMBEDDING_MODEL"
	EnvGenerationModel EnvVar = "GENERATION_MODEL"
	EnvVisionProjector EnvVar = "VISION_PROJECTOR"
	EnvRerankerModel   EnvVar = "RERANKER_MODEL"
)

// settingDefaults are the values used when the variable is unset. HOME-based
// paths are built in LoadSettings.
var settingDefaults = map[EnvVar]string{
	EnvCudaArch: "86",
	EnvCudaHome: "/opt/cuda",
	// ON tunes llama.cpp to this CPU. CI builds with OFF (AVX2, FMA, F16C:
	// every x86-64 runner has them) because the cached library may run on
	// another machine, where a native build can die with an illegal
	// instruction.
	EnvLlamaNative: "ON",
	// Go's 10-minute test limit is too short for the model suites on CPU.
	EnvEvalTimeout: "1h",
	EnvFuzzTime:    "30s",
	EnvScale:       "1000,10000",
	EnvScaleTopK:   "6",
	EnvScaleReport: "bench/retrieval-scale.txt",
}

const (
	modelsDirUnderHome = ".local/share/cade/models"
	prefixUnderHome    = ".local"
	nvccUnderCudaHome  = "bin/nvcc"
	goTagSeparator     = ","
)

// Environment reads the variables that override settings; ProcessEnvironment
// is the real one.
type Environment interface {
	LookupEnv(name EnvVar) (string, bool)
}

// ProcessEnvironment reads this process's environment.
type ProcessEnvironment struct{}

// LookupEnv returns the variable's value and whether it is set at all, so
// an empty GO_TAGS can differ from an unset one.
func (ProcessEnvironment) LookupEnv(name EnvVar) (string, bool) {
	return os.LookupEnv(string(name))
}

// Settings are the knobs of the build; every field can be overridden by the
// EnvVar of the same name.
type Settings struct {
	CudaArch    string
	CudaHome    string
	NvccCCBin   string
	LlamaNative string
	ModelsDir   string
	Prefix      string
	DestDir     string
	EvalTimeout string
	FuzzTime    string
	Scale       string
	ScaleTopK   string
	ScaleReport string
	EvalMode    string
	// ExtraGoTags are added to sqlite_fts5 by the eval and bench targets.
	ExtraGoTags []string
	modelPaths  map[Model]string
}

// LoadSettings applies env over the defaults.
//
//	settings := LoadSettings(ProcessEnvironment{})
func LoadSettings(env Environment) Settings {
	home := setting(env, EnvHome)
	settings := Settings{
		CudaArch: setting(env, EnvCudaArch), CudaHome: setting(env, EnvCudaHome),
		NvccCCBin: setting(env, EnvNvccCCBin), LlamaNative: setting(env, EnvLlamaNative),
		ModelsDir: settingOr(env, EnvModelsDir, filepath.Join(home, modelsDirUnderHome)),
		Prefix:    settingOr(env, EnvPrefix, filepath.Join(home, prefixUnderHome)),
		DestDir:   setting(env, EnvDestDir), EvalTimeout: setting(env, EnvEvalTimeout),
		FuzzTime: setting(env, EnvFuzzTime), Scale: setting(env, EnvScale),
		ScaleTopK: setting(env, EnvScaleTopK), ScaleReport: setting(env, EnvScaleReport),
		EvalMode: setting(env, EnvEvalMode),
	}
	settings.ExtraGoTags = extraGoTags(env, settings.CudaHome)
	settings.modelPaths = modelPaths(env, settings.ModelsDir)
	return settings
}

// NvccPath is where the CUDA Toolkit's compiler should be.
func (s Settings) NvccPath() string {
	return filepath.Join(s.CudaHome, nvccUnderCudaHome)
}

// ModelPath is the local file of model.
func (s Settings) ModelPath(model Model) string {
	return s.modelPaths[model]
}

func setting(env Environment, name EnvVar) string {
	return settingOr(env, name, settingDefaults[name])
}

func settingOr(env Environment, name EnvVar, fallback string) string {
	value, found := env.LookupEnv(name)
	if !found {
		return fallback
	}
	return value
}

// extraGoTags defaults to cuda when the CUDA Toolkit is installed, so the
// evaluations run on the GPU; GO_TAGS= forces the CPU build.
func extraGoTags(env Environment, cudaHome string) []string {
	value, found := env.LookupEnv(EnvGoTags)
	if !found {
		return autodetectedGoTags(cudaHome)
	}
	var tags []string
	for _, tag := range strings.Split(value, goTagSeparator) {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func autodetectedGoTags(cudaHome string) []string {
	if !fileExists(filepath.Join(cudaHome, nvccUnderCudaHome)) {
		return nil
	}
	return []string{cudaTag}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
