LLAMA_TAG  := b11195
LLAMA_DIR  := third_party/llama.cpp
CUDA_ARCH  ?= 86
CUDA_HOME  ?= /opt/cuda
# nvcc rejects host compilers newer than it supports; Arch's cuda package
# exports a compatible one in NVCC_CCBIN (/etc/profile.d/cuda.sh).
NVCC_CCBIN ?=
MODELS_DIR ?= $(HOME)/.local/share/cade/models

EMBEDDING_MODEL_URL  := https://huggingface.co/nomic-ai/nomic-embed-text-v2-moe-GGUF/resolve/main/nomic-embed-text-v2-moe.Q4_K_M.gguf
GENERATION_MODEL_URL := https://huggingface.co/Qwen/Qwen2.5-3B-Instruct-GGUF/resolve/main/qwen2.5-3b-instruct-q4_k_m.gguf
EMBEDDING_MODEL      := $(MODELS_DIR)/$(notdir $(EMBEDDING_MODEL_URL))
GENERATION_MODEL     := $(MODELS_DIR)/$(notdir $(GENERATION_MODEL_URL))

# Only the llama library is built: no tools, server or downloader (LLAMA_CURL
# off), so nothing in the binary can reach the network (RNF1).
LLAMA_CMAKE_FLAGS := -G Ninja -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF \
	-DCMAKE_POSITION_INDEPENDENT_CODE=ON -DGGML_OPENMP=OFF -DLLAMA_CURL=OFF \
	-DLLAMA_BUILD_TESTS=OFF -DLLAMA_BUILD_EXAMPLES=OFF -DLLAMA_BUILD_TOOLS=OFF -DLLAMA_BUILD_SERVER=OFF

PREFIX ?= $(HOME)/.local

# ON tunes llama.cpp to this CPU. CI builds with OFF (AVX2, FMA, F16C: every
# x86-64 runner has them) because the cached library may run on another
# machine, where a native build can die with an illegal instruction.
LLAMA_NATIVE ?= ON

# Pinned so `make lint` and CI agree; CI builds it with this module's Go,
# since a golangci-lint built with an older Go refuses newer modules.
GOLANGCI_LINT_VERSION := v2.14.0

# Go test timeout for the model suites; CPU runners need hours, not minutes.
EVAL_TIMEOUT ?= 1h

# The SQLite driver only compiles FTS5 (keyword search) with this tag; every
# build and test needs it. GO_TAGS adds cuda for the GPU build.
comma := ,
TAGS = sqlite_fts5$(if $(GO_TAGS),$(comma)$(GO_TAGS))

.PHONY: build cuda install uninstall test cover test-models eval eval-plan eval-retrieval eval-injection eval-scale bench fmt fmt-check vet lint check llama llama-cuda models clean

build: llama
	go build -tags sqlite_fts5 -o bin/cade ./cmd/cade

# Installs whichever binary is in bin/ (CPU or CUDA); builds the CPU one if
# none exists, so `make cuda install` keeps the GPU build.
install:
	test -x bin/cade || $(MAKE) build
	install -Dm755 bin/cade $(DESTDIR)$(PREFIX)/bin/cade

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/cade

cuda: llama-cuda
	go build -tags sqlite_fts5,cuda -o bin/cade ./cmd/cade

test: llama
	go test -tags sqlite_fts5 ./...

# Same tests with a coverage profile; prints the per-function table and the
# total last.
cover: llama
	go test -tags sqlite_fts5 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# Also runs the llama.cpp binding against the real models.
test-models: llama models
	CADE_TEST_EMBEDDING_MODEL=$(EMBEDDING_MODEL) CADE_TEST_GENERATION_MODEL=$(GENERATION_MODEL) go test -tags sqlite_fts5 ./...

# Scores the question planner against testdata/queries/plan.json with the
# real model, on the GPU when the CUDA Toolkit is installed (GO_TAGS= forces
# the CPU build).
GO_TAGS ?= $(if $(wildcard $(CUDA_HOME)/bin/nvcc),cuda)

eval-plan: $(if $(filter cuda,$(GO_TAGS)),llama-cuda,llama) $(GENERATION_MODEL)
	CADE_TEST_GENERATION_MODEL=$(GENERATION_MODEL) go test -tags $(TAGS) -count=1 -v -timeout $(EVAL_TIMEOUT) -run TestPlanSuiteWithModel ./internal/queryplan

# Scores retrieval with the real embedder and SQLite store: the calibration
# set reports where the distance gates belong, the test set (never used for
# tuning) is checked against its floors. testdata/queries/retrieval/.
eval-retrieval: $(if $(filter cuda,$(GO_TAGS)),llama-cuda,llama) $(EMBEDDING_MODEL)
	CADE_TEST_EMBEDDING_MODEL=$(EMBEDDING_MODEL) CADE_EVAL_MODE=$(MODE) go test -tags $(TAGS) -count=1 -v -timeout $(EVAL_TIMEOUT) -run 'TestRetrieval(Calibration|Suite)WithModel' ./internal/retrievalsuite

# Answers the prompt-injection cases (testdata/queries/injection.json) with
# both real models: each reply must come from the real evidence, not from
# the event telling the model what to say.
eval-injection: $(if $(filter cuda,$(GO_TAGS)),llama-cuda,llama) $(EMBEDDING_MODEL) $(GENERATION_MODEL)
	CADE_TEST_EMBEDDING_MODEL=$(EMBEDDING_MODEL) CADE_TEST_GENERATION_MODEL=$(GENERATION_MODEL) go test -tags $(TAGS) -count=1 -v -timeout $(EVAL_TIMEOUT) -run TestInjectionWithModel ./internal/retrievalsuite

# Test-set metrics as the corpus grows with distractors, saved next to the
# benchmark baseline. Embeddings are cached in ~/.cache/cade/eval, so only
# the first run of a size pays for them (~3 ms per event on a GPU).
SCALE ?= 1000,10000
eval-scale: $(if $(filter cuda,$(GO_TAGS)),llama-cuda,llama) $(EMBEDDING_MODEL)
	CADE_TEST_EMBEDDING_MODEL=$(EMBEDDING_MODEL) CADE_EVAL_SCALE=$(SCALE) CADE_EVAL_MODE=$(MODE) go test -tags $(TAGS) -count=1 -v -timeout 3h \
		-run TestRetrievalScaleWithModel ./internal/retrievalsuite | tee bench/retrieval-scale.txt

eval: eval-plan eval-retrieval eval-injection

# Latency and memory: storage at 1k/10k/100k synthetic events (search,
# reads, writes, bytes per event), the models (embedding, question
# interpretation, answer generation) and a whole `cade ask` up to its first
# token, with the page cache warm and cold. Save the output to compare
# runs; GO_TAGS= measures the CPU build (bench/baseline-cpu.txt).
bench: $(if $(filter cuda,$(GO_TAGS)),llama-cuda,llama) models
	CADE_TEST_EMBEDDING_MODEL=$(EMBEDDING_MODEL) CADE_TEST_GENERATION_MODEL=$(GENERATION_MODEL) \
		go test -tags $(TAGS) -run '^$$' -bench . -benchtime 5x -timeout 2h ./internal/storage/sqlitestore ./internal/benchmarks

fmt:
	gofmt -w cmd internal

# Fails listing the files gofmt would change.
fmt-check:
	@unformatted=$$(gofmt -l cmd internal); test -z "$$unformatted" || { echo "gofmt would change:"; echo "$$unformatted"; exit 1; }

vet: llama
	go vet -tags sqlite_fts5 ./...

# Linters and exclusions are in .golangci.yml. Install the pinned version
# with: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
lint: llama
	golangci-lint run ./...

# What CI runs on every push and pull request.
check: fmt-check vet lint test

# Prints a variable for CI cache keys: make -s print-LLAMA_TAG
print-%:
	@echo '$($*)'

llama: $(LLAMA_DIR)/build/src/libllama.a

llama-cuda: $(LLAMA_DIR)/build-cuda/src/libllama.a

$(LLAMA_DIR)/build/src/libllama.a: | $(LLAMA_DIR)
	cmake -S $(LLAMA_DIR) -B $(LLAMA_DIR)/build $(LLAMA_CMAKE_FLAGS) -DGGML_NATIVE=$(LLAMA_NATIVE)
	cmake --build $(LLAMA_DIR)/build --target llama

$(LLAMA_DIR)/build-cuda/src/libllama.a: | $(LLAMA_DIR)
	test -x $(CUDA_HOME)/bin/nvcc || { echo "nvcc não encontrado em $(CUDA_HOME); instale o CUDA Toolkit (pacman -S cuda)"; exit 1; }
	cmake -S $(LLAMA_DIR) -B $(LLAMA_DIR)/build-cuda $(LLAMA_CMAKE_FLAGS) -DGGML_NATIVE=$(LLAMA_NATIVE) \
		-DGGML_CUDA=ON -DCMAKE_CUDA_ARCHITECTURES=$(CUDA_ARCH) -DCMAKE_CUDA_COMPILER=$(CUDA_HOME)/bin/nvcc \
		$(if $(NVCC_CCBIN),-DCMAKE_CUDA_HOST_COMPILER=$(NVCC_CCBIN))
	cmake --build $(LLAMA_DIR)/build-cuda --target llama

$(LLAMA_DIR):
	git clone --depth 1 --branch $(LLAMA_TAG) https://github.com/ggml-org/llama.cpp $(LLAMA_DIR)

models: $(EMBEDDING_MODEL) $(GENERATION_MODEL)

$(EMBEDDING_MODEL) $(GENERATION_MODEL):
	mkdir -p $(MODELS_DIR)
	curl -fL -o $@ $(if $(filter $@,$(EMBEDDING_MODEL)),$(EMBEDDING_MODEL_URL),$(GENERATION_MODEL_URL))

clean:
	rm -rf bin coverage.out $(LLAMA_DIR)/build $(LLAMA_DIR)/build-cuda
