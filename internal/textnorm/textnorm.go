// Package textnorm normalizes Portuguese text for keyword matching, so
// "Última", "ultima" and "ÚLTIMA" compare equal.
package textnorm

import "strings"

var accentFolder = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e",
	"í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ç", "c")

// Fold lowercases text and strips Portuguese accents.
//
//	textnorm.Fold("Última Ação") == "ultima acao"
func Fold(text string) string {
	return accentFolder.Replace(strings.ToLower(text))
}
