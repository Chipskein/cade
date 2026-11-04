package config

import (
	"strings"
	"testing"
)

func TestPDFsAreOffByDefaultWithLimits(t *testing.T) {
	cfg := Defaults()
	if cfg.Sources.PDFs {
		t.Error("sources.pdfs must start off: a new source is opt-in")
	}
	if cfg.Ingest.MaxPDFBytes <= 0 || cfg.Ingest.MaxPDFPages <= 0 {
		t.Errorf("limits = %d bytes, %d pages", cfg.Ingest.MaxPDFBytes, cfg.Ingest.MaxPDFPages)
	}
}

func TestPDFLimitsAreCheckedOnlyWhenOn(t *testing.T) {
	cfg := Defaults()
	cfg.Ingest.MaxPDFBytes, cfg.Ingest.MaxPDFPages = 0, 0
	if err := cfg.validatePDFLimits(); err != nil {
		t.Errorf("off: %v", err)
	}
	cfg.Sources.PDFs = true
	if err := cfg.validatePDFLimits(); err == nil || !strings.Contains(err.Error(), "ingest.max_pdf_bytes is 0") {
		t.Errorf("no byte limit: error = %v", err)
	}
	cfg.Ingest.MaxPDFBytes = 1
	if err := cfg.validatePDFLimits(); err == nil || !strings.Contains(err.Error(), "ingest.max_pdf_pages is 0") {
		t.Errorf("no page limit: error = %v", err)
	}
	cfg.Ingest.MaxPDFPages = 1
	if err := cfg.validate(); err != nil {
		t.Errorf("valid limits: %v", err)
	}
}
