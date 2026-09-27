package cli

import "strings"

// Language selects the language of the CLI's help, labels and messages.
// Answers to `ask` follow the question's language instead.
type Language int

const (
	Portuguese Language = iota
	English
)

// localeVariables are read in gettext's order of precedence.
var localeVariables = []string{"LC_ALL", "LC_MESSAGES", "LANG"}

// LanguageFromEnv follows the locale: Portuguese for "pt…" (pt_BR.UTF-8,
// pt_PT…), English otherwise, including an unset or "C" locale.
//
//	language := cli.LanguageFromEnv(os.Getenv)
func LanguageFromEnv(getenv func(string) string) Language {
	for _, variable := range localeVariables {
		value := getenv(variable)
		if value == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(value), "pt") {
			return Portuguese
		}
		return English
	}
	return English
}

// pick returns the text written in l.
func (l Language) pick(portuguese, english string) string {
	if l == English {
		return english
	}
	return portuguese
}

// languageFromSetting applies ui.language: "pt" or "en" override the
// locale, anything else ("auto") keeps it.
func languageFromSetting(setting string, fromLocale Language) Language {
	switch setting {
	case "pt":
		return Portuguese
	case "en":
		return English
	}
	return fromLocale
}
