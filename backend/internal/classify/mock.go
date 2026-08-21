package classify

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"path/filepath"
	"strings"

	"zoa/backend/internal/models"
)

// MockClassifier predicts without calling any external service.
//
// The implementation plan calls for exactly this as the last-resort fallback:
// if the vision integration is too slow or unavailable on the day, a visibly
// working (if simplified) AI step protects the demo, where cutting the feature
// outright does not. It is also what the handler tests run against, so the test
// suite needs no network and no API key.
//
// Two strategies, in order:
//
//  1. Filename hint — "pet_bottle_01.jpg" predicts "pet". This is what makes a
//     rehearsed walkthrough reproducible: name the sample photos after their
//     categories and every run predicts correctly.
//  2. Content hash — any other photo maps deterministically onto the taxonomy.
//     The same image always yields the same answer, so a demo never surprises
//     you with a different result on the second take.
type MockClassifier struct{}

// NewMockClassifier builds a mock classifier.
func NewMockClassifier() *MockClassifier { return &MockClassifier{} }

// Name identifies this classifier.
func (m *MockClassifier) Name() string { return "mock" }

// Classify returns a deterministic prediction.
func (m *MockClassifier) Classify(_ context.Context, req Request) (*Prediction, error) {
	if len(req.Image) == 0 {
		return nil, ErrUnavailable
	}

	if key, ok := categoryFromFilename(req.Filename); ok {
		// A deliberately named sample is treated as a confident hit — the point of
		// the fallback is a demo that looks like the real thing.
		return sanitise(&Prediction{
			Category:     key,
			Confidence:   0.93,
			Alternatives: neighbours(key),
		})
	}

	// Deterministic pick: the first 8 bytes of the digest select a category, so
	// the answer is stable per image but spread across the taxonomy across images.
	sum := sha256.Sum256(req.Image)
	idx := binary.BigEndian.Uint64(sum[:8]) % uint64(len(models.MaterialTaxonomy))
	key := models.MaterialTaxonomy[idx].Key

	// Confidence in a plausible mid band. Never 0.9+: an unhinted photo is a
	// guess, and a mock that always claims near-certainty would misrepresent what
	// the real classifier does on a hard image.
	confidence := 0.55 + float64(sum[8]%30)/100

	return sanitise(&Prediction{
		Category:     key,
		Confidence:   confidence,
		Alternatives: neighbours(key),
	})
}

// categoryFromFilename looks for a taxonomy key in the filename.
//
// Matching is on token boundaries, not raw substrings. A substring search looks
// fine until "IMG_kitchen_scraps.jpg" matches the key "ps" inside "scraps" and
// predicts polystyrene foam for a photo of food waste. Short resin keys ("ps",
// "pp") make that failure easy to hit and hard to spot, so the filename is split
// into tokens and keys must match whole ones.
func categoryFromFilename(name string) (string, bool) {
	if name == "" {
		return "", false
	}

	base := strings.ToLower(filepath.Base(name))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	tokens := tokenise(base)
	if len(tokens) == 0 {
		return "", false
	}

	// Multi-part keys first, by descending specificity: "glass_clear" must win
	// over a hypothetical bare "glass", and "other_plastic" over "plastic".
	best := ""
	bestParts := 0
	for _, m := range models.MaterialTaxonomy {
		parts := strings.Split(m.Key, "_")
		if !containsSequence(tokens, parts) {
			continue
		}
		if len(parts) > bestParts || (len(parts) == bestParts && len(m.Key) > len(best)) {
			best, bestParts = m.Key, len(parts)
		}
	}
	if best != "" {
		return best, true
	}

	// Informal words, so "cans.jpg" or "plastic_bottle.jpg" still land somewhere
	// sensible. Matched inside a token so simple plurals work ("cans" → "can"),
	// but never across tokens.
	for _, alias := range filenameAliases {
		for _, token := range tokens {
			if strings.Contains(token, alias.hint) {
				return alias.key, true
			}
		}
	}
	return "", false
}

// tokenise splits a filename into alphanumeric runs, so "IMG_4021-pet.v2" yields
// [img 4021 pet v2].
func tokenise(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		isDigit := r >= '0' && r <= '9'
		isLower := r >= 'a' && r <= 'z'
		return !isDigit && !isLower
	})
}

// containsSequence reports whether want appears as a contiguous run in tokens.
func containsSequence(tokens, want []string) bool {
	if len(want) == 0 || len(want) > len(tokens) {
		return false
	}
	for i := 0; i+len(want) <= len(tokens); i++ {
		match := true
		for j, w := range want {
			if tokens[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// filenameAliases maps informal words onto taxonomy keys. Ordered: the first
// match wins, so put more specific hints before broader ones.
var filenameAliases = []struct {
	hint string
	key  string
}{
	{"bottle", "pet"},
	{"jerrican", "hdpe"},
	{"jerry", "hdpe"},
	{"milk", "hdpe"},
	{"bag", "ldpe"},
	{"film", "ldpe"},
	{"wrap", "ldpe"},
	{"cap", "pp"},
	{"tub", "pp"},
	{"bucket", "pp"},
	{"foam", "ps"},
	{"box", "cardboard"},
	{"carton", "cardboard"},
	{"corrugated", "cardboard"},
	{"newspaper", "mixed_paper"},
	{"paper", "mixed_paper"},
	{"glass", "glass_clear"},
	{"jar", "glass_clear"},
	{"can", "aluminum"},
	{"aluminium", "aluminum"},
	{"tin", "steel_tin"},
	{"steel", "steel_tin"},
	{"food", "food_waste"},
	{"kitchen", "food_waste"},
	{"garden", "garden_waste"},
	{"leaves", "garden_waste"},
	{"grass", "garden_waste"},
	{"plastic", "other_plastic"},
	{"metal", "steel_tin"},
	{"organic", "food_waste"},
}

// neighbours returns up to two same-group runner-ups, so the UI's one-tap
// correction offers the categories a human would actually confuse with this one.
func neighbours(key string) []Alternative {
	group := GroupFor(key)
	if group == "" {
		return nil
	}

	var out []Alternative
	for _, m := range models.MaterialTaxonomy {
		if m.Key == key || m.Group != group {
			continue
		}
		out = append(out, Alternative{Category: m.Key, Confidence: 0.04})
		if len(out) == 2 {
			break
		}
	}
	return out
}
