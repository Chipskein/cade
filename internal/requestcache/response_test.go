package requestcache

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/webstore"
	"github.com/klauspost/compress/gzip"
)

const (
	messagesURL  = "https://discord.com/api/v9/channels/42/messages?limit=50"
	messagesBody = `[{"id":"7","channel_id":"42","content":"olá"}]`
)

func gzipped(t *testing.T, plain string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	if _, err := writer.Write([]byte(plain)); err != nil {
		t.Fatalf("gzip: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return out.Bytes()
}

func TestRecordDecodesTheBodyIntoTheValue(t *testing.T) {
	response := Response{URL: messagesURL, ContentEncoding: "gzip", Body: gzipped(t, messagesBody)}
	record := response.Record(webstore.KindHTTPCache, "discord-messages")
	if record.DecodeErr != nil || record.Value.Items[0].Get("content").String() != "olá" {
		t.Fatalf("Record = %+v; want the decoded message", record)
	}
}

func TestRecordLocatesTheResponse(t *testing.T) {
	response := Response{URL: messagesURL, Namespace: "api-v1", Body: []byte(messagesBody)}
	record := response.Record(webstore.KindCacheAPI, "discord-messages")
	want := webstore.Record{Kind: webstore.KindCacheAPI, Origin: "https://discord.com", Namespace: "api-v1", Container: "discord-messages", Key: messagesURL}
	record.Value = nil
	if record != want {
		t.Fatalf("Record = %+v; want %+v", record, want)
	}
}

func TestRecordKeepsABodyThatIsNotJSON(t *testing.T) {
	record := Response{URL: messagesURL, Body: []byte("<html>")}.Record(webstore.KindHTTPCache, "discord-messages")
	if record.Value != nil || record.DecodeErr == nil || !strings.Contains(record.DecodeErr.Error(), messagesURL) {
		t.Fatalf("Record = %+v; want a DecodeErr naming the URL", record)
	}
}

func TestRecordKeepsABodyThatDoesNotDecompress(t *testing.T) {
	record := Response{URL: messagesURL, ContentEncoding: "br", Body: []byte(messagesBody)}.Record(webstore.KindHTTPCache, "discord-messages")
	if record.DecodeErr == nil || record.Container != "discord-messages" {
		t.Fatalf("Record = %+v; want a counted record with DecodeErr", record)
	}
}
