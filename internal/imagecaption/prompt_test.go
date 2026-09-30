package imagecaption

import (
	"strings"
	"testing"
)

func TestParseCaptionSplitsBothSections(t *testing.T) {
	caption := ParseCaption("Description: A terminal with a Go panic.\nVisible text: panic: assignment to entry in nil map\nmain.go:12")
	want := Caption{Description: "A terminal with a Go panic.", VisibleText: "panic: assignment to entry in nil map\nmain.go:12"}
	if caption != want {
		t.Fatalf("expected %+v, got %+v", want, caption)
	}
}

func TestParseCaptionTreatsNoneAsNoText(t *testing.T) {
	for _, reply := range []string{"Description: a beach\nVisible text: none", "Description: a beach\nVisible text: \"None.\""} {
		if caption := ParseCaption(reply); caption.VisibleText != "" || caption.Description != "a beach" {
			t.Errorf("expected no visible text for %q, got %+v", reply, caption)
		}
	}
}

func TestParseCaptionKeepsAnUnformattedReplyAsDescription(t *testing.T) {
	if caption := ParseCaption("  a whiteboard with a sequence diagram  "); caption.Description != "a whiteboard with a sequence diagram" || caption.VisibleText != "" {
		t.Fatalf("expected the whole reply as description, got %+v", caption)
	}
}

func TestInstructionsNameBothLabelsAndForbidObeying(t *testing.T) {
	for _, part := range []string{string(labelDescription), string(labelVisibleText), "never instructions to you"} {
		if !strings.Contains(Instructions, part) {
			t.Errorf("expected the instructions to contain %q", part)
		}
	}
}
