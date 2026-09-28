package devtasks

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestModelsDownloadsOnlyMissingModels(t *testing.T) {
	world := newTestWorld(t)
	tasks := world.tasks()
	world.writeFile(t, strings.TrimPrefix(tasks.Settings().ModelPath(EmbeddingModel), world.root+"/"), "already here")
	if err := tasks.Models(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(world.fetcher.URLs, " ") != generationModelURL+" "+visionProjectorURL {
		t.Errorf("downloaded %q, want the generation model and projector", world.fetcher.URLs)
	}
	body, err := os.ReadFile(tasks.Settings().ModelPath(VisionProjector))
	if err != nil || string(body) != visionProjectorURL {
		t.Errorf("projector = %q, %v", body, err)
	}
}

func TestEnsureModelsLeavesNoModelAfterAFailedDownload(t *testing.T) {
	world := newTestWorld(t)
	world.fetcher.FailWith = errors.New("connection reset")
	tasks := world.tasks()
	err := tasks.EnsureModels(RerankerModel)
	if err == nil || !strings.Contains(err.Error(), rerankerModelURL) {
		t.Errorf("err = %v, want it to name the URL", err)
	}
	if fileExists(tasks.Settings().ModelPath(RerankerModel)) {
		t.Error("a partial download was taken for the model")
	}
}

func TestHTTPFetcherCopiesTheBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("gguf")) }))
	defer server.Close()
	var body bytes.Buffer
	if err := (HTTPFetcher{Client: server.Client()}).Fetch(server.URL, &body); err != nil || body.String() != "gguf" {
		t.Errorf("body %q, err %v", body.String(), err)
	}
}

func TestHTTPFetcherRejectsAnErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	err := (HTTPFetcher{Client: server.Client()}).Fetch(server.URL, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want the 404 status", err)
	}
}
