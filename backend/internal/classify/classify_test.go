package classify

import (
	"context"
	"strings"
	"testing"

	"zoa/backend/internal/models"
)

// sanitise is the trust boundary for model output, so it gets direct coverage:
// everything downstream (the DB column, the accuracy metric) assumes it holds.

func TestSanitiseRejectsCategoryOutsideTaxonomy(t *testing.T) {
	_, err := sanitise(&Prediction{Category: "styrofoam_peanuts", Confidence: 0.9})
	if err == nil {
		t.Fatal("expected an error for a category outside the taxonomy")
	}
	if !strings.Contains(err.Error(), "taxonomy") {
		t.Errorf("error = %v, want it to mention the taxonomy", err)
	}
}

func TestSanitiseClampsConfidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   float64
		want float64
	}{
		{"above one", 1.4, 1},
		{"below zero", -0.3, 0},
		{"in range", 0.62, 0.62},
		{"exactly one", 1, 1},
		{"exactly zero", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sanitise(&Prediction{Category: "pet", Confidence: tc.in})
			if err != nil {
				t.Fatalf("sanitise: %v", err)
			}
			if got.Confidence != tc.want {
				t.Errorf("confidence = %v, want %v", got.Confidence, tc.want)
			}
		})
	}
}

// A bad runner-up must not discard a good primary answer.
func TestSanitiseDropsBadAlternativesButKeepsPrimary(t *testing.T) {
	got, err := sanitise(&Prediction{
		Category:   "pet",
		Confidence: 0.8,
		Alternatives: []Alternative{
			{Category: "not_a_material", Confidence: 0.1}, // dropped
			{Category: "pet", Confidence: 0.05},           // dropped: duplicates primary
			{Category: "hdpe", Confidence: 0.04},          // kept
		},
	})
	if err != nil {
		t.Fatalf("sanitise: %v", err)
	}

	if got.Category != "pet" {
		t.Errorf("category = %q, want pet", got.Category)
	}
	if len(got.Alternatives) != 1 || got.Alternatives[0].Category != "hdpe" {
		t.Errorf("alternatives = %+v, want only hdpe", got.Alternatives)
	}
}

func TestSanitiseRejectsNil(t *testing.T) {
	if _, err := sanitise(nil); err == nil {
		t.Fatal("expected an error for a nil prediction")
	}
}

// --- taxonomy helpers ---

func TestRequiresSourceTypeOnlyForOrganics(t *testing.T) {
	for _, m := range models.MaterialTaxonomy {
		want := m.Group == GroupOrganic
		if got := RequiresSourceType(m.Key); got != want {
			t.Errorf("RequiresSourceType(%q) = %v, want %v (group %q)", m.Key, got, want, m.Group)
		}
	}
}

func TestGroupAndLabelCoverWholeTaxonomy(t *testing.T) {
	for _, m := range models.MaterialTaxonomy {
		if got := GroupFor(m.Key); got != m.Group {
			t.Errorf("GroupFor(%q) = %q, want %q", m.Key, got, m.Group)
		}
		if got := LabelFor(m.Key); got != m.Label {
			t.Errorf("LabelFor(%q) = %q, want %q", m.Key, got, m.Label)
		}
	}
	if got := GroupFor("nope"); got != "" {
		t.Errorf("GroupFor(unknown) = %q, want empty", got)
	}
}

// --- the JSON schema sent to the model ---

// The enum is generated from the taxonomy, so adding a material must not leave
// the classifier unable to predict it.
func TestResponseSchemaEnumMatchesTaxonomy(t *testing.T) {
	schema := responseSchema()

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema has no properties object")
	}
	category, ok := properties["category"].(map[string]any)
	if !ok {
		t.Fatal("schema has no category property")
	}
	enum, ok := category["enum"].([]string)
	if !ok {
		t.Fatalf("category enum is %T, want []string", category["enum"])
	}

	if len(enum) != len(models.MaterialTaxonomy) {
		t.Errorf("enum has %d entries, taxonomy has %d", len(enum), len(models.MaterialTaxonomy))
	}
	for i, m := range models.MaterialTaxonomy {
		if enum[i] != m.Key {
			t.Errorf("enum[%d] = %q, want %q", i, enum[i], m.Key)
		}
	}

	// Strict object: an unexpected key would otherwise arrive silently.
	if additional, _ := schema["additionalProperties"].(bool); additional {
		t.Error("additionalProperties should be false")
	}
}

// --- mock classifier ---

func TestMockClassifierKeysOffFilename(t *testing.T) {
	mock := NewMockClassifier()
	image := []byte("pretend this is a jpeg")

	for _, tc := range []struct{ filename, want string }{
		{"pet_bottle_01.jpg", "pet"},
		{"glass_clear_sample.png", "glass_clear"},
		{"food_waste_hotel.jpg", "food_waste"},
		{"garden_waste.jpg", "garden_waste"},
		{"other_plastic_mixed.jpg", "other_plastic"},
		// Aliases, so informally named samples still land sensibly.
		{"cans.jpg", "aluminum"},
		{"newspaper_stack.jpg", "mixed_paper"},
		{"IMG_kitchen_scraps.jpg", "food_waste"},
	} {
		t.Run(tc.filename, func(t *testing.T) {
			got, err := mock.Classify(context.Background(), Request{
				Image: image, MediaType: "image/jpeg", Filename: tc.filename,
			})
			if err != nil {
				t.Fatalf("Classify: %v", err)
			}
			if got.Category != tc.want {
				t.Errorf("category = %q, want %q", got.Category, tc.want)
			}
		})
	}
}

// "glass_clear" must not be shadowed by a shorter key that is a substring of it.
// Regression: a raw substring search matches the key "ps" inside "scraps" and
// predicts polystyrene for a photo of food waste. Short resin keys make this
// easy to reintroduce, so the boundary behaviour is pinned explicitly.
func TestMockClassifierDoesNotMatchKeysInsideWords(t *testing.T) {
	mock := NewMockClassifier()

	for _, tc := range []struct{ filename, notWant string }{
		{"IMG_kitchen_scraps.jpg", "ps"}, // "scraps" ⊃ "ps"
		{"wrapper_pile.jpg", "pp"},       // "wrapper" ⊃ "pp"
		{"shopping_pile.jpg", "pp"},      // "shopping" ⊃ "pp"
	} {
		t.Run(tc.filename, func(t *testing.T) {
			got, err := mock.Classify(context.Background(), Request{
				Image: []byte("x"), MediaType: "image/jpeg", Filename: tc.filename,
			})
			if err != nil {
				t.Fatalf("Classify: %v", err)
			}
			if got.Category == tc.notWant {
				t.Errorf("filename %q matched key %q inside a word", tc.filename, tc.notWant)
			}
		})
	}
}

// "glass_clear" must not be shadowed by a shorter key that is a substring of it.
func TestMockClassifierPrefersLongestKeyMatch(t *testing.T) {
	mock := NewMockClassifier()

	got, err := mock.Classify(context.Background(), Request{
		Image: []byte("x"), MediaType: "image/jpeg", Filename: "glass_colored_bottle.jpg",
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got.Category != "glass_colored" {
		t.Errorf("category = %q, want glass_colored", got.Category)
	}
}

func TestMockClassifierIsDeterministicWithoutFilenameHint(t *testing.T) {
	mock := NewMockClassifier()
	image := []byte("a specific set of bytes that hashes one way")

	first, err := mock.Classify(context.Background(), Request{
		Image: image, MediaType: "image/jpeg", Filename: "IMG_0001.jpg",
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	second, err := mock.Classify(context.Background(), Request{
		Image: image, MediaType: "image/jpeg", Filename: "IMG_0001.jpg",
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}

	if first.Category != second.Category || first.Confidence != second.Confidence {
		t.Errorf("not deterministic: %+v then %+v", first, second)
	}
}

func TestMockClassifierAlwaysPredictsValidCategory(t *testing.T) {
	mock := NewMockClassifier()

	// Walk many distinct inputs through the hash branch.
	for i := 0; i < 200; i++ {
		image := []byte(strings.Repeat("z", i+1))
		got, err := mock.Classify(context.Background(), Request{
			Image: image, MediaType: "image/jpeg", Filename: "unhinted.jpg",
		})
		if err != nil {
			t.Fatalf("Classify(%d): %v", i, err)
		}
		if !models.IsValidMaterialType(got.Category) {
			t.Fatalf("input %d predicted %q, which is not in the taxonomy", i, got.Category)
		}
		if got.Confidence <= 0 || got.Confidence > 1 {
			t.Fatalf("input %d confidence = %v, want (0,1]", i, got.Confidence)
		}
	}
}

func TestMockClassifierRejectsEmptyImage(t *testing.T) {
	mock := NewMockClassifier()

	if _, err := mock.Classify(context.Background(), Request{MediaType: "image/jpeg"}); err == nil {
		t.Fatal("expected an error for an empty image")
	}
}

func TestMockAlternativesShareTheGroupAndExcludePrimary(t *testing.T) {
	mock := NewMockClassifier()

	got, err := mock.Classify(context.Background(), Request{
		Image: []byte("x"), MediaType: "image/jpeg", Filename: "pet_bottle.jpg",
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}

	for _, alt := range got.Alternatives {
		if alt.Category == got.Category {
			t.Errorf("alternative repeats the primary category %q", got.Category)
		}
		// Same-group runner-ups are the ones a human would actually confuse.
		if GroupFor(alt.Category) != GroupFor(got.Category) {
			t.Errorf("alternative %q is in group %q, want %q",
				alt.Category, GroupFor(alt.Category), GroupFor(got.Category))
		}
	}
}

func TestIsValidSourceType(t *testing.T) {
	for _, valid := range []string{SourceResidential, SourceHotel} {
		if !IsValidSourceType(valid) {
			t.Errorf("IsValidSourceType(%q) = false, want true", valid)
		}
	}
	for _, invalid := range []string{"", "Hotel", "office", "spaceship"} {
		if IsValidSourceType(invalid) {
			t.Errorf("IsValidSourceType(%q) = true, want false", invalid)
		}
	}
}
