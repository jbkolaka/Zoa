// Package classify turns a photo of waste into a material-taxonomy prediction.
//
// The contract that shapes this package is FR-23: classification is an assist,
// never a blocker. Every failure mode — no API key, a network stall, a model
// refusal, an unparseable answer, a category outside the taxonomy — resolves to
// "no prediction" rather than an error the submission flow has to recover from.
// Callers therefore treat a nil prediction as normal, not exceptional.
package classify

import (
	"context"
	"errors"
	"fmt"
	"time"

	"zoa/backend/internal/models"
)

// ErrUnavailable means no classifier is configured. Handlers translate this to a
// degraded response, not a 5xx — a backend with no vision credentials still
// serves the submission flow perfectly well with manual selection.
var ErrUnavailable = errors.New("classification is not configured")

// Prediction is one classification result.
type Prediction struct {
	// Category is a key from models.MaterialTaxonomy — guaranteed valid, because
	// anything else is rejected before a Prediction is constructed.
	Category string

	// Confidence is the model's own estimate in [0,1].
	Confidence float64

	// Alternatives are runner-up guesses, most confident first. Advisory: the UI
	// offers them as one-tap corrections so a near-miss costs a tap, not a scroll
	// through fourteen categories.
	Alternatives []Alternative
}

// Alternative is a runner-up category.
type Alternative struct {
	Category   string
	Confidence float64
}

// Request is one classification input.
type Request struct {
	// Image is the raw photo bytes.
	Image []byte

	// MediaType is the image's MIME type, e.g. "image/jpeg".
	MediaType string

	// Filename is the uploaded file's name, if any. Advisory only: the Claude
	// classifier ignores it (a filename is user-controlled and says nothing
	// reliable about pixels), while the mock classifier keys off it so a
	// rehearsed demo is reproducible — the fallback the implementation plan
	// calls for if the vision path proves too slow on the day.
	Filename string
}

// Classifier predicts a material category from an image.
//
// Implementations must respect ctx's deadline: the caller sets it from the
// TRD §3 latency budget and relies on it being honoured rather than on the
// implementation's own idea of a reasonable wait.
type Classifier interface {
	Classify(ctx context.Context, req Request) (*Prediction, error)

	// Name identifies the implementation in logs and in the /health payload, so
	// "why is it degraded" is answerable without reading the config.
	Name() string
}

// MaxImageBytes caps an upload at 8 MB, matching docs/API_CONTRACT.md. A modern
// phone photo is 2–5 MB, so this accepts real camera output while refusing
// something that is not a snapshot.
const MaxImageBytes = 8 << 20

// AllowedMediaTypes are the image formats the endpoint accepts.
var AllowedMediaTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
}

// taxonomyKeys returns every valid category key, in taxonomy order.
func taxonomyKeys() []string {
	keys := make([]string, 0, len(models.MaterialTaxonomy))
	for _, m := range models.MaterialTaxonomy {
		keys = append(keys, m.Key)
	}
	return keys
}

// clampConfidence forces a confidence into [0,1].
//
// A model is free to return 1.4 or -0.2 and occasionally does; letting that
// reach the client would put "140% sure" on screen, so the value is clamped at
// the boundary rather than trusted or rejected.
func clampConfidence(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}

// sanitise drops anything outside the taxonomy and clamps confidences.
//
// This is the trust boundary for model output: a hallucinated "styrofoam" would
// otherwise be written to submissions.predicted_category and pollute the
// accuracy metric that is the entire point of storing it (FR-22).
func sanitise(p *Prediction) (*Prediction, error) {
	if p == nil {
		return nil, fmt.Errorf("nil prediction")
	}
	if !models.IsValidMaterialType(p.Category) {
		return nil, fmt.Errorf("predicted category %q is not in the taxonomy", p.Category)
	}

	out := &Prediction{
		Category:   p.Category,
		Confidence: clampConfidence(p.Confidence),
	}

	for _, alt := range p.Alternatives {
		// Silently skipped rather than failing the whole prediction: a bad
		// runner-up should not discard a good primary answer.
		if !models.IsValidMaterialType(alt.Category) || alt.Category == out.Category {
			continue
		}
		out.Alternatives = append(out.Alternatives, Alternative{
			Category:   alt.Category,
			Confidence: clampConfidence(alt.Confidence),
		})
	}
	return out, nil
}

// GroupFor returns the taxonomy group for a category key, or "" if unknown.
func GroupFor(key string) string {
	for _, m := range models.MaterialTaxonomy {
		if m.Key == key {
			return m.Group
		}
	}
	return ""
}

// LabelFor returns the human-readable label for a category key, or "".
func LabelFor(key string) string {
	for _, m := range models.MaterialTaxonomy {
		if m.Key == key {
			return m.Label
		}
	}
	return ""
}

// GroupOrganic is the taxonomy group that requires a source type (FR-24).
const GroupOrganic = "organic"

// RequiresSourceType reports whether a category obliges the client to ask
// hotel-vs-residential before submitting.
func RequiresSourceType(key string) bool {
	return GroupFor(key) == GroupOrganic
}

// Source types (FR-4a).
const (
	SourceResidential = "residential"
	SourceHotel       = "hotel"
)

// IsValidSourceType reports whether v is an accepted source type.
func IsValidSourceType(v string) bool {
	return v == SourceResidential || v == SourceHotel
}

// DefaultTimeout is the classification budget from TRD §3 ("<3s under demo
// conditions"). Configurable, because the right number depends on the venue's
// uplink as much as on the model.
const DefaultTimeout = 3 * time.Second
