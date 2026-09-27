LLAMA_TAG  := b11195
LLAMA_DIR  := third_party/llama.cpp
CUDA_ARCH  ?= 86
CUDA_HOME  ?= /opt/cuda
# nvcc rejects host compilers newer than it supports; Arch's cuda package
# exports a compatible one in NVCC_CCBIN (/etc/profile.d/cuda.sh).
NVCC_CCBIN ?=
MODELS_DIR ?= $(HOME)/.local/share/cade/models

EMBEDDING_MODEL_URL  := https://huggingface.co/nomic-ai/nomic-embed-text-v2-moe-GGUF/resolve/main/nomic-embed-text-v2-moe.Q4_K_M.gguf
# Qwen publishes no GGUF of Qwen3.5; unsloth's conversion is pinned to a
# commit because the repository is re-uploaded when templates are fixed.
QWEN35_URL           := https://huggingface.co/unsloth/Qwen3.5-2B-GGUF/resolve/f6d5376be1edb4d416d56da11e5397a961aca8ae
GENERATION_MODEL_URL := $(QWEN35_URL)/Qwen3.5-2B-Q4_K_M.gguf
# The vision projector (phase 19); every size's file is named mmproj-F16.gguf,
# so the local copy carries the model's name.
VISION_PROJECTOR_URL := $(QWEN35_URL)/mmproj-F16.gguf
EMBEDDING_MODEL      := $(MODELS_DIR)/$(notdir $(EMBEDDING_MODEL_URL))
GENERATION_MODEL     := $(MODELS_DIR)/$(notdir $(GENERATION_MODEL_URL))
VISION_PROJECTOR     := $(MODELS_DIR)/mmproj-Qwen3.5-2B-F16.gguf

# Only the llama and mtmd (vision) libraries are built: no tools, server or
# downloader (LLAMA_CURL off), so nothing in the binary can reach the network
# (RNF1). Without subprocesses mtmd has no video, which would run ffmpeg.
LLAMA_CMAKE_FLAGS := -G Ninja -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF \
	-DCMAKE_POSITION_INDEPENDENT_CODE=ON -DGGML_OPENMP=OFF -DLLAMA_CURL=OFF \
	-DLLAMA_BUILD_TESTS=OFF -DLLAMA_BUILD_EXAMPLES=OFF -DLLAMA_BUILD_TOOLS=OFF -DLLAMA_BUILD_SERVER=OFF \
	-DLLAMA_BUILD_COMMON=OFF -DLLAMA_BUILD_MTMD=ON -DLLAMA_SUBPROCESS=OFF -DMTMD_VIDEO=OFF

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

# `cade version`: the tag (or commit) and the commit's date, so two builds
# of one commit report the same thing.
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null)
BUILD_DATE ?= $(shell git log -1 --format=%cd --date=format:%Y-%m-%d 2>/dev/null)
BUILDINFO  := github.com/chipskein/cade/internal/buildinfo
LDFLAGS    := -X $(BUILDINFO).version=$(VERSION) -X $(BUILDINFO).commit=$(COMMIT) \
	-X $(BUILDINFO).date=$(BUILD_DATE) -X $(BUILDINFO).llamaTag=$(LLAMA_TAG)

.PHONY: build cuda install uninstall dist test cover fuzz test-models eval eval-plan eval-retrieval eval-injection eval-scale bench fmt fmt-check vet lint check llama llama-cuda models clean

build: llama
	go build -tags sqlite_fts5 -ldflags "$(LDFLAGS)" -o bin/cade ./cmd/cade

# Installs whichever binary is in bin/ (CPU or CUDA); builds the CPU one if
# none exists, so `make cuda install` keeps the GPU build.
install:
	test -x bin/cade || $(MAKE) build
	install -Dm755 bin/cade $(DESTDIR)$(PREFIX)/bin/cade

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/cade

# The release archive: the CPU binary with the licenses and docs a user
# needs, plus its SHA-256. The release workflow builds it with
# LLAMA_NATIVE=OFF so it runs on any x86-64 CPU with AVX2; CUDA stays a
# local build (it ties the binary to a driver and GPU architecture).
DIST_NAME  = cade-$(VERSION)-linux-$(shell go env GOARCH)-cpu
DIST_FILES := LICENSE THIRD_PARTY_NOTICES.md README.md README.pt-BR.md PRIVACY.md PRIVACY.pt-BR.md \
	CHANGELOG.md CHANGELOG.pt-BR.md config.example.json

dist: build
	rm -rf dist/$(DIST_NAME) && mkdir -p dist/$(DIST_NAME)
	cp bin/cade $(DIST_FILES) dist/$(DIST_NAME)/
	tar -C dist -czf dist/$(DIST_NAME).tar.gz $(DIST_NAME)
	cd dist && sha256sum $(DIST_NAME).tar.gz > $(DIST_NAME).tar.gz.sha256

cuda: llama-cuda
	go build -tags sqlite_fts5,cuda -ldflags "$(LDFLAGS)" -o bin/cade ./cmd/cade

test: llama
	go test -tags sqlite_fts5 ./...

# Same tests with a coverage profile; prints the per-function table and the
# total last.
cover: llama
	go test -tags sqlite_fts5 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# Fuzzes the Teams cache parsers, each target for FUZZTIME; what a run
# finds lands in testdata/fuzz/ and is replayed by every `make test`.
FUZZTIME ?= 30s
FUZZ_TARGETS := leveldbraw:FuzzJournalBatches leveldbraw:FuzzDecodeBatch leveldbraw:FuzzTableEntries \
	leveldbraw:FuzzBlockEntries v8value:FuzzDecode indexeddb:FuzzDecodeKeyPrefix indexeddb:FuzzDecodeRecords \
	ingest/teamssource:FuzzCollectReplyChain

fuzz:
	@for target in $(FUZZ_TARGETS); do \
		pkg=$${target%%:*}; name=$${target##*:}; \
		echo "== $$pkg $$name"; \
		go test -tags sqlite_fts5 -run '^$$' -fuzz "^$$name\$$" -fuzztime $(FUZZTIME) ./internal/$$pkg || exit 1; \
	done

# Also runs the llama.cpp binding against the real models.
test-models: llama models
	CADE_TEST_EMBEDDING_MODEL=$(EMBEDDING_MODEL) CADE_TEST_GENERATION_MODEL=$(GENERATION_MODEL) \
		CADE_TEST_VISION_PROJECTOR=$(VISION_PROJECTOR) go test -tags sqlite_fts5 ./...

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

# libmtmd.a is the last library built, so a build directory from before
# mtmd was added is reconfigured and completed.
llama: $(LLAMA_DIR)/build/tools/mtmd/libmtmd.a

llama-cuda: $(LLAMA_DIR)/build-cuda/tools/mtmd/libmtmd.a

$(LLAMA_DIR)/build/tools/mtmd/libmtmd.a: | $(LLAMA_DIR)
	cmake -S $(LLAMA_DIR) -B $(LLAMA_DIR)/build $(LLAMA_CMAKE_FLAGS) -DGGML_NATIVE=$(LLAMA_NATIVE)
	cmake --build $(LLAMA_DIR)/build --target llama mtmd

$(LLAMA_DIR)/build-cuda/tools/mtmd/libmtmd.a: | $(LLAMA_DIR)
	test -x $(CUDA_HOME)/bin/nvcc || { echo "nvcc não encontrado em $(CUDA_HOME); instale o CUDA Toolkit (pacman -S cuda)"; exit 1; }
	cmake -S $(LLAMA_DIR) -B $(LLAMA_DIR)/build-cuda $(LLAMA_CMAKE_FLAGS) -DGGML_NATIVE=$(LLAMA_NATIVE) \
		-DGGML_CUDA=ON -DCMAKE_CUDA_ARCHITECTURES=$(CUDA_ARCH) -DCMAKE_CUDA_COMPILER=$(CUDA_HOME)/bin/nvcc \
		$(if $(NVCC_CCBIN),-DCMAKE_CUDA_HOST_COMPILER=$(NVCC_CCBIN))
	cmake --build $(LLAMA_DIR)/build-cuda --target llama mtmd

$(LLAMA_DIR):
	git clone --depth 1 --branch $(LLAMA_TAG) https://github.com/ggml-org/llama.cpp $(LLAMA_DIR)

models: $(EMBEDDING_MODEL) $(GENERATION_MODEL) $(VISION_PROJECTOR)

# Downloads to a .part file first, so an interrupted download is not taken
# for a model on the next run.
download = mkdir -p $(dir $@) && curl -fL -o $@.part $(1) && mv $@.part $@

$(EMBEDDING_MODEL):
	$(call download,$(EMBEDDING_MODEL_URL))

$(GENERATION_MODEL):
	$(call download,$(GENERATION_MODEL_URL))

$(VISION_PROJECTOR):
	$(call download,$(VISION_PROJECTOR_URL))

clean:
	rm -rf bin dist coverage.out $(LLAMA_DIR)/build $(LLAMA_DIR)/build-cuda
