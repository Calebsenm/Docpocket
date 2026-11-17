package collections

import (
	"strings"
	"unicode"
)

// stopwords are ignored by the full-text matcher. The list covers the most
// common Spanish and English particles.
var stopwords = map[string]bool{
	"a": true, "al": true, "algo": true, "algunas": true, "algunos": true, "ante": true,
	"antes": true, "con": true, "contra": true, "cual": true, "cuando": true, "de": true,
	"del": true, "desde": true, "donde": true, "durante": true, "e": true, "el": true,
	"ella": true, "ellas": true, "ellos": true, "en": true, "entre": true, "era": true,
	"es": true, "esa": true, "esas": true, "ese": true, "eso": true, "esos": true,
	"esta": true, "está": true, "estas": true, "este": true, "esto": true, "estos": true,
	"hay": true, "la": true, "las": true, "le": true, "les": true, "lo": true, "los": true,
	"más": true, "me": true, "mi": true, "mis": true, "mucho": true, "muy": true, "ni": true,
	"no": true, "o": true, "para": true, "pero": true, "por": true, "que": true, "qué": true,
	"se": true, "según": true, "ser": true, "si": true, "sin": true, "son": true, "su": true,
	"sus": true, "te": true, "tu": true, "tus": true, "un": true, "una": true, "unas": true,
	"uno": true, "unos": true, "y": true, "ya": true, "yo": true,
	"and": true, "the": true, "of": true, "to": true, "in": true, "is": true, "it": true,
	"for": true, "on": true, "with": true, "as": true, "at": true, "by": true, "an": true,
	"be": true, "or": true, "this": true, "that": true, "are": true, "was": true, "from": true,
	"not": true, "but": true, "have": true, "has": true, "had": true,
}

// Tokenize splits text into lowercased alphanumeric tokens, dropping
// stopwords and single characters. It is the building block of full-text
// matching and of the full-text index.
func Tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) < 2 || stopwords[f] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// TextMatch reports whether query text matches document text. Matching is
// token based: any query token equal to a document token, or a prefix of one
// (for tokens of 4+ characters), counts as a hit. Stopwords are ignored.
func TextMatch(doc, query string) bool {
	q := Tokenize(query)
	if len(q) == 0 {
		return false
	}
	docTokens := Tokenize(doc)
	if len(docTokens) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(docTokens))
	for _, t := range docTokens {
		set[t] = struct{}{}
	}
	for _, t := range q {
		if _, ok := set[t]; ok {
			return true
		}
	}
	for _, t := range q {
		if len(t) < 4 {
			continue
		}
		for dt := range set {
			if strings.HasPrefix(dt, t) {
				return true
			}
		}
	}
	return false
}
