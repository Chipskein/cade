package sitestorage

import (
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/v8value"
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
