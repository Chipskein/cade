package chromiumstorage

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/sitestorage"
	"github.com/chipskein/cade/internal/testcheck"
	"github.com/chipskein/cade/internal/testfakes"
	"github.com/chipskein/cade/internal/webstore"
)

const (
	chatGPTDir = "004"
	teamsDir   = "003"
	chatJSON   = `{"messages":[{"id":"m1","text":"oi"}]}`
	stateJSON  = `{"open":"c1"}`
)

// pickledFileInfo lays out a FileInfo as the Paths database keeps it.
func pickledFileInfo(parent int64, dataPath, name string) []byte {
	payload := binary.LittleEndian.AppendUint64(nil, uint64(parent))
	for _, text := range []string{dataPath, name} {
		payload = binary.LittleEndian.AppendUint32(payload, uint32(len(text)))
		payload = append(payload, text...)
		payload = append(payload, make([]byte, (pickleAlignment-len(text)%pickleAlignment)%pickleAlignment)...)
	}
	payload = binary.LittleEndian.AppendUint64(payload, 13_300_000_000_000_000)
	return append(binary.LittleEndian.AppendUint32(nil, uint32(len(payload))), payload...)
}

// opfsEntry is one entry of a synthetic tree; content is written to its
// data path when the entry is a file.
type opfsEntry struct {
	id       int64
	parent   int64
	dataPath string
	name     string
	content  string
}

// writeFileSystem lays out a profile's "File System": the Origins
// database naming ChatGPT's and Teams' directories, and ChatGPT's tree.
func writeFileSystem(t *testing.T, entries []opfsEntry) string {
	t.Helper()
	fileSystem := filepath.Join(t.TempDir(), "File System")
	var origins testfakes.LevelDBJournal
	origins.Put([]byte(originKeyPrefix+"https_chatgpt.com_0"), []byte(chatGPTDir))
	origins.Put([]byte(originKeyPrefix+"https_teams.cloud.microsoft_0"), []byte(teamsDir))
	writeJournal(t, filepath.Join(fileSystem, originsDir), &origins)
	for _, dir := range []string{chatGPTDir, teamsDir} {
		writeTree(t, filepath.Join(fileSystem, dir), entries)
	}
	return fileSystem
}

func writeTree(t *testing.T, location string, entries []opfsEntry) {
	t.Helper()
	var paths testfakes.LevelDBJournal
	paths.Put([]byte("LAST_FILE_ID"), []byte("9"))
	paths.Put([]byte("0"), pickledFileInfo(0, "", ""))
	for _, entry := range entries {
		paths.Put([]byte(strconv.FormatInt(entry.id, 10)), pickledFileInfo(entry.parent, entry.dataPath, entry.name))
		if entry.content == "" {
			continue
		}
		diskPath := filepath.Join(location, sandboxTypeDir, entry.dataPath)
		testcheck.NoError(t, os.MkdirAll(filepath.Dir(diskPath), 0o700))
		testcheck.NoError(t, os.WriteFile(diskPath, []byte(entry.content), 0o600))
	}
	writeJournal(t, filepath.Join(location, sandboxTypeDir, pathsDir), &paths)
}

func writeJournal(t *testing.T, dir string, journal *testfakes.LevelDBJournal) {
	t.Helper()
	testcheck.NoError(t, os.MkdirAll(dir, 0o700))
	testcheck.NoError(t, journal.WriteDir(dir))
}

func chatTree() []opfsEntry {
	return []opfsEntry{
		{id: 1, parent: 0, name: "cache"},
		{id: 2, parent: 1, dataPath: "00/00000002", name: "c1.json", content: chatJSON},
		{id: 3, parent: 0, dataPath: "00/00000003", name: "state.json", content: stateJSON},
		{id: 4, parent: 0, dataPath: "../../escape", name: "escape.json"},
		{id: 5, parent: 99, dataPath: "00/00000005", name: "orphan.json", content: stateJSON},
	}
}

func opfsReaderOf(t *testing.T, origins ...string) OPFSReader {
	t.Helper()
	return NewOPFSReader(readerOf(t, origins...).scope)
}

func TestOPFSReadGivesTheReachableFilesByPath(t *testing.T) {
	fileSystem := writeFileSystem(t, chatTree())
	records, err := opfsReaderOf(t, chatGPT).Read(filepath.Join(fileSystem, chatGPTDir))
	testcheck.NoError(t, err)
	var keys []string
	for _, record := range records {
		keys = append(keys, record.Key)
	}
	if strings.Join(keys, ",") != "cache/c1.json,state.json" {
		t.Fatalf("Read keys = %v; want the two reachable files inside the directory", keys)
	}
}

func TestOPFSReadParsesJSONFilesInTheirDirectory(t *testing.T) {
	fileSystem := writeFileSystem(t, chatTree())
	records, err := opfsReaderOf(t, chatGPT).Read(filepath.Join(fileSystem, chatGPTDir))
	testcheck.NoError(t, err)
	chat := recordOf(records, "c1.json")
	if chat == nil || chat.Origin != chatGPT || chat.Kind != webstore.KindOPFS || chat.Namespace != "cache" || chat.Value.Get("messages").Items[0].Get("text").String() != "oi" {
		t.Fatalf("chat record = %+v; want the parsed file of ChatGPT in cache", chat)
	}
}

func TestOPFSReadRefusesAnOriginOutsideTheScope(t *testing.T) {
	fileSystem := writeFileSystem(t, chatTree())
	records, err := opfsReaderOf(t, chatGPT).Read(filepath.Join(fileSystem, teamsDir))
	if err == nil || records != nil || !strings.Contains(err.Error(), teams) {
		t.Fatalf("Read = %+v, %v; want a refusal naming Teams", records, err)
	}
}

func TestOPFSReadNamesADirectoryWithoutOrigin(t *testing.T) {
	fileSystem := writeFileSystem(t, chatTree())
	writeTree(t, filepath.Join(fileSystem, "009"), chatTree())
	if _, err := opfsReaderOf(t, chatGPT).Read(filepath.Join(fileSystem, "009")); err == nil || !strings.Contains(err.Error(), `"009"`) {
		t.Fatalf("Read error = %v; want one naming the directory", err)
	}
}

func TestOPFSReadRefusesAnEmptyScope(t *testing.T) {
	fileSystem := writeFileSystem(t, chatTree())
	if _, err := opfsReaderOf(t).Read(filepath.Join(fileSystem, chatGPTDir)); !errors.Is(err, sitestorage.ErrEmptyScope) {
		t.Fatalf("Read error = %v; want ErrEmptyScope", err)
	}
}

func TestOPFSRecognizesANumberedDirectoryBesideOrigins(t *testing.T) {
	fileSystem := writeFileSystem(t, chatTree())
	reader := opfsReaderOf(t, chatGPT)
	if !reader.Recognizes(filepath.Join(fileSystem, chatGPTDir)) || reader.Recognizes(fileSystem) || reader.Recognizes(filepath.Join(fileSystem, originsDir)) {
		t.Fatal("Recognizes wrong; want only the numbered directories")
	}
}

func TestOriginOfIdentifierUndoesStorageIdentifiers(t *testing.T) {
	for identifier, origin := range map[string]string{
		"https_teams.microsoft.com_0": "https://teams.microsoft.com",
		"http_localhost_8080":         "http://localhost:8080",
		"chrome-extension":            "chrome-extension",
	} {
		if got := originOfIdentifier(identifier); got != origin {
			t.Errorf("originOfIdentifier(%q) = %q; want %q", identifier, got, origin)
		}
	}
}

func TestDecodeFileInfoRejectsATruncatedPickle(t *testing.T) {
	pickled := pickledFileInfo(0, "00/00000002", "c1.json")
	if _, err := decodeFileInfo(pickled[:len(pickled)-9]); err == nil || !strings.Contains(err.Error(), "payload size") {
		t.Fatalf("decodeFileInfo error = %v; want the size mismatch named", err)
	}
	binary.LittleEndian.PutUint32(pickled[pickleHeaderLen+int64Len:], 1000)
	if _, err := decodeFileInfo(pickled); err == nil || !strings.Contains(err.Error(), "1000 bytes") {
		t.Fatalf("decodeFileInfo error = %v; want the overlong string named", err)
	}
}
