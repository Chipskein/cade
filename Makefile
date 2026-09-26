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

.PHONY: build cuda install uninstall test test-models eval eval-plan eval-retrieval fmt llama llama-cuda models clean

build: llama
	go build -o bin/cade ./cmd/cade

# Installs whichever binary is in bin/ (CPU or CUDA); builds the CPU one if
# none exists, so `make cuda install` keeps the GPU build.
install:
	test -x bin/cade || $(MAKE) build
	install -Dm755 bin/cade $(DESTDIR)$(PREFIX)/bin/cade

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/cade

cuda: llama-cuda
	go build -tags cuda -o bin/cade ./cmd/cade

test: llama
	go test ./...

# Also runs the llama.cpp binding against the real models.
test-models: llama models
	CADE_TEST_EMBEDDING_MODEL=$(EMBEDDING_MODEL) CADE_TEST_GENERATION_MODEL=$(GENERATION_MODEL) go test ./...

# Scores the question planner against testdata/queries/plan.json with the
# real model, on the GPU when the CUDA Toolkit is installed (GO_TAGS= forces
# the CPU build).
GO_TAGS ?= $(if $(wildcard $(CUDA_HOME)/bin/nvcc),cuda)

eval-plan: $(if $(filter cuda,$(GO_TAGS)),llama-cuda,llama) $(GENERATION_MODEL)
	CADE_TEST_GENERATION_MODEL=$(GENERATION_MODEL) go test $(if $(GO_TAGS),-tags $(GO_TAGS)) -count=1 -v -run TestPlanSuiteWithModel ./internal/queryplan

# Scores retrieval (recall, MRR, rejection) on testdata/queries/retrieval.json
# with the real embedder and SQLite store.
eval-retrieval: $(if $(filter cuda,$(GO_TAGS)),llama-cuda,llama) $(EMBEDDING_MODEL)
	CADE_TEST_EMBEDDING_MODEL=$(EMBEDDING_MODEL) go test $(if $(GO_TAGS),-tags $(GO_TAGS)) -count=1 -v -run TestRetrievalSuiteWithModel ./internal/retrievalsuite

eval: eval-plan eval-retrieval

fmt:
	gofmt -w cmd internal

llama: $(LLAMA_DIR)/build/src/libllama.a

llama-cuda: $(LLAMA_DIR)/build-cuda/src/libllama.a

$(LLAMA_DIR)/build/src/libllama.a: | $(LLAMA_DIR)
	cmake -S $(LLAMA_DIR) -B $(LLAMA_DIR)/build $(LLAMA_CMAKE_FLAGS) -DGGML_NATIVE=ON
	cmake --build $(LLAMA_DIR)/build --target llama

$(LLAMA_DIR)/build-cuda/src/libllama.a: | $(LLAMA_DIR)
	test -x $(CUDA_HOME)/bin/nvcc || { echo "nvcc não encontrado em $(CUDA_HOME); instale o CUDA Toolkit (pacman -S cuda)"; exit 1; }
	cmake -S $(LLAMA_DIR) -B $(LLAMA_DIR)/build-cuda $(LLAMA_CMAKE_FLAGS) -DGGML_NATIVE=ON \
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
	rm -rf bin $(LLAMA_DIR)/build $(LLAMA_DIR)/build-cuda
