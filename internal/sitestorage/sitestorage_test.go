package sitestorage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

const chatGPT = "https://chatgpt.com"

func TestAllowsOnlyTheConfiguredOrigins(t *testing.T) {
	scope, err := NewScope([]string{chatGPT, "http://localhost:8080"})
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	for _, origin := range []string{chatGPT, "HTTPS://ChatGPT.com", "http://localhost:8080"} {
		if !scope.Allows(origin) {
			t.Errorf("Allows(%q) = false; want true", origin)
		}
	}
	for _, origin := range []string{"https://teams.cloud.microsoft", "http://chatgpt.com", "https://chatgpt.com.evil.test", "http://localhost"} {
		if scope.Allows(origin) {
			t.Errorf("Allows(%q) = true; want false", origin)
		}
	}
}

func TestNewScopeRejectsWhatIsNotAnOrigin(t *testing.T) {
	for _, origin := range []string{"chatgpt.com", "file:///tmp", "https://chatgpt.com/c/1", "https://chatgpt.com?x=1", "https://"} {
		_, err := NewScope([]string{origin})
		if err == nil || !strings.Contains(err.Error(), origin) {
			t.Errorf("NewScope(%q) error = %v; want one naming the value", origin, err)
		}
	}
}

func TestEmptyScopeReadsNothing(t *testing.T) {
	scope, err := NewScope(nil)
	if err != nil || !scope.Empty() || scope.Allows(chatGPT) {
		t.Fatalf("NewScope(nil) = %+v, %v; want an empty scope that allows nothing", scope, err)
	}
}

func TestRefusalNamesTheOriginAndLocation(t *testing.T) {
	err := Refusal(chatGPT, "/p/ls")
	if !strings.Contains(err.Error(), `"https://chatgpt.com"`) || !strings.Contains(err.Error(), `"/p/ls"`) {
		t.Fatalf("Refusal = %v; want the origin and the location", err)
	}
}

func TestTextValueParsesJSON(t *testing.T) {
	value := TextValue(`{"drafts":[{"content":"oi"}]}`)
	if got := value.Get("drafts").Items[0].Get("content").String(); got != "oi" {
		t.Fatalf("TextValue(JSON) content = %q; want oi", got)
	}
}

func TestTextValueKeepsOtherTextAsAString(t *testing.T) {
	value := TextValue("dark mode")
	if value.Kind != v8value.KindString || value.Text != "dark mode" {
		t.Fatalf("TextValue(text) = %+v; want the string itself", value)
	}
}

func TestUTF16LEDecodesSurrogatePairs(t *testing.T) {
	text, err := UTF16LE([]byte{'o', 0, 'i', 0, 0x3d, 0xd8, 0x00, 0xde})
	if err != nil || text != "oi😀" {
		t.Fatalf("UTF16LE = %q, %v; want oi😀", text, err)
	}
}

func TestUTF16LERejectsAnOddLength(t *testing.T) {
	if _, err := UTF16LE([]byte{'o', 0, 'i'}); err == nil || !strings.Contains(err.Error(), "3 bytes") {
		t.Fatalf("UTF16LE error = %v; want the length named", err)
	}
}

func writeOPFSFile(t *testing.T, content []byte) OPFSFile {
	t.Helper()
	diskPath := filepath.Join(t.TempDir(), "AB")
	if err := os.WriteFile(diskPath, content, 0o600); err != nil {
		t.Fatalf("write OPFS file: %v", err)
	}
	return OPFSFile{Origin: chatGPT, Dir: "cache/chats", Name: "c1.json", DiskPath: diskPath}
}

func TestOPFSFileRecordParsesJSON(t *testing.T) {
	record := writeOPFSFile(t, []byte(`{"messages":[{"text":"oi"}]}`)).Record()
	if record.DecodeErr != nil || record.Kind != webstore.KindOPFS || record.Namespace != "cache/chats" || record.Container != "c1.json" || record.Key != "cache/chats/c1.json" {
		t.Fatalf("Record = %+v; want the file located by directory and name", record)
	}
	if got := record.Value.Get("messages").Items[0].Get("text").String(); got != "oi" {
		t.Fatalf("Record text = %q; want oi", got)
	}
}

func TestOPFSFileRecordReportsAFileThatIsNotJSON(t *testing.T) {
	record := writeOPFSFile(t, []byte("SQLite format 3\x00")).Record()
	if record.Value != nil || record.DecodeErr == nil || !strings.Contains(record.DecodeErr.Error(), "cache/chats/c1.json") {
		t.Fatalf("Record = %+v; want a DecodeErr naming the file", record)
	}
}

func TestOPFSFileRecordLeavesALargeFileUnread(t *testing.T) {
	file := writeOPFSFile(t, nil)
	if err := os.Truncate(file.DiskPath, MaxFileBytes+1); err != nil {
		t.Fatalf("grow OPFS file: %v", err)
	}
	if record := file.Record(); record.DecodeErr == nil || !strings.Contains(record.DecodeErr.Error(), "at most") {
		t.Fatalf("Record = %+v; want a DecodeErr naming the size limit", record)
	}
}
