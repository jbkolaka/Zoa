package handlers_test

import (
	"net/http"
	"testing"

	"zoa/backend/internal/models"
)

// admin returns the seeded-and-promoted admin token from adminFixture.
//
// The catalogue tests reuse that fixture rather than building a second one: they
// need the same cast (an admin to act, a recycler to redeem) and the seeded
// partners and vouchers migration 004 already provides.

func TestCatalogAdminRequiresAdmin(t *testing.T) {
	f := newAdminFixture(t)

	// Every route, not just one: a role check missing from a single verb is exactly
	// the kind of gap that survives review.
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/admin/partners"},
		{http.MethodPost, "/admin/partners"},
		{http.MethodGet, "/admin/partners/1"},
		{http.MethodPatch, "/admin/partners/1"},
		{http.MethodDelete, "/admin/partners/1"},
		{http.MethodGet, "/admin/vouchers"},
		{http.MethodPost, "/admin/vouchers"},
		{http.MethodGet, "/admin/vouchers/1"},
		{http.MethodPatch, "/admin/vouchers/1"},
		{http.MethodDelete, "/admin/vouchers/1"},
	}

	for _, tc := range cases {
		for name, token := range map[string]string{
			"recycler":  f.userToken,
			"collector": f.collectorToken,
		} {
			recorder := doJSON(t, f.router, tc.method, tc.path, map[string]any{}, token)
			if recorder.Code != http.StatusForbidden {
				t.Errorf("%s %s as %s: status = %d, want 403",
					tc.method, tc.path, name, recorder.Code)
			}
		}
	}
}

// TestAdminListingsShowInactiveRows: the admin view exists to show what the
// catalogue is hiding, so filtering it the same way would make a deactivated offer
// unreachable and impossible to switch back on.
func TestAdminListingsShowInactiveRows(t *testing.T) {
	f := newAdminFixture(t)

	// Migration 004 seeds one deliberately inactive voucher.
	recorder := doJSON(t, f.router, http.MethodGet, "/admin/vouchers", nil, f.adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", recorder.Code, recorder.Body)
	}

	var inactive int
	for _, raw := range decode(t, recorder)["vouchers"].([]any) {
		if !raw.(map[string]any)["active"].(bool) {
			inactive++
		}
	}
	if inactive == 0 {
		t.Error("no inactive vouchers listed — the admin view is filtering like the catalogue")
	}

	// And the public catalogue still hides it, which is the contrast that matters.
	recorder = doJSON(t, f.router, http.MethodGet, "/vouchers", nil, f.userToken)
	for _, raw := range decode(t, recorder)["vouchers"].([]any) {
		if !raw.(map[string]any)["active"].(bool) {
			t.Error("the public catalogue leaked an inactive voucher")
		}
	}
}

func TestCreatePartnerAndVoucher(t *testing.T) {
	f := newAdminFixture(t)

	recorder := doJSON(t, f.router, http.MethodPost, "/admin/partners", map[string]any{
		"name": "  Carrefour  ",
	}, f.adminToken)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create partner: status %d, body %s", recorder.Code, recorder.Body)
	}

	partner := decode(t, recorder)
	// Trimmed on the way in, like every other free-text field.
	if partner["name"] != "Carrefour" {
		t.Errorf("name = %v, want it trimmed to Carrefour", partner["name"])
	}
	// Active by default: a partner is created because it is joining the programme.
	if partner["active"] != true {
		t.Errorf("active = %v, want true by default", partner["active"])
	}
	partnerID := int64(partner["id"].(float64))

	recorder = doJSON(t, f.router, http.MethodPost, "/admin/vouchers", map[string]any{
		"partner_id":     partnerID,
		"title":          "KSh 300 off",
		"points_cost":    500,
		"discount_type":  "Fixed", // case is normalised for the CHECK constraint
		"discount_value": 300,
		"expiry_days":    30,
	}, f.adminToken)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create voucher: status %d, body %s", recorder.Code, recorder.Body)
	}

	voucher := decode(t, recorder)
	if voucher["discount_type"] != models.DiscountFixed {
		t.Errorf("discount_type = %v, want %q", voucher["discount_type"], models.DiscountFixed)
	}
	// Omitted stock means unlimited, and must serialise as null rather than 0.
	if voucher["stock_remaining"] != nil {
		t.Errorf("stock_remaining = %v, want null when omitted", voucher["stock_remaining"])
	}
	if partnerBlock, ok := voucher["partner"].(map[string]any); !ok || partnerBlock["name"] != "Carrefour" {
		t.Errorf("partner not embedded correctly: %v", voucher["partner"])
	}

	// And it is immediately redeemable-looking in the public catalogue.
	recorder = doJSON(t, f.router, http.MethodGet, "/vouchers", nil, f.userToken)
	var found bool
	for _, raw := range decode(t, recorder)["vouchers"].([]any) {
		if raw.(map[string]any)["title"] == "KSh 300 off" {
			found = true
		}
	}
	if !found {
		t.Error("a newly created active voucher did not appear in the catalogue")
	}
}

func TestCreateVoucherValidation(t *testing.T) {
	f := newAdminFixture(t)

	cases := map[string]struct {
		body  map[string]any
		field string
	}{
		"missing everything": {map[string]any{}, "title"},
		"unknown partner": {map[string]any{
			"partner_id": 9999, "title": "x", "points_cost": 10,
			"discount_type": "fixed", "discount_value": 5, "expiry_days": 7,
		}, "partner_id"},
		"zero cost": {map[string]any{
			"partner_id": 1, "title": "x", "points_cost": 0,
			"discount_type": "fixed", "discount_value": 5, "expiry_days": 7,
		}, "points_cost"},
		"percentage over 100": {map[string]any{
			"partner_id": 1, "title": "x", "points_cost": 10,
			"discount_type": "percentage", "discount_value": 150, "expiry_days": 7,
		}, "discount_value"},
		"bad discount type": {map[string]any{
			"partner_id": 1, "title": "x", "points_cost": 10,
			"discount_type": "buy_one_get_one", "discount_value": 5, "expiry_days": 7,
		}, "discount_type"},
		"negative stock": {map[string]any{
			"partner_id": 1, "title": "x", "points_cost": 10,
			"discount_type": "fixed", "discount_value": 5, "expiry_days": 7,
			"stock_remaining": -1,
		}, "stock_remaining"},
	}

	for name, tc := range cases {
		recorder := doJSON(t, f.router, http.MethodPost, "/admin/vouchers", tc.body, f.adminToken)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400; body %s", name, recorder.Code, recorder.Body)
			continue
		}
		fields, ok := decode(t, recorder)["error"].(map[string]any)["fields"].(map[string]any)
		if !ok || fields[tc.field] == nil {
			t.Errorf("%s: no %q field error; body %s", name, tc.field, recorder.Body)
		}
	}
}

// TestPatchStockDistinguishesAbsentFromNull is why Optional[T] exists: omitting
// stock_remaining must leave it alone, while sending an explicit null must make the
// voucher unlimited. A plain pointer collapses those two into one.
func TestPatchStockDistinguishesAbsentFromNull(t *testing.T) {
	f := newAdminFixture(t)

	id := f.voucherID(t, "KSh 100 off your shopping") // seeded with stock 200

	// Absent: stock untouched by an unrelated edit.
	recorder := doJSON(t, f.router, http.MethodPatch, "/admin/vouchers/"+itoa(id),
		map[string]any{"title": "KSh 100 off groceries"}, f.adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("patch title: status %d, body %s", recorder.Code, recorder.Body)
	}
	if got := decode(t, recorder)["stock_remaining"]; got == nil || int64(got.(float64)) != 200 {
		t.Errorf("stock_remaining = %v, want 200 left alone", got)
	}

	// Explicit null: now unlimited.
	recorder = doJSON(t, f.router, http.MethodPatch, "/admin/vouchers/"+itoa(id),
		map[string]any{"stock_remaining": nil}, f.adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("patch stock to null: status %d, body %s", recorder.Code, recorder.Body)
	}
	if got := decode(t, recorder)["stock_remaining"]; got != nil {
		t.Errorf("stock_remaining = %v, want null (unlimited)", got)
	}

	// And a number puts a limit back.
	recorder = doJSON(t, f.router, http.MethodPatch, "/admin/vouchers/"+itoa(id),
		map[string]any{"stock_remaining": 5}, f.adminToken)
	if got := decode(t, recorder)["stock_remaining"]; got == nil || int64(got.(float64)) != 5 {
		t.Errorf("stock_remaining = %v, want 5", got)
	}
}

func TestPatchWithNoFieldsIsRefused(t *testing.T) {
	f := newAdminFixture(t)

	// An empty patch is far more likely a client bug than a deliberate no-op.
	recorder := doJSON(t, f.router, http.MethodPatch, "/admin/partners/1",
		map[string]any{}, f.adminToken)
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body %s", recorder.Code, recorder.Body)
	}
}

func TestPatchCanClearAPartnerLogoButNotItsName(t *testing.T) {
	f := newAdminFixture(t)

	recorder := doJSON(t, f.router, http.MethodPatch, "/admin/partners/1",
		map[string]any{"logo_url": "https://example.test/logo.png"}, f.adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("set logo: status %d, body %s", recorder.Code, recorder.Body)
	}

	recorder = doJSON(t, f.router, http.MethodPatch, "/admin/partners/1",
		map[string]any{"logo_url": nil}, f.adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("clear logo: status %d, body %s", recorder.Code, recorder.Body)
	}
	if got := decode(t, recorder)["logo_url"]; got != nil {
		t.Errorf("logo_url = %v, want null after clearing", got)
	}

	// The name is different: every screen renders it, so blanking it would leave a
	// voucher attributed to nobody.
	recorder = doJSON(t, f.router, http.MethodPatch, "/admin/partners/1",
		map[string]any{"name": "   "}, f.adminToken)
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("blank name: status = %d, want 400", recorder.Code)
	}
}

func TestAdminUnknownIDsAreNotFound(t *testing.T) {
	f := newAdminFixture(t)

	for _, path := range []string{"/admin/partners/9999", "/admin/vouchers/9999"} {
		recorder := doJSON(t, f.router, http.MethodGet, path, nil, f.adminToken)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, recorder.Code)
		}

		recorder = doJSON(t, f.router, http.MethodPatch, path,
			map[string]any{"active": false}, f.adminToken)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("PATCH %s: status = %d, want 404", path, recorder.Code)
		}

		recorder = doJSON(t, f.router, http.MethodDelete, path, nil, f.adminToken)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("DELETE %s: status = %d, want 404", path, recorder.Code)
		}
	}
}

// TestSoftDeleteKeepsIssuedCodesWorking is the invariant the whole soft-delete
// design exists for: withdrawing an offer must not strand a user standing at a till
// holding a code they already paid for.
func TestSoftDeleteKeepsIssuedCodesWorking(t *testing.T) {
	f := newAdminFixture(t)

	// Earn, then redeem the cheapest seeded voucher.
	id := f.submit(t, "pet", 8.0, "")
	f.verify(t, id, 8.0, "")

	voucherID := f.voucherID(t, "KSh 100 off your shopping")
	recorder := doJSON(t, f.router, http.MethodPost, "/redemptions",
		map[string]any{"voucher_id": voucherID}, f.userToken)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("redeem: status %d, body %s", recorder.Code, recorder.Body)
	}
	code := decode(t, recorder)["code"].(string)

	// Now withdraw the offer entirely.
	recorder = doJSON(t, f.router, http.MethodDelete,
		"/admin/vouchers/"+itoa(voucherID), nil, f.adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("deactivate: status %d, body %s", recorder.Code, recorder.Body)
	}
	if decode(t, recorder)["voucher"].(map[string]any)["active"] != false {
		t.Error("voucher is still active after a delete")
	}

	// The row survives — a hard delete would have broken the redemption's FK.
	var stillThere int64
	if err := f.conn.QueryRow(
		`SELECT COUNT(*) FROM vouchers WHERE id = ?`, voucherID,
	).Scan(&stillThere); err != nil {
		t.Fatalf("count voucher: %v", err)
	}
	if stillThere != 1 {
		t.Fatal("the voucher row was hard-deleted — issued redemptions now dangle")
	}

	// It is gone from the catalogue and no longer redeemable...
	recorder = doJSON(t, f.router, http.MethodGet, "/vouchers/"+itoa(voucherID), nil, f.userToken)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("catalogue lookup: status = %d, want 404", recorder.Code)
	}

	// ...but the code already in the user's hands still verifies.
	recorder = doJSON(t, f.router, http.MethodGet, "/redemptions", nil, f.userToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("history: status %d", recorder.Code)
	}
	if items := decode(t, recorder)["redemptions"].([]any); len(items) != 1 {
		t.Errorf("history has %d codes, want the withdrawn one still listed", len(items))
	}

	// Promote the recycler's own account is not needed — admin may verify.
	recorder = doJSON(t, f.router, http.MethodPost,
		"/redemptions/"+code+"/verify", nil, f.adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("verify a code for a withdrawn offer: status %d, want 200; body %s",
			recorder.Code, recorder.Body)
	}
}

// TestDeactivatingAPartnerHidesItsVouchers: a retailer leaving takes its whole
// offer list with it, without any voucher row being rewritten.
func TestDeactivatingAPartnerHidesItsVouchers(t *testing.T) {
	f := newAdminFixture(t)

	var partnerID int64
	if err := f.conn.QueryRow(
		`SELECT id FROM partners WHERE name = ?`, "Java House",
	).Scan(&partnerID); err != nil {
		t.Fatalf("look up partner: %v", err)
	}

	before := f.catalogueTitles(t)

	recorder := doJSON(t, f.router, http.MethodDelete,
		"/admin/partners/"+itoa(partnerID), nil, f.adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("deactivate partner: status %d, body %s", recorder.Code, recorder.Body)
	}

	after := f.catalogueTitles(t)
	if len(after) >= len(before) {
		t.Errorf("catalogue went from %d to %d entries — the partner's vouchers are still listed",
			len(before), len(after))
	}

	// The vouchers themselves were not touched; only the join hides them.
	var stillActive int64
	if err := f.conn.QueryRow(
		`SELECT COUNT(*) FROM vouchers WHERE partner_id = ? AND active = 1`, partnerID,
	).Scan(&stillActive); err != nil {
		t.Fatalf("count vouchers: %v", err)
	}
	if stillActive == 0 {
		t.Error("voucher rows were rewritten — deactivating a partner should not cascade")
	}
}

// catalogueTitles lists what the public catalogue currently shows.
func (f *adminFixture) catalogueTitles(t *testing.T) []string {
	t.Helper()

	recorder := doJSON(t, f.router, http.MethodGet, "/vouchers", nil, f.userToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("/vouchers: status %d", recorder.Code)
	}

	var titles []string
	for _, raw := range decode(t, recorder)["vouchers"].([]any) {
		titles = append(titles, raw.(map[string]any)["title"].(string))
	}
	return titles
}

// voucherID resolves a seeded voucher by title.
func (f *adminFixture) voucherID(t *testing.T, title string) int64 {
	t.Helper()

	var id int64
	if err := f.conn.QueryRow(
		`SELECT id FROM vouchers WHERE title = ?`, title,
	).Scan(&id); err != nil {
		t.Fatalf("look up voucher %q: %v", title, err)
	}
	return id
}
