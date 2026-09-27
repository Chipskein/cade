package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestStringNamesEveryDetail(t *testing.T) {
	info := Info{Version: "v0.1.0", Commit: "8727192", Date: "2026-09-27", Accelerator: "CPU", LlamaTag: "b11195"}
	if got, want := info.String(), "cade v0.1.0 (commit 8727192, 2026-09-27, CPU build, llama.cpp b11195)"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestStringSkipsUnknownDetails(t *testing.T) {
	if got, want := (Info{Version: "dev", Accelerator: "CUDA"}).String(), "cade dev (CUDA build)"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestVCSStampFillsOnlyWhatIsMissing(t *testing.T) {
	settings := []debug.BuildSetting{{Key: "vcs.revision", Value: "8727192abcdef0123456789"}, {Key: "vcs.time", Value: "2026-09-27T05:40:00Z"}}
	if got := withVCSStamp(Info{}, settings); got.Commit != "8727192abcde" || got.Date != "2026-09-27" {
		t.Fatalf("expected the stamp's commit and day, got %+v", got)
	}
	if got := withVCSStamp(Info{Commit: "set", Date: "set"}, settings); got.Commit != "set" || got.Date != "set" {
		t.Fatalf("expected -ldflags values kept, got %+v", got)
	}
}

func TestReadDefaultsToDev(t *testing.T) {
	if info := Read(); info.Version != "dev" || info.Accelerator != "CPU" {
		t.Fatalf("expected an unversioned CPU test binary, got %+v", info)
	}
}
