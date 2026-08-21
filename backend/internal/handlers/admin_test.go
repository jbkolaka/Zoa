package handlers_test

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/models"
)

// adminFixture is an admin, a recycler and a collector — enough to drive real
// numbers through the platform rather than asserting against seeded rows.
type adminFixture struct {
	router *gin.Engine
	conn   *sql.DB

	adminToken     string
	userToken      string
	userID         int64
	collectorToken string
}

func newAdminFixture(t *testing.T) *adminFixture {
	t.Helper()

	router, conn := newTestRouterWithDB(t)

	userToken, user := registerUser(t, router, "+254712345678", "Amina Wanjiru")
	collectorToken, collector := registerUser(t, router, "+254712000002", "Joseph Kariuki")
	adminToken, admin := registerUser(t, router, "+254712000004", "Zoa Operations")

	for id, role := range map[int64]string{
		int64(collector["id"].(float64)): models.RoleCollector,
		int64(admin["id"].(float64)):     models.RoleAdmin,
	} {
		if _, err := conn.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, id); err != nil {
			t.Fatalf("promote %d to %s: %v", id, role, err)
		}
	}

	return &adminFixture{
		router:         router,
		conn:           conn,
		adminToken:     adminToken,
		userToken:      userToken,
		userID:         int64(user["id"].(float64)),
		collectorToken: collectorToken,
	}
}

// submit logs a submission, optionally carrying a classifier prediction.
func (f *adminFixture) submit(t *testing.T, material string, kg float64, predicted string) int64 {
	t.Helper()

	body := map[string]any{"material_type": material, "estimated_qty_kg": kg}
	if predicted != "" {
		body["predicted_category"] = predicted
		body["predicted_confidence"] = 0.9
	}
	if models.GroupForMaterial(material) == models.GroupOrganic {
		body["source_type"] = models.SourceResidential
	}

	recorder := doJSON(t, f.router, http.MethodPost, "/submissions", body, f.userToken)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("submit: status %d, body %s", recorder.Code, recorder.Body)
	}
	return int64(decode(t, recorder)["id"].(float64))
}

// verify confirms a submission, optionally correcting the material — which is how
// a wrong prediction gets recorded as wrong.
func (f *adminFixture) verify(t *testing.T, id int64, kg float64, corrected string) {
	t.Helper()

	body := map[string]any{"verified_qty_kg": kg}
	if corrected != "" {
		body["material_type"] = corrected
	}

	recorder := doJSON(t, f.router, http.MethodPatch,
		"/submissions/"+itoa(id)+"/verify", body, f.collectorToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("verify %d: status %d, body %s", id, recorder.Code, recorder.Body)
	}
}

// stats reads the overview as the admin.
func (f *adminFixture) stats(t *testing.T) map[string]any {
	t.Helper()

	recorder := doJSON(t, f.router, http.MethodGet, "/admin/stats", nil, f.adminToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("/admin/stats: status %d, body %s", recorder.Code, recorder.Body)
	}
	return decode(t, recorder)
}

func section(t *testing.T, stats map[string]any, key string) map[string]any {
	t.Helper()

	out, ok := stats[key].(map[string]any)
	if !ok {
		t.Fatalf("stats has no %q section: %v", key, stats)
	}
	return out
}

func number(t *testing.T, block map[string]any, key string) int64 {
	t.Helper()

	raw, ok := block[key].(float64)
	if !ok {
		t.Fatalf("%q is not a number: %v", key, block[key])
	}
	return int64(raw)
}

func TestStatsRequiresAdmin(t *testing.T) {
	f := newAdminFixture(t)

	// A collector sees the queue but not the platform: this endpoint exposes every
	// user's totals, so inheriting it from a lower role would be a leak.
	for name, token := range map[string]string{
		"recycler":  f.userToken,
		"collector": f.collectorToken,
	} {
		recorder := doJSON(t, f.router, http.MethodGet, "/admin/stats", nil, token)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403; body %s", name, recorder.Code, recorder.Body)
		}
	}
}

// TestStatsOnAnEmptyPlatformIsAllZeros is the case most likely to be wrong in
// aggregate SQL: SUM over no rows is NULL, and a missing GROUP BY key is not the
// same as a zero.
func TestStatsOnAnEmptyPlatformIsAllZeros(t *testing.T) {
	f := newAdminFixture(t)

	stats := f.stats(t)

	submissions := section(t, stats, "submissions")
	if got := number(t, submissions, "total"); got != 0 {
		t.Errorf("submissions.total = %d, want 0", got)
	}
	if got := submissions["total_verified_kg"].(float64); got != 0 {
		t.Errorf("total_verified_kg = %v, want 0", got)
	}

	// Every status key must be present at zero, so a client charting this never has
	// to tell "no such key" apart from "none of these".
	byStatus := submissions["by_status"].(map[string]any)
	for _, status := range []string{
		models.SubmissionPending, models.SubmissionCollected,
		models.SubmissionVerified, models.SubmissionRejected,
	} {
		if _, ok := byStatus[status]; !ok {
			t.Errorf("by_status is missing %q: %v", status, byStatus)
		}
	}

	points := section(t, stats, "points")
	for _, key := range []string{"total_issued", "total_spent", "outstanding"} {
		if got := number(t, points, key); got != 0 {
			t.Errorf("points.%s = %d, want 0", key, got)
		}
	}

	// Accuracy must be null, not 0.0: an untested model is not a model that is
	// wrong every time, and "0% accurate" from an empty database would be the worst
	// possible way to present this feature.
	classification := section(t, stats, "classification")
	if classification["accuracy"] != nil {
		t.Errorf("accuracy = %v, want null on an empty platform", classification["accuracy"])
	}
}

func TestStatsCountsUsersByRole(t *testing.T) {
	f := newAdminFixture(t)

	users := section(t, f.stats(t), "users")
	if got := number(t, users, "total"); got != 3 {
		t.Errorf("users.total = %d, want 3", got)
	}

	byRole := users["by_role"].(map[string]any)
	want := map[string]int64{
		models.RoleUser:         1,
		models.RoleCollector:    1,
		models.RoleAdmin:        1,
		models.RolePartnerStaff: 0,
	}
	for role, count := range want {
		got, ok := byRole[role].(float64)
		if !ok {
			t.Errorf("by_role is missing %q: %v", role, byRole)
			continue
		}
		if int64(got) != count {
			t.Errorf("by_role[%q] = %d, want %d", role, int64(got), count)
		}
	}
}

// TestStatsCountsOnlyVerifiedWeight: the headline "kg diverted" figure has to be
// what a collector measured, not what users guessed, or it is a number the
// platform cannot stand behind.
func TestStatsCountsOnlyVerifiedWeight(t *testing.T) {
	f := newAdminFixture(t)

	verified := f.submit(t, "pet", 4.0, "")
	f.verify(t, verified, 3.5, "")

	f.submit(t, "cardboard", 100.0, "") // still pending — must not count

	submissions := section(t, f.stats(t), "submissions")
	if got := number(t, submissions, "total"); got != 2 {
		t.Errorf("submissions.total = %d, want 2", got)
	}
	if got := submissions["total_verified_kg"].(float64); got != 3.5 {
		t.Errorf("total_verified_kg = %v, want 3.5 — an estimate leaked in", got)
	}

	byStatus := submissions["by_status"].(map[string]any)
	if got := int64(byStatus[models.SubmissionVerified].(float64)); got != 1 {
		t.Errorf("by_status.verified = %d, want 1", got)
	}
	if got := int64(byStatus[models.SubmissionPending].(float64)); got != 1 {
		t.Errorf("by_status.pending = %d, want 1", got)
	}
}

// TestStatsClassificationAccuracy is the FR-22 metric end to end: a prediction the
// collector accepted counts as correct, one they corrected counts as wrong, and an
// unverified prediction counts as neither.
func TestStatsClassificationAccuracy(t *testing.T) {
	f := newAdminFixture(t)

	// Predicted pet, collector agreed → correct.
	right := f.submit(t, "pet", 4.0, "pet")
	f.verify(t, right, 4.0, "")

	// Predicted pet, collector corrected it to hdpe → wrong. The correction
	// overwrites material_type and leaves predicted_category alone, which is the
	// whole reason this is measurable.
	wrong := f.submit(t, "pet", 4.0, "pet")
	f.verify(t, wrong, 4.0, "hdpe")

	// Predicted but never verified → counted as a prediction, but not judged.
	f.submit(t, "glass_clear", 2.0, "glass_clear")

	// No prediction at all → invisible to this block.
	noPrediction := f.submit(t, "aluminum", 1.0, "")
	f.verify(t, noPrediction, 1.0, "")

	classification := section(t, f.stats(t), "classification")

	if got := number(t, classification, "predictions_made"); got != 3 {
		t.Errorf("predictions_made = %d, want 3", got)
	}
	if got := number(t, classification, "verified_against"); got != 2 {
		t.Errorf("verified_against = %d, want 2 — an unverified prediction was judged", got)
	}
	if got := number(t, classification, "correct"); got != 1 {
		t.Errorf("correct = %d, want 1", got)
	}

	accuracy, ok := classification["accuracy"].(float64)
	if !ok {
		t.Fatalf("accuracy is not a number: %v", classification["accuracy"])
	}
	if accuracy != 0.5 {
		t.Errorf("accuracy = %v, want 0.5 (1 of 2 verified predictions)", accuracy)
	}
}

// TestStatsCorrectedPredictionSurvivesAsWrong pins the property the metric depends
// on: a collector's correction must not quietly rewrite predicted_category, or
// every wrong guess would retroactively become a right one and accuracy would
// always read 100%.
func TestStatsCorrectedPredictionSurvivesAsWrong(t *testing.T) {
	f := newAdminFixture(t)

	id := f.submit(t, "pet", 4.0, "pet")
	f.verify(t, id, 4.0, "hdpe")

	var predicted, material string
	if err := f.conn.QueryRow(
		`SELECT predicted_category, material_type FROM submissions WHERE id = ?`, id,
	).Scan(&predicted, &material); err != nil {
		t.Fatalf("read submission: %v", err)
	}
	if predicted != "pet" {
		t.Errorf("predicted_category = %q, want it preserved as pet", predicted)
	}
	if material != "hdpe" {
		t.Errorf("material_type = %q, want the correction to hdpe", material)
	}

	classification := section(t, f.stats(t), "classification")
	if got := number(t, classification, "correct"); got != 0 {
		t.Errorf("correct = %d, want 0 — a corrected prediction counted as right", got)
	}
}

// TestStatsPointsMatchLedgerAndBalances walks earn-then-spend and asserts the
// invariant the whole design rests on, at platform scale: issued minus spent is
// outstanding, and outstanding equals the sum of every cached balance.
func TestStatsPointsMatchLedgerAndBalances(t *testing.T) {
	f := newAdminFixture(t)

	// pet pays 25/kg, so 8 kg earns 200 — exactly the cost of the seeded
	// "Free tea or coffee" voucher, which lands the balance on a clean zero.
	id := f.submit(t, "pet", 8.0, "")
	f.verify(t, id, 8.0, "")

	var voucherID int64
	if err := f.conn.QueryRow(
		`SELECT id FROM vouchers WHERE title = ?`, "Free tea or coffee",
	).Scan(&voucherID); err != nil {
		t.Fatalf("look up voucher: %v", err)
	}

	recorder := doJSON(t, f.router, http.MethodPost, "/redemptions",
		map[string]any{"voucher_id": voucherID}, f.userToken)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("redeem: status %d, body %s", recorder.Code, recorder.Body)
	}

	stats := f.stats(t)
	points := section(t, stats, "points")

	if got := number(t, points, "total_issued"); got != 200 {
		t.Errorf("total_issued = %d, want 200", got)
	}
	if got := number(t, points, "total_spent"); got != 200 {
		t.Errorf("total_spent = %d, want 200", got)
	}
	if got := number(t, points, "outstanding"); got != 0 {
		t.Errorf("outstanding = %d, want 0", got)
	}

	// The cross-check that would catch a drift between the ledger and the cached
	// balances, which no single-user test can see.
	var summedBalances int64
	if err := f.conn.QueryRow(
		`SELECT COALESCE(SUM(points_balance), 0) FROM users`,
	).Scan(&summedBalances); err != nil {
		t.Fatalf("sum balances: %v", err)
	}
	if summedBalances != number(t, points, "outstanding") {
		t.Errorf("balances sum to %d but outstanding is %d — the ledger and the cache have drifted",
			summedBalances, number(t, points, "outstanding"))
	}

	redemptions := section(t, stats, "redemptions")
	if got := number(t, redemptions, "total"); got != 1 {
		t.Errorf("redemptions.total = %d, want 1", got)
	}
	byStatus := redemptions["by_status"].(map[string]any)
	if got := int64(byStatus[models.RedemptionIssued].(float64)); got != 1 {
		t.Errorf("by_status.issued = %d, want 1", got)
	}
	// Present at zero rather than absent, same rule as everywhere else.
	if _, ok := byStatus[models.RedemptionExpired]; !ok {
		t.Errorf("by_status is missing %q: %v", models.RedemptionExpired, byStatus)
	}
}
