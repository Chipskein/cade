package htmltext

import "testing"

func TestToText(t *testing.T) {
	cases := map[string]string{
		"<p>Olá <b>time</b></p>":                   "Olá time",
		"linha 1<br>linha 2<br/>":                  "linha 1\nlinha 2",
		"<p>a</p><p></p><p>b</p>":                  "a\nb",
		"R&amp;D &lt;3 &quot;ok&quot;&nbsp;fim":    `R&D <3 "ok" fim`,
		"<style>.x{}</style><div>visível</div>":    "visível",
		"<span itemtype=\"x\">@Bruno</span>, veja": "@Bruno, veja",
	}
	for input, expected := range cases {
		if got := ToText(input); got != expected {
			t.Errorf("ToText(%q) = %q, expected %q", input, got, expected)
		}
	}
}

// Regression: quote authors and mentions were glued to the following text
// ("Marcos Lima - Atlasmas é estranho").
func TestToTextSeparatesQuotesAndMentions(t *testing.T) {
	body := `<blockquote itemscope="" itemtype="http://schema.skype.com/Reply"><strong itemprop="mri" itemid="8:orgid:x">Marcos Lima - Atlas</strong><span itemprop="time"></span><p itemprop="preview">mas é estranho</p></blockquote>` +
		`<p><span itemtype="http://schema.skype.com/Mention" itemscope="" itemid="0">Vitor Alves</span>, veja isso</p>`
	expected := "@Vitor Alves, veja isso\n↪ em resposta a Marcos Lima - Atlas: mas é estranho"
	if got := ToText(body); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestExtractQuotesWithoutQuote(t *testing.T) {
	remaining, quotes := extractQuotes("<p>oi</p>")
	if remaining != "<p>oi</p>" || quotes != nil {
		t.Fatalf("expected no quotes, got %q / %v", remaining, quotes)
	}
}
