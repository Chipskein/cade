package config

// allGPULayers offloads every layer in CUDA builds; both default models
// together need about 2.3 GB of VRAM.
const allGPULayers = -1

const (
	defaultGenerationModelPath  = "~/.local/share/cade/models/Qwen3.5-2B-Q4_K_M.gguf"
	defaultVisionProjectorPath  = "~/.local/share/cade/models/mmproj-Qwen3.5-2B-F16.gguf"
	previousGenerationModelPath = "~/.local/share/cade/models/qwen2.5-3b-instruct-q4_k_m.gguf"
)

// Defaults returns the configuration used when no file overrides it. Paths
// may start with "~"; Load expands them.
func Defaults() Config {
	return Config{
		DatabasePath: "~/.local/share/cade/cade.db",
		Embedding:    defaultEmbedding(),
		// Qwen3.5-2B (Apache-2.0) replaced Qwen2.5-3B (non-commercial
		// license): it matches or beats it on every plan-suite field and,
		// with the citation example in the answer prompt, cites every
		// injection case. The 4B does not fit the ~2.5 GB of VRAM budget.
		Generation: ModelConfig{
			ModelPath:     defaultGenerationModelPath,
			ContextTokens: 8192,
			GPULayers:     allGPULayers,
		},
		// 0.72 was calibrated on nomic-embed-text-v2-moe: relevant hits fell
		// at 0.59–0.71, unrelated ones mostly above 0.74. The LLM's
		// SEM_INFORMACAO reply is the final guard for the overlap. 0.61 gates
		// whole unfiltered questions: the midpoint the calibration set of the
		// retrieval suite reports with chunked vectors (closest event at most
		// 0.596 when something answered, at least 0.624 when not; 0.606 and
		// 0.621 on the CPU build); the test set, never used for tuning,
		// checks it. The store records the model
		// these gates belong to, and reindex and doctor warn when it changes.
		// top_k 6: the phase 17 sweep (docs/BENCHMARKS.md) kept recall and
		// MRR of 8 with two fewer events for the model to read.
		// Ranking a filtered event reads its vectors, ~0.19 ms each
		// (BenchmarkChunksFor): 1000 stay under 0.2 s, about one filtered
		// vector search at 100k events. More go through the vector index.
		Retrieval: RetrievalConfig{TopK: 6, MaxDistance: 0.72, MaxBestDistance: 0.61, MaxAnswerTokens: 512, Mode: "hybrid",
			MaxFilteredEvents: 1000},
		Sources: defaultSources(),
		Tasks:   TasksConfig{TaskURLPatterns: defaultTaskURLPatterns},
		UI:      UIConfig{Language: "auto", DateOrder: "auto"},
		Ingest:  defaultIngest(),
		Vision:  VisionConfig{ProjectorPath: defaultVisionProjectorPath, ContextTokens: visionContextTokens},
	}
}

// visionContextTokens holds a 1024 px image (~1000 tokens), the prompt
// and a 384-token description, with room to spare.
const visionContextTokens = 2048

// Image limits (phase 19). Describing takes ~1.7 s per screenshot on an RTX
// 3060 and ~22 s on a 6-core CPU (BenchmarkDescribeImage): 50 per run keep
// a first CPU ingestion under ~20 minutes, and the rest follow in later
// runs. 20 MiB covers phone photos; larger files are rarely screenshots.
const (
	defaultMaxImagesPerRun = 50
	defaultMaxImageBytes   = 20 << 20
)

func defaultIngest() IngestConfig {
	return IngestConfig{
		Redact:          true,
		Retention:       RetentionConfig{MaxAgeDays: map[string]int{"git": 0, "browser": 0, "file": 0, "teams": 0}},
		MaxImageBytes:   defaultMaxImageBytes,
		MaxImagesPerRun: defaultMaxImagesPerRun,
		Background:      defaultBackground(),
	}
}

// defaultTaskURLPatterns cover common trackers; each only matches its own
// URLs, so unused ones cost nothing.
var defaultTaskURLPatterns = []string{
	`atlassian\.net/browse/([A-Z][A-Z0-9]+-\d+)`,
	`linear\.app/[\w-]+/issue/([A-Z][A-Z0-9]+-\d+)`,
	`github\.com/([\w.-]+/[\w.-]+)/issues/(\d+)`,
	`dev\.azure\.com/[\w.-]+/[\w.%-]+/_workitems/edit/(\d+)`,
}

func defaultEmbedding() EmbeddingConfig {
	return EmbeddingConfig{
		ModelConfig: ModelConfig{
			// Multilingual: v1.5 is English-centric and ranked Portuguese
			// questions poorly.
			ModelPath: "~/.local/share/cade/models/nomic-embed-text-v2-moe.Q4_K_M.gguf",
			// 0 = the model's training context (512 for this model); a
			// larger value is capped there, since longer inputs degrade.
			ContextTokens: 0,
			GPULayers:     allGPULayers,
		},
		QueryPrefix:    "search_query: ",
		DocumentPrefix: "search_document: ",
	}
}

func defaultSources() SourcesConfig {
	return SourcesConfig{
		GitRepositories:    []string{},
		GitAuthors:         []string{},
		GitIdentities:      []string{"auto"},
		BrowserHistories:   []string{},
		TeamsIndexedDBDirs: []string{},
		Directories:        []string{},
		IgnoredDirNames:    []string{".git", "node_modules", "vendor", "__pycache__", ".venv", "target"},
		IgnoredFileGlobs:   defaultIgnoredFileGlobs,
		MaxFileBytes:       256 * 1024,
	}
}

var defaultIgnoredFileGlobs = []string{".env*", "*.pem", "*.key", "id_rsa*", "id_ed25519*", "*.p12", "*.pfx", "credentials*", ".netrc", ".npmrc", ".pypirc", ".git-credentials"}
