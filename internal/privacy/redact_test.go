package privacy

import (
	"github.com/chipskein/cade/internal/event"
	"strings"
	"testing"
)

func TestTextRedactsKnownCredentialFormats(t *testing.T) {
	fixtures := []string{"ghp_abcdefghijklmnopqrstuvwxyz123456", "github_pat_abcdefghijklmnopqrstuvwxyz123456", "glpat-abcdefghijklmnop", "AKIA1234567890ABCDEF", "xoxb-123456789012-abcdef", "eyJhbGciOiJub25lIn0.eyJzdWIiOiIxIn0.signature", "-----BEGIN RSA PRIVATE KEY-----\nsecret\n-----END RSA PRIVATE KEY-----"}
	joined := Text(strings.Join(fixtures, " "))
	for _, fixture := range fixtures {
		if strings.Contains(joined, fixture) {
			t.Errorf("secret %q remained in %q", fixture, joined)
		}
	}
	for _, label := range []string{"github-token", "gitlab-token", "aws-key", "slack-token", "jwt", "private-key"} {
		if !strings.Contains(joined, "[redacted:"+label+"]") {
			t.Errorf("expected label %q in %q", label, joined)
		}
	}
}

func TestTextRedactsCredentialsEmbeddedInWords(t *testing.T) {
	secret := "ghp_abcdefghijklmnopqrstuvwxyz123456"
	got := Text("x" + secret + "!")
	want := "x[redacted:github-token]!"
	if got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}

func TestEventRedactsTextAndMetadataForEverySource(t *testing.T) {
	secret := "ghp_abcdefghijklmnopqrstuvwxyz123456"
	for _, source := range []event.Source{event.SourceGit, event.SourceBrowser, event.SourceFile, event.SourceTeams} {
		ev := Event(event.Event{Source: source, Content: "message " + secret, Metadata: event.Metadata{"text": secret, "url": "https://example.com/?token=secret"}}, true)
		if strings.Contains(ev.Content, secret) || strings.Contains(ev.Metadata["text"], secret) || strings.Contains(ev.Metadata["url"], "token=") {
			t.Errorf("%s event retained secret: %+v", source, ev)
		}
	}
}

func TestURLRemovesSecretQueryParametersAndKeepsPageIdentity(t *testing.T) {
	got := URL("https://example.com/page?code=abc&topic=go&X-Amz-Signature=secret")
	if got != "https://example.com/page?topic=go" {
		t.Fatalf("URL() = %q", got)
	}
}
