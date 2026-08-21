package handlers_test

import (
	"fmt"
	"net/http"
	"testing"
)

// The catalogue comes from migration 004, so these tests run against the real
// seed rather than fixtures: if the seed regresses, the demo has no vouchers, and
// that is exactly what should fail a build.

// voucherList fetches the catalogue and returns (vouchers, points_balance).
func voucherList(t *testing.T, f *submissionFixture, query, token string) ([]map[string]any, int64) {
	t.Helper()

	recorder := doJSON(t, f.router, http.MethodGet, "/vouchers"+query, nil, token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /vouchers%s: status %d, body %s", query, recorder.Code, recorder.Body)
	}

	body := decodeBody(t, recorder)

	raw, _ := body["vouchers"].([]any)
	vouchers := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if voucher, ok := item.(map[string]any); ok {
			vouchers = append(vouchers, voucher)
		}
	}

	balance, _ := body["points_balance"].(float64)
	return vouchers, int64(balance)
}

func TestVouchersRequireAuth(t *testing.T) {
	f := newSubmissionFixture(t)

	for _, path := range []string{"/vouchers", "/vouchers/1", "/partners"} {
		recorder := doJSON(t, f.router, http.MethodGet, path, nil, "")
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", path, recorder.Code)
		}
	}
}

func TestVoucherCatalogueIsSeededAndShaped(t *testing.T) {
	f := newSubmissionFixture(t)

	vouchers, balance := voucherList(t, f, "", f.userToken)

	if len(vouchers) == 0 {
		t.Fatal("catalogue is empty — migration 004 did not seed vouchers")
	}
	if balance != 0 {
		t.Errorf("points_balance = %d, want 0 for a fresh user", balance)
	}

	// Every field the contract promises, on the first row.
	first := vouchers[0]
	for _, key := range []string{
		"id", "partner_id", "title", "points_cost", "discount_type",
		"discount_value", "expiry_days", "active", "partner", "affordable",
	} {
		if _, present := first[key]; !present {
			t.Errorf("voucher is missing %q: %v", key, first)
		}
	}

	// The partner is embedded so the catalogue renders in one call.
	partner, ok := first["partner"].(map[string]any)
	if !ok {
		t.Fatalf("partner is not an object: %v", first["partner"])
	}
	if name, _ := partner["name"].(string); name == "" {
		t.Errorf("embedded partner has no name: %v", partner)
	}

	// discount_type is a closed set the client branches on.
	for _, voucher := range vouchers {
		kind, _ := voucher["discount_type"].(string)
		if kind != "percentage" && kind != "fixed" {
			t.Errorf("voucher %v has discount_type %q", voucher["id"], kind)
		}
	}
}

// Cheapest first puts what the user can reach soonest at the top.
func TestVoucherCatalogueIsOrderedByCost(t *testing.T) {
	f := newSubmissionFixture(t)

	vouchers, _ := voucherList(t, f, "", f.userToken)

	previous := int64(-1)
	for _, voucher := range vouchers {
		cost := int64(voucher["points_cost"].(float64))
		if cost < previous {
			t.Errorf("cost %d came after %d — catalogue is not cheapest-first", cost, previous)
		}
		previous = cost
	}
}

// A fresh user affords nothing, so `affordable` must be false everywhere. This is
// the flag the client disables its button on, so a wrong default is a button that
// lies.
func TestAffordableIsFalseForBrokeUser(t *testing.T) {
	f := newSubmissionFixture(t)

	vouchers, _ := voucherList(t, f, "", f.userToken)

	for _, voucher := range vouchers {
		if affordable, _ := voucher["affordable"].(bool); affordable {
			t.Errorf("voucher %v (cost %v) is affordable at balance 0",
				voucher["id"], voucher["points_cost"])
		}
	}
}

// The balance must be read live, not from the token: a user who earns points mid
// session should see the catalogue change without signing out.
func TestAffordabilityTracksEarnedPoints(t *testing.T) {
	f := newSubmissionFixture(t)

	// Earn a known number of points on the same token the catalogue is read with.
	// aluminum is 40/kg, so 20kg = 800 points.
	id := f.submit(t, "aluminum", 20.0)
	doJSON(t, f.router, http.MethodPatch, fmt.Sprintf("/submissions/%d/verify", id),
		map[string]any{"verified_qty_kg": 20.0}, f.collector)

	vouchers, balance := voucherList(t, f, "", f.userToken)
	if balance != 800 {
		t.Fatalf("points_balance = %d, want 800 — the balance is not read live", balance)
	}

	var affordableCount int
	for _, voucher := range vouchers {
		cost := int64(voucher["points_cost"].(float64))
		affordable, _ := voucher["affordable"].(bool)

		if want := balance >= cost; affordable != want {
			t.Errorf("voucher %v cost %d at balance %d: affordable = %v, want %v",
				voucher["id"], cost, balance, affordable, want)
		}
		if affordable {
			affordableCount++
		}
	}

	if affordableCount == 0 {
		t.Error("800 points affords nothing — the seed's cheapest voucher is out of reach")
	}
}

func TestAffordableFilterNarrowsTheCatalogue(t *testing.T) {
	f := newSubmissionFixture(t)

	// 300 points: enough for the cheapest tier, not for the rest.
	id := f.submit(t, "pet", 12.0) // 25/kg → 300
	doJSON(t, f.router, http.MethodPatch, fmt.Sprintf("/submissions/%d/verify", id),
		map[string]any{"verified_qty_kg": 12.0}, f.collector)

	all, balance := voucherList(t, f, "", f.userToken)
	affordable, _ := voucherList(t, f, "?affordable=true", f.userToken)

	if balance != 300 {
		t.Fatalf("points_balance = %d, want 300", balance)
	}
	if len(affordable) == 0 {
		t.Fatal("affordable=true returned nothing at 300 points")
	}
	if len(affordable) >= len(all) {
		t.Errorf("affordable=true returned %d of %d — the filter did nothing",
			len(affordable), len(all))
	}

	for _, voucher := range affordable {
		cost := int64(voucher["points_cost"].(float64))
		if cost > balance {
			t.Errorf("voucher %v costs %d, over the %d balance", voucher["id"], cost, balance)
		}
		if ok, _ := voucher["affordable"].(bool); !ok {
			t.Errorf("voucher %v in the affordable list is flagged unaffordable", voucher["id"])
		}
	}
}

// `affordable=false` means "show everything", not "show what I cannot afford".
func TestAffordableFalseDoesNotFilter(t *testing.T) {
	f := newSubmissionFixture(t)

	all, _ := voucherList(t, f, "", f.userToken)
	explicit, _ := voucherList(t, f, "?affordable=false", f.userToken)

	if len(all) != len(explicit) {
		t.Errorf("affordable=false returned %d, unfiltered returned %d — should match",
			len(explicit), len(all))
	}
}

func TestPartnerFilter(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := doJSON(t, f.router, http.MethodGet, "/partners", nil, f.userToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /partners: status %d, body %s", recorder.Code, recorder.Body)
	}

	rawPartners, _ := decodeBody(t, recorder)["partners"].([]any)
	if len(rawPartners) < 2 {
		t.Fatalf("expected several seeded partners, got %d", len(rawPartners))
	}

	first, _ := rawPartners[0].(map[string]any)
	partnerID := int64(first["id"].(float64))

	filtered, _ := voucherList(t, f, fmt.Sprintf("?partner_id=%d", partnerID), f.userToken)
	if len(filtered) == 0 {
		t.Fatalf("partner %d has no vouchers", partnerID)
	}

	for _, voucher := range filtered {
		if got := int64(voucher["partner_id"].(float64)); got != partnerID {
			t.Errorf("voucher %v has partner_id %d, want %d", voucher["id"], got, partnerID)
		}
	}

	// And it genuinely narrows: one partner does not own the whole catalogue.
	all, _ := voucherList(t, f, "", f.userToken)
	if len(filtered) >= len(all) {
		t.Errorf("partner filter returned %d of %d rows", len(filtered), len(all))
	}
}

func TestPartnerFilterRejectsGarbage(t *testing.T) {
	f := newSubmissionFixture(t)

	for _, raw := range []string{"abc", "0", "-3", "1.5"} {
		recorder := doJSON(t, f.router, http.MethodGet, "/vouchers?partner_id="+raw, nil, f.userToken)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("partner_id=%q: status = %d, want 400", raw, recorder.Code)
		}
	}
}

// An unknown partner is an empty catalogue, not an error: the filter is valid,
// there is simply nothing behind it.
func TestUnknownPartnerReturnsEmptyList(t *testing.T) {
	f := newSubmissionFixture(t)

	vouchers, _ := voucherList(t, f, "?partner_id=99999", f.userToken)
	if len(vouchers) != 0 {
		t.Errorf("got %d vouchers for a nonexistent partner", len(vouchers))
	}
}

// --- GET /vouchers/:id ---

func TestVoucherByID(t *testing.T) {
	f := newSubmissionFixture(t)

	vouchers, _ := voucherList(t, f, "", f.userToken)
	wantID := int64(vouchers[0]["id"].(float64))

	recorder := doJSON(t, f.router, http.MethodGet,
		fmt.Sprintf("/vouchers/%d", wantID), nil, f.userToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", recorder.Code, recorder.Body)
	}

	body := decodeBody(t, recorder)
	if got := int64(body["id"].(float64)); got != wantID {
		t.Errorf("id = %d, want %d", got, wantID)
	}
	// Same shape as a list entry, so the detail screen reuses the list model.
	if _, ok := body["partner"].(map[string]any); !ok {
		t.Errorf("detail response has no embedded partner: %v", body)
	}
	if _, present := body["affordable"]; !present {
		t.Error("detail response has no affordable flag")
	}
}

func TestVoucherByIDRejectsMissingAndInactive(t *testing.T) {
	f := newSubmissionFixture(t)

	// The seed carries one inactive voucher ("Expired launch promo") specifically
	// so this path is covered by real data. It must be invisible in the catalogue
	// and unresolvable by id — and indistinguishable from a nonexistent one, so an
	// inactive offer cannot be discovered by probing.
	vouchers, _ := voucherList(t, f, "", f.userToken)
	for _, voucher := range vouchers {
		if title, _ := voucher["title"].(string); title == "Expired launch promo" {
			t.Error("an inactive voucher is visible in the catalogue")
		}
	}

	missing := doJSON(t, f.router, http.MethodGet, "/vouchers/99999", nil, f.userToken)
	if missing.Code != http.StatusNotFound {
		t.Errorf("unknown id: status = %d, want 404", missing.Code)
	}

	// Find the inactive voucher's id the only way a prober could: by scanning.
	// Whatever id it has must answer exactly like the unknown one above.
	for id := int64(1); id <= 20; id++ {
		recorder := doJSON(t, f.router, http.MethodGet,
			fmt.Sprintf("/vouchers/%d", id), nil, f.userToken)
		if recorder.Code == http.StatusNotFound {
			if recorder.Body.String() != missing.Body.String() {
				t.Errorf("id %d 404s differently from an unknown id:\n  %s\n  %s",
					id, recorder.Body, missing.Body)
			}
		}
	}
}

func TestVoucherByIDRejectsGarbageID(t *testing.T) {
	f := newSubmissionFixture(t)

	for _, raw := range []string{"abc", "0", "-1"} {
		recorder := doJSON(t, f.router, http.MethodGet, "/vouchers/"+raw, nil, f.userToken)
		if recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusNotFound {
			t.Errorf("id=%q: status = %d, want 400 or 404", raw, recorder.Code)
		}
	}
}

// Out-of-stock vouchers are hidden rather than shown disabled: stock is real, and
// a catalogue that offers something redemption will refuse has misled the user.
func TestOutOfStockVouchersAreHidden(t *testing.T) {
	f := newSubmissionFixture(t)

	vouchers, _ := voucherList(t, f, "", f.userToken)
	for _, voucher := range vouchers {
		stock, present := voucher["stock_remaining"]
		if !present || stock == nil {
			continue // NULL means unlimited
		}
		if remaining := int64(stock.(float64)); remaining <= 0 {
			t.Errorf("voucher %v is listed with %d in stock", voucher["id"], remaining)
		}
	}
}

// Unlimited stock must survive the round trip as null, not 0 — the client renders
// "0 left" for a voucher that is actually always available.
func TestUnlimitedStockSerialisesAsNull(t *testing.T) {
	f := newSubmissionFixture(t)

	vouchers, _ := voucherList(t, f, "", f.userToken)

	var sawUnlimited bool
	for _, voucher := range vouchers {
		if stock, present := voucher["stock_remaining"]; present && stock == nil {
			sawUnlimited = true
			break
		}
	}
	if !sawUnlimited {
		t.Error("no voucher has null stock — the unlimited case is unexercised by the seed")
	}
}

// A collector or admin browsing the catalogue is not an error; they have a
// balance like anyone else.
func TestCatalogueIsReadableByCollector(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := doJSON(t, f.router, http.MethodGet, "/vouchers", nil, f.collector)
	if recorder.Code != http.StatusOK {
		t.Errorf("collector: status = %d, want 200", recorder.Code)
	}
}
