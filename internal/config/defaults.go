package config

// allGPULayers offloads every layer in CUDA builds; both default models
// together need about 3 GB of VRAM.
const allGPULayers = -1

// Defaults returns the configuration used when no file overrides it. Paths
// may start with "~"; Load expands them.
func Defaults() Config {
	return Config{
		DatabasePath: "~/.local/share/cade/cade.db",
		Embedding:    defaultEmbedding(),
		// 3B rather than 1.5B: the smaller model answered SEM_INFORMACAO to
		// scoped listing questions and rarely cited evidence.
		Generation: ModelConfig{
			ModelPath:     "~/.local/share/cade/models/qwen2.5-3b-instruct-q4_k_m.gguf",
			ContextTokens: 8192,
			GPULayers:     allGPULayers,
		},
		// 0.72 was calibrated on nomic-embed-text-v2-moe: relevant hits fell
		// at 0.59–0.71, unrelated ones mostly above 0.74. The LLM's
		// SEM_INFORMACAO reply is the final guard for the overlap. 0.61 gates
		// whole unfiltered questions: the midpoint the calibration set of the
		// retrieval suite reports with chunked vectors (closest event at most
		// 0.596 when something answered, at least 0.624 when not); the test
		// set, never used for tuning, checks it.
		// Ranking a filtered event reads its vectors, ~0.19 ms each
		// (BenchmarkChunksFor): 1000 stay under 0.2 s, about one filtered
		// vector search at 100k events. More go through the vector index.
		Retrieval: RetrievalConfig{TopK: 8, MaxDistance: 0.72, MaxBestDistance: 0.61, MaxAnswerTokens: 512, Mode: "hybrid",
			MaxFilteredEvents: 1000},
		Sources: defaultSources(),
		Tasks:   TasksConfig{TaskURLPatterns: defaultTaskURLPatterns},
		UI:      UIConfig{Language: "auto", DateOrder: "auto"},
	}
}

// defaultTaskURLPatterns cover common trackers; each only matches its own
// URLs, so unused ones cost nothing.
var defaultTaskURLPatterns = []string{
	`proj4\.me/projects/(\d+)/tasks/(\d+)`,
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
		MaxFileBytes:       256 * 1024,
	}
}
