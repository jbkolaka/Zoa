package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/classify"
	"zoa/backend/internal/httpx"
)

// ClassifyHandler serves POST /submissions/classify (Phase 2.5).
//
// The endpoint's defining property is that it does not fail. Every internal
// problem — no classifier configured, a timeout, a refusal, a garbled answer —
// returns 200 with `degraded: true`, because the client's only sane reaction to
// any of them is identical: fall through to manual material selection (FR-23).
// Returning 4xx/5xx would force the Flutter client to special-case error
// recovery inside a flow that is explicitly allowed to proceed without AI.
type ClassifyHandler struct {
	classifier classify.Classifier
	timeout    time.Duration
}

// NewClassifyHandler builds the classification handler. A nil classifier is
// valid and yields permanently degraded responses.
func NewClassifyHandler(classifier classify.Classifier, timeout time.Duration) *ClassifyHandler {
	if timeout <= 0 {
		timeout = classify.DefaultTimeout
	}
	return &ClassifyHandler{classifier: classifier, timeout: timeout}
}

// classifyResponse is the shape in docs/API_CONTRACT.md § Phase 2.5.
type classifyResponse struct {
	PredictedCategory   string                `json:"predicted_category"`
	PredictedConfidence float64               `json:"predicted_confidence"`
	Label               string                `json:"label"`
	Group               string                `json:"group"`
	RequiresSourceType  bool                  `json:"requires_source_type"`
	Alternatives        []classifyAlternative `json:"alternatives"`
	LatencyMs           int64                 `json:"latency_ms"`
	Degraded            bool                  `json:"degraded"`

	// Reason explains a degraded response. Present only when Degraded is true:
	// without it, "the AI did nothing" is indistinguishable from a missing API
	// key at demo time, which is the one moment you cannot afford to guess.
	Reason string `json:"reason,omitempty"`
}

type classifyAlternative struct {
	PredictedCategory   string  `json:"predicted_category"`
	PredictedConfidence float64 `json:"predicted_confidence"`
}

// Classify handles POST /submissions/classify.
func (h *ClassifyHandler) Classify(c *gin.Context) {
	started := time.Now()

	// A body over the cap is refused before it is read into memory, so an
	// oversized upload cannot be used to exhaust the process.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, classify.MaxImageBytes+1024)

	file, header, err := c.Request.FormFile("photo")
	if err != nil {
		// A malformed request *is* the client's fault and is worth a 400: unlike a
		// classifier failure, retrying the same upload will not help.
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"attach a photo to classify",
			map[string]string{"photo": "a photo file is required"})
		return
	}
	defer file.Close()

	if header.Size > classify.MaxImageBytes {
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"that photo is too large",
			map[string]string{"photo": "photos must be 8 MB or smaller"})
		return
	}

	image := make([]byte, 0, header.Size)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			image = append(image, buf[:n]...)
		}
		if readErr != nil {
			break
		}
		if len(image) > classify.MaxImageBytes {
			httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
				"that photo is too large",
				map[string]string{"photo": "photos must be 8 MB or smaller"})
			return
		}
	}

	if len(image) == 0 {
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"that photo appears to be empty",
			map[string]string{"photo": "the uploaded file was empty"})
		return
	}

	// Sniffed from content, not from the declared Content-Type: a phone that
	// mislabels its own upload should not turn into a confusing model error.
	mediaType := http.DetectContentType(image)
	if !classify.AllowedMediaTypes[mediaType] {
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"that file is not a JPEG or PNG photo",
			map[string]string{"photo": "use a JPEG or PNG image"})
		return
	}

	if h.classifier == nil {
		h.degrade(c, started, "classification is not configured")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	prediction, err := h.classifier.Classify(ctx, classify.Request{
		Image:     image,
		MediaType: mediaType,
		Filename:  header.Filename,
	})
	if err != nil {
		// Logged, not surfaced: the operator needs the detail, the client needs
		// only to know it should ask the user to pick a material.
		log.Printf("classify: %s failed after %s: %v",
			h.classifier.Name(), time.Since(started).Round(time.Millisecond), err)
		h.degrade(c, started, degradeReason(ctx, err))
		return
	}

	alternatives := make([]classifyAlternative, 0, len(prediction.Alternatives))
	for _, alt := range prediction.Alternatives {
		alternatives = append(alternatives, classifyAlternative{
			PredictedCategory:   alt.Category,
			PredictedConfidence: alt.Confidence,
		})
	}

	c.JSON(http.StatusOK, classifyResponse{
		PredictedCategory:   prediction.Category,
		PredictedConfidence: prediction.Confidence,
		Label:               classify.LabelFor(prediction.Category),
		Group:               classify.GroupFor(prediction.Category),
		RequiresSourceType:  classify.RequiresSourceType(prediction.Category),
		Alternatives:        alternatives,
		LatencyMs:           time.Since(started).Milliseconds(),
		Degraded:            false,
	})
}

// degrade writes the "carry on manually" response. Always 200 (FR-23).
func (h *ClassifyHandler) degrade(c *gin.Context, started time.Time, reason string) {
	c.JSON(http.StatusOK, classifyResponse{
		Alternatives: []classifyAlternative{},
		LatencyMs:    time.Since(started).Milliseconds(),
		Degraded:     true,
		Reason:       reason,
	})
}

// degradeReason turns an internal failure into a short operator-facing reason.
func degradeReason(ctx context.Context, err error) string {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "classification exceeded its time budget"
	case errors.Is(ctx.Err(), context.Canceled):
		return "the request was cancelled before classification finished"
	case errors.Is(err, classify.ErrUnavailable):
		return "classification is not configured"
	default:
		return fmt.Sprintf("classification failed: %s", err)
	}
}
