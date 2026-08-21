package classify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"zoa/backend/internal/models"
)

// ClaudeClassifier classifies waste photos with Claude's vision capability.
type ClaudeClassifier struct {
	client anthropic.Client
	model  string
}

// NewClaudeClassifier builds a Claude-backed classifier.
//
// An empty apiKey is not an error here — the SDK also resolves credentials from
// the environment and from an `ant auth login` profile, so the caller cannot
// know from the key alone whether auth will work. Misconfiguration surfaces as a
// degraded response on the first call rather than as a boot failure, because a
// backend that refuses to start over a missing vision key would take the whole
// submission flow down with it.
func NewClaudeClassifier(apiKey, model string) *ClaudeClassifier {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if model == "" {
		model = DefaultModel
	}
	return &ClaudeClassifier{client: anthropic.NewClient(opts...), model: model}
}

// DefaultModel is the vision model used when none is configured.
const DefaultModel = "claude-opus-5"

// Name identifies this classifier.
func (c *ClaudeClassifier) Name() string { return "claude:" + c.model }

// maxTokens is small on purpose: the reply is a short JSON object, and a low
// ceiling is one less thing that can turn a 2-second call into a 10-second one.
const maxTokens = 512

// systemPrompt frames the task. Kept terse — the schema does the structural
// work, so prose here is spent only on the judgement calls the schema cannot
// express.
const systemPrompt = `You identify recyclable material from a photograph for a Kenyan recycling programme.

Return the single taxonomy category that best matches the dominant recyclable item in the photo.

Guidance:
- Judge the material, not the contents: an empty PET water bottle is "pet" whether or not it held water.
- Plastics are distinguished by resin. Clear drink bottles are "pet"; opaque jerricans, milk and detergent bottles are "hdpe"; thin bags, sacks and film are "ldpe"; bottle caps, tubs and buckets are "pp"; foam and brittle takeaway boxes are "ps". Use "other_plastic" only when the resin genuinely cannot be told apart.
- "food_waste" is kitchen and plate waste; "garden_waste" is leaves, prunings and grass.
- Corrugated boxes are "cardboard"; printed paper, newspaper and office paper are "mixed_paper".
- Set confidence honestly. A blurred, dark, or mixed-material photo should score low — a collector re-weighs and re-checks every submission, so an uncertain guess costs nothing, while a confident wrong one wastes their correction.`

// Classify sends the image to Claude and returns a sanitised prediction.
func (c *ClaudeClassifier) Classify(ctx context.Context, req Request) (*Prediction, error) {
	if len(req.Image) == 0 {
		return nil, fmt.Errorf("empty image")
	}

	encoded := base64.StdEncoding.EncodeToString(req.Image)

	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: maxTokens,
		System: []anthropic.TextBlockParam{{
			Text: systemPrompt,
		}},
		// Effort low, not because the task is unimportant, but because the whole
		// call has a ~3s budget (TRD §3) and deep deliberation is what would blow
		// it. Identifying a bottle is not a reasoning-heavy problem.
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffortLow,
			Format: anthropic.JSONOutputFormatParam{Schema: responseSchema()},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewImageBlockBase64(req.MediaType, encoded),
				anthropic.NewTextBlock("Identify the recyclable material in this photo."),
			),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("classification request: %w", err)
	}

	// A safety decline is a legitimate outcome, not a bug: it means this photo
	// will not be classified. Reported as an error so the caller degrades to
	// manual selection, which is exactly the required behaviour (FR-23).
	if resp.StopReason == anthropic.StopReasonRefusal {
		return nil, fmt.Errorf("classification declined (%s)", resp.StopDetails.Category)
	}

	raw := firstText(resp)
	if raw == "" {
		return nil, fmt.Errorf("classification returned no text")
	}

	var parsed schemaResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parse classification: %w", err)
	}

	prediction := &Prediction{
		Category:   parsed.Category,
		Confidence: parsed.Confidence,
	}
	for _, alt := range parsed.Alternatives {
		prediction.Alternatives = append(prediction.Alternatives, Alternative{
			Category:   alt.Category,
			Confidence: alt.Confidence,
		})
	}

	return sanitise(prediction)
}

// firstText returns the first text block's content.
func firstText(resp *anthropic.Message) string {
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			if s := strings.TrimSpace(t.Text); s != "" {
				return s
			}
		}
	}
	return ""
}

// schemaResponse mirrors responseSchema.
type schemaResponse struct {
	Category     string  `json:"category"`
	Confidence   float64 `json:"confidence"`
	Alternatives []struct {
		Category   string  `json:"category"`
		Confidence float64 `json:"confidence"`
	} `json:"alternatives"`
}

// responseSchema constrains the reply to a taxonomy key plus confidences.
//
// The category enum is generated from models.MaterialTaxonomy rather than
// written out, so adding a material in one place cannot leave the classifier
// unable to predict it.
func responseSchema() map[string]any {
	keys := taxonomyKeys()

	alternative := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category":   map[string]any{"type": "string", "enum": keys},
			"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
		},
		"required":             []string{"category", "confidence"},
		"additionalProperties": false,
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category": map[string]any{
				"type":        "string",
				"enum":        keys,
				"description": "The single best-matching material taxonomy key.",
			},
			"confidence": map[string]any{
				"type":        "number",
				"minimum":     0,
				"maximum":     1,
				"description": "Honest confidence in the chosen category.",
			},
			"alternatives": map[string]any{
				"type":        "array",
				"items":       alternative,
				"maxItems":    3,
				"description": "Runner-up categories, most confident first. May be empty.",
			},
		},
		"required":             []string{"category", "confidence", "alternatives"},
		"additionalProperties": false,
	}
}

// compile-time assurance that the taxonomy is non-empty, since an empty enum
// would make the schema unsatisfiable and every call would fail at runtime.
var _ = func() struct{} {
	if len(models.MaterialTaxonomy) == 0 {
		panic("classify: material taxonomy is empty")
	}
	return struct{}{}
}()
