package validate

import (
	"unicode/utf8"

	"github.com/pemistahl/lingua-go"
)

// LanguageDetector wraps lingua-go to detect the language of generated text.
type LanguageDetector struct {
	detector lingua.LanguageDetector
}

// langMap maps lingua.Language constants to our language codes.
var langMap = map[lingua.Language]string{
	lingua.English: "en",
	lingua.German:  "de",
	lingua.Spanish: "es",
	lingua.French:  "fr",
	lingua.Italian: "it",
}

// codeLangMap maps our language codes to lingua.Language constants.
var codeLangMap = map[string]lingua.Language{
	"en": lingua.English,
	"de": lingua.German,
	"es": lingua.Spanish,
	"fr": lingua.French,
	"it": lingua.Italian,
}

// NewLanguageDetector builds a detector for the 5 target languages with
// a minimum relative distance of 0.25 for accuracy.
func NewLanguageDetector() *LanguageDetector {
	detector := lingua.NewLanguageDetectorBuilder().
		FromLanguages(lingua.English, lingua.German, lingua.Spanish, lingua.French, lingua.Italian).
		WithMinimumRelativeDistance(0.25).
		Build()

	return &LanguageDetector{detector: detector}
}

// Check detects the language of text and returns whether it matches the expected language code.
// Text shorter than 20 runes is skipped (too short for reliable detection, returns matches=true).
func (ld *LanguageDetector) Check(text, expectedLang string) (detected string, matches bool) {
	if utf8.RuneCountInString(text) < 20 {
		return expectedLang, true
	}

	detectedLang, ok := ld.detector.DetectLanguageOf(text)
	if !ok {
		// Could not confidently detect; assume it matches to avoid false positives.
		return "unknown", true
	}

	detectedCode, known := langMap[detectedLang]
	if !known {
		return "unknown", true
	}

	return detectedCode, detectedCode == expectedLang
}
