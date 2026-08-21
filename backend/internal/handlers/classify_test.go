package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"zoa/backend/internal/models"
)

// --- helpers ---

// jpegBytes builds a real, decodable JPEG. The handler sniffs the media type
// from content rather than trusting the multipart header, so a test fixture has
// to be a genuine image, not arbitrary bytes.
func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 120, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// postPhoto uploads content as multipart field `photo`.
func postPhoto(t *testing.T, f *submissionFixture, filename string, content []byte, token string) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if filename != "" {
		part, err := writer.CreateFormFile("photo", filename)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("write photo: %v", err)
		}
	} else {
		// A well-formed multipart body with no `photo` field at all.
		if err := writer.WriteField("note", "no photo here"); err != nil {
			t.Fatalf("write field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/submissions/classify", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	return recorder
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
	}
	return body
}

// --- POST /submissions/classify ---

// The test fixture runs with the mock classifier (no ANTHROPIC_API_KEY is set in
// tests), which is deliberate: these tests pin the endpoint's contract, not the
// model's judgement.

func TestClassifyRequiresAuth(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := postPhoto(t, f, "pet_bottle.jpg", jpegBytes(t, 8, 8), "")
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", recorder.Code)
	}
}

func TestClassifyReturnsPredictionForNamedSample(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := postPhoto(t, f, "pet_bottle_01.jpg", jpegBytes(t, 16, 16), f.userToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", recorder.Code, recorder.Body)
	}

	body := decodeBody(t, recorder)

	if got := body["predicted_category"]; got != "pet" {
		t.Errorf("predicted_category = %v, want pet", got)
	}
	if got, _ := body["degraded"].(bool); got {
		t.Errorf("degraded = true, want false (reason %v)", body["reason"])
	}
	if got := body["label"]; got != "PET bottles" {
		t.Errorf("label = %v, want \"PET bottles\"", got)
	}
	if got := body["group"]; got != "plastics" {
		t.Errorf("group = %v, want plastics", got)
	}
	if got, _ := body["requires_source_type"].(bool); got {
		t.Errorf("requires_source_type = true, want false for plastics")
	}

	confidence, ok := body["predicted_confidence"].(float64)
	if !ok || confidence <= 0 || confidence > 1 {
		t.Errorf("predicted_confidence = %v, want a value in (0,1]", body["predicted_confidence"])
	}

	// latency_ms is present even on the fast path — the client shows it, and a
	// missing key would read as 0ms rather than as "not measured".
	if _, ok := body["latency_ms"].(float64); !ok {
		t.Errorf("latency_ms missing or not a number: %v", body["latency_ms"])
	}
}

// FR-24: organics oblige the client to ask hotel-vs-residential, and the flag
// that tells it to do so must be set by the classifier response.
func TestClassifyFlagsOrganicSourceTypeRequirement(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := postPhoto(t, f, "food_waste_kitchen.jpg", jpegBytes(t, 16, 16), f.userToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", recorder.Code, recorder.Body)
	}

	body := decodeBody(t, recorder)
	if got := body["predicted_category"]; got != "food_waste" {
		t.Fatalf("predicted_category = %v, want food_waste", got)
	}
	if got, _ := body["requires_source_type"].(bool); !got {
		t.Errorf("requires_source_type = false, want true for organics")
	}
	if got := body["group"]; got != "organic" {
		t.Errorf("group = %v, want organic", got)
	}
}

func TestClassifyIsDeterministicForSameImage(t *testing.T) {
	f := newSubmissionFixture(t)
	photo := jpegBytes(t, 24, 24)

	// Unnamed file, so the content-hash path decides — a demo must not produce a
	// different answer on the second take.
	first := decodeBody(t, postPhoto(t, f, "IMG_4021.jpg", photo, f.userToken))
	second := decodeBody(t, postPhoto(t, f, "IMG_4021.jpg", photo, f.userToken))

	if first["predicted_category"] != second["predicted_category"] {
		t.Errorf("same image classified differently: %v then %v",
			first["predicted_category"], second["predicted_category"])
	}
	if first["predicted_confidence"] != second["predicted_confidence"] {
		t.Errorf("same image scored differently: %v then %v",
			first["predicted_confidence"], second["predicted_confidence"])
	}
}

func TestClassifyPredictionIsAlwaysInTaxonomy(t *testing.T) {
	f := newSubmissionFixture(t)

	// Several distinct images, to walk the content-hash branch across categories.
	for i := 1; i <= 12; i++ {
		photo := jpegBytes(t, 8+i, 8+i)
		body := decodeBody(t, postPhoto(t, f, fmt.Sprintf("IMG_%04d.jpg", i), photo, f.userToken))

		category, _ := body["predicted_category"].(string)
		if !models.IsValidMaterialType(category) {
			t.Errorf("image %d: predicted %q, which is not in the taxonomy", i, category)
		}
	}
}

// Alternatives must never repeat the primary guess, or the UI's one-tap
// correction would offer the user the answer they are trying to change.
func TestClassifyAlternativesExcludePrimary(t *testing.T) {
	f := newSubmissionFixture(t)

	body := decodeBody(t, postPhoto(t, f, "glass_clear_jar.jpg", jpegBytes(t, 16, 16), f.userToken))
	primary, _ := body["predicted_category"].(string)

	alternatives, _ := body["alternatives"].([]any)
	for _, raw := range alternatives {
		alt, _ := raw.(map[string]any)
		if alt["predicted_category"] == primary {
			t.Errorf("alternatives repeat the primary category %q", primary)
		}
		category, _ := alt["predicted_category"].(string)
		if !models.IsValidMaterialType(category) {
			t.Errorf("alternative %q is not in the taxonomy", category)
		}
	}
}

// --- malformed uploads: these are the client's fault and do get a 4xx ---

func TestClassifyRejectsMissingPhoto(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := postPhoto(t, f, "", nil, f.userToken)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body %s", recorder.Code, recorder.Body)
	}

	body := decodeBody(t, recorder)
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "validation_error" {
		t.Errorf("code = %v, want validation_error", errObj["code"])
	}
}

func TestClassifyRejectsNonImage(t *testing.T) {
	f := newSubmissionFixture(t)

	// A PDF, correctly named .jpg — the sniffed type must win over the filename.
	recorder := postPhoto(t, f, "receipt.jpg", []byte("%PDF-1.7\n%mock pdf content\n"), f.userToken)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body %s", recorder.Code, recorder.Body)
	}

	body := decodeBody(t, recorder)
	errObj, _ := body["error"].(map[string]any)
	fields, _ := errObj["fields"].(map[string]any)
	if _, ok := fields["photo"]; !ok {
		t.Errorf("expected a photo field error, got %v", errObj)
	}
}

func TestClassifyRejectsEmptyPhoto(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := postPhoto(t, f, "empty.jpg", []byte{}, f.userToken)
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400, body %s", recorder.Code, recorder.Body)
	}
}

// --- POST /submissions: the Phase 2.5 fields ---

func TestCreateStoresPredictionAlongsideManualChoice(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
		"material_type":        "hdpe", // what the user confirmed
		"estimated_qty_kg":     3.0,
		"predicted_category":   "pet", // what the AI guessed — deliberately different
		"predicted_confidence": 0.71,
	}, f.userToken)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", recorder.Code, recorder.Body)
	}

	body := decodeBody(t, recorder)

	// FR-22: both values survive. The manual choice is operative; the prediction
	// is kept precisely because it disagrees.
	if got := body["material_type"]; got != "hdpe" {
		t.Errorf("material_type = %v, want hdpe (the prediction must not override it)", got)
	}
	if got := body["predicted_category"]; got != "pet" {
		t.Errorf("predicted_category = %v, want pet", got)
	}
	if got, _ := body["predicted_confidence"].(float64); got != 0.71 {
		t.Errorf("predicted_confidence = %v, want 0.71", got)
	}
}

// A prediction the server does not recognise must not block a submission the
// user already confirmed by hand (FR-23).
func TestCreateIgnoresUnknownPredictedCategory(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
		"material_type":        "pet",
		"estimated_qty_kg":     2.0,
		"predicted_category":   "styrofoam_peanuts", // not in the taxonomy
		"predicted_confidence": 0.88,
	}, f.userToken)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s — an unknown prediction must not fail the submission",
			recorder.Code, recorder.Body)
	}

	body := decodeBody(t, recorder)
	if got := body["predicted_category"]; got != nil {
		t.Errorf("predicted_category = %v, want null (unknown keys are dropped)", got)
	}
	if got := body["predicted_confidence"]; got != nil {
		t.Errorf("predicted_confidence = %v, want null when the category was dropped", got)
	}
}

func TestCreateClampsOutOfRangeConfidence(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
		"material_type":        "pet",
		"estimated_qty_kg":     2.0,
		"predicted_category":   "pet",
		"predicted_confidence": 1.4, // a model or client can produce this
	}, f.userToken)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", recorder.Code, recorder.Body)
	}

	body := decodeBody(t, recorder)
	if got, _ := body["predicted_confidence"].(float64); got != 1 {
		t.Errorf("predicted_confidence = %v, want 1 (clamped)", got)
	}
}

// FR-24 / FR-4a: organics must declare their source.
func TestCreateRequiresSourceTypeForOrganics(t *testing.T) {
	f := newSubmissionFixture(t)

	for _, material := range []string{"food_waste", "garden_waste"} {
		recorder := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
			"material_type":    material,
			"estimated_qty_kg": 12.0,
		}, f.userToken)

		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", material, recorder.Code)
			continue
		}

		body := decodeBody(t, recorder)
		errObj, _ := body["error"].(map[string]any)
		fields, _ := errObj["fields"].(map[string]any)
		if _, ok := fields["source_type"]; !ok {
			t.Errorf("%s: expected a source_type field error, got %v", material, errObj)
		}
	}
}

func TestCreateAcceptsBothSourceTypesForOrganics(t *testing.T) {
	f := newSubmissionFixture(t)

	for _, source := range []string{models.SourceResidential, models.SourceHotel} {
		recorder := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
			"material_type":    "food_waste",
			"estimated_qty_kg": 30.0,
			"source_type":      source,
		}, f.userToken)

		if recorder.Code != http.StatusCreated {
			t.Fatalf("%s: status = %d, body %s", source, recorder.Code, recorder.Body)
		}
		if got := decodeBody(t, recorder)["source_type"]; got != source {
			t.Errorf("source_type = %v, want %s", got, source)
		}
	}
}

func TestCreateRejectsUnknownSourceType(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
		"material_type":    "food_waste",
		"estimated_qty_kg": 10.0,
		"source_type":      "spaceship",
	}, f.userToken)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body %s", recorder.Code, recorder.Body)
	}
}

// Non-organic submissions leave the column NULL, so "source_type is set" keeps
// meaning "this is organic waste with a declared origin".
func TestCreateDropsSourceTypeForNonOrganics(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
		"material_type":    "pet",
		"estimated_qty_kg": 4.0,
		"source_type":      models.SourceHotel, // harmless, but not meaningful here
	}, f.userToken)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", recorder.Code, recorder.Body)
	}
	if got := decodeBody(t, recorder)["source_type"]; got != nil {
		t.Errorf("source_type = %v, want null for a non-organic material", got)
	}
}

// Phase 2 submissions carry no Phase 2.5 fields at all; those must serialise as
// null rather than as "" or 0, which the client would render as a real value.
func TestCreateWithoutPredictionLeavesFieldsNull(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
		"material_type":    "cardboard",
		"estimated_qty_kg": 5.0,
	}, f.userToken)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", recorder.Code, recorder.Body)
	}

	body := decodeBody(t, recorder)
	for _, key := range []string{"predicted_category", "predicted_confidence", "source_type"} {
		if got, present := body[key]; !present || got != nil {
			t.Errorf("%s = %v (present=%v), want an explicit null", key, got, present)
		}
	}
}

// FR-22's payoff: a collector's correction must not erase what the AI predicted,
// or the accuracy metric would only ever record agreement.
func TestVerifyPreservesPredictionAfterCorrection(t *testing.T) {
	f := newSubmissionFixture(t)

	created := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
		"material_type":        "pet",
		"estimated_qty_kg":     6.0,
		"predicted_category":   "pet",
		"predicted_confidence": 0.9,
	}, f.userToken)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", created.Code, created.Body)
	}
	id := int64(decodeBody(t, created)["id"].(float64))

	// The collector disagrees and corrects the material.
	verified := doJSON(t, f.router, http.MethodPatch,
		fmt.Sprintf("/submissions/%d/verify", id),
		map[string]any{"verified_qty_kg": 6.0, "material_type": "hdpe"}, f.collector)
	if verified.Code != http.StatusOK {
		t.Fatalf("verify: status %d, body %s", verified.Code, verified.Body)
	}

	body := decodeBody(t, verified)
	submission, _ := body["submission"].(map[string]any)

	if got := submission["material_type"]; got != "hdpe" {
		t.Errorf("material_type = %v, want hdpe (the correction)", got)
	}
	if got := submission["predicted_category"]; got != "pet" {
		t.Errorf("predicted_category = %v, want pet — the prediction must survive being corrected", got)
	}
	if got, _ := submission["predicted_confidence"].(float64); got != 0.9 {
		t.Errorf("predicted_confidence = %v, want 0.9", got)
	}
}
