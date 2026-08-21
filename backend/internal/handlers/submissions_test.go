package handlers_test

import (
	"database/sql"
	"net/http"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/models"
)

// submissionFixture is a recycler, a collector, and their tokens.
type submissionFixture struct {
	router    *gin.Engine
	conn      *sql.DB
	userToken string
	userID    int64
	collector string
}

func newSubmissionFixture(t *testing.T) *submissionFixture {
	t.Helper()

	router, conn := newTestRouterWithDB(t)

	userToken, user := registerUser(t, router, "+254712345678", "Amina Wanjiru")
	userID := int64(user["id"].(float64))

	// register only ever creates role `user`, so the collector is promoted
	// directly — the same thing an admin would do in the real product.
	collectorToken, collector := registerUser(t, router, "+254712000002", "Joseph Kariuki")
	if _, err := conn.Exec(
		`UPDATE users SET role = 'collector' WHERE id = ?`,
		int64(collector["id"].(float64)),
	); err != nil {
		t.Fatalf("promote collector: %v", err)
	}

	return &submissionFixture{
		router:    router,
		conn:      conn,
		userToken: userToken,
		userID:    userID,
		collector: collectorToken,
	}
}

// submit creates a submission and returns its id.
//
// Organic materials get a source_type automatically: Phase 2.5 makes it
// mandatory for that group (FR-24), and threading it through every caller would
// bury what these tests are actually about. The rule itself is covered directly
// by TestCreateRequiresSourceTypeForOrganics and friends.
func (f *submissionFixture) submit(t *testing.T, material string, kg float64) int64 {
	t.Helper()

	body := map[string]any{
		"material_type":    material,
		"estimated_qty_kg": kg,
		"location":         "Kilimani drop-off point",
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

// balance reads the recycler's current points balance via /me.
func (f *submissionFixture) balance(t *testing.T) int64 {
	t.Helper()

	recorder := doJSON(t, f.router, http.MethodGet, "/me", nil, f.userToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("/me: status %d", recorder.Code)
	}
	return int64(decode(t, recorder)["points_balance"].(float64))
}

func TestCreateSubmissionStartsPending(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
		"material_type":    "pet",
		"estimated_qty_kg": 4.5,
		"location":         "Kilimani drop-off point",
	}, f.userToken)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", recorder.Code, recorder.Body)
	}

	body := decode(t, recorder)
	if body["status"] != "pending" {
		t.Errorf("status = %v, want pending", body["status"])
	}
	if body["verified_qty_kg"] != nil {
		t.Errorf("verified_qty_kg = %v, want null", body["verified_qty_kg"])
	}
	if body["verified_at"] != nil {
		t.Errorf("verified_at = %v, want null", body["verified_at"])
	}
	if body["collector_id"] != nil {
		t.Errorf("collector_id = %v, want null", body["collector_id"])
	}
	if body["points_awarded"] != nil {
		t.Errorf("points_awarded = %v, want null before verification", body["points_awarded"])
	}
}

// TestCreateSubmissionIgnoresClientStatus enforces App Flow §3: the client
// triggers actions, the server owns transitions. A client that could post
// status=verified would mint points for itself.
func TestCreateSubmissionIgnoresClientStatus(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
		"material_type":    "pet",
		"estimated_qty_kg": 4.5,
		"status":           "verified",
		"verified_qty_kg":  1000,
	}, f.userToken)

	body := decode(t, recorder)
	if body["status"] != "pending" {
		t.Errorf("status = %v, want pending", body["status"])
	}
	if body["verified_qty_kg"] != nil {
		t.Errorf("verified_qty_kg = %v, want null", body["verified_qty_kg"])
	}
	if got := f.balance(t); got != 0 {
		t.Errorf("balance = %d, want 0 — points were credited on create", got)
	}
}

func TestCreateSubmissionValidation(t *testing.T) {
	f := newSubmissionFixture(t)

	cases := map[string]struct {
		body  map[string]any
		field string
	}{
		"unknown material": {
			body:  map[string]any{"material_type": "unobtanium", "estimated_qty_kg": 4.5},
			field: "material_type",
		},
		"missing material": {
			body:  map[string]any{"estimated_qty_kg": 4.5},
			field: "material_type",
		},
		"zero weight": {
			body:  map[string]any{"material_type": "pet", "estimated_qty_kg": 0},
			field: "estimated_qty_kg",
		},
		"negative weight": {
			body:  map[string]any{"material_type": "pet", "estimated_qty_kg": -3},
			field: "estimated_qty_kg",
		},
		"missing weight": {
			body:  map[string]any{"material_type": "pet"},
			field: "estimated_qty_kg",
		},
		"absurd weight": {
			body:  map[string]any{"material_type": "pet", "estimated_qty_kg": 99999},
			field: "estimated_qty_kg",
		},
	}

	for name, tc := range cases {
		recorder := doJSON(t, f.router, http.MethodPost, "/submissions", tc.body, f.userToken)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400; body %s", name, recorder.Code, recorder.Body)
			continue
		}
		detail, _ := decode(t, recorder)["error"].(map[string]any)
		fields, _ := detail["fields"].(map[string]any)
		if fields[tc.field] == nil {
			t.Errorf("%s: expected a %q field error, got %v", name, tc.field, fields)
		}
	}
}

func TestCreateSubmissionRequiresAuth(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
		"material_type":    "pet",
		"estimated_qty_kg": 4.5,
	}, "")

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

// TestVerifyCreditsPointsAndLedger is the Phase 2 deliverable in one test:
// verification credits the balance, writes the ledger, and stamps the submission.
func TestVerifyCreditsPointsAndLedger(t *testing.T) {
	f := newSubmissionFixture(t)
	id := f.submit(t, "pet", 4.5)

	// pet is seeded at 25 pts/kg, so 4.2kg → round(105.0) = 105.
	recorder := doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify", map[string]any{
		"verified_qty_kg": 4.2,
	}, f.collector)
	_ = id

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", recorder.Code, recorder.Body)
	}

	body := decode(t, recorder)
	if awarded, _ := body["points_awarded"].(float64); awarded != 105 {
		t.Errorf("points_awarded = %v, want 105", body["points_awarded"])
	}
	if balance, _ := body["points_balance"].(float64); balance != 105 {
		t.Errorf("points_balance = %v, want 105", body["points_balance"])
	}

	submission, _ := body["submission"].(map[string]any)
	if submission["status"] != "verified" {
		t.Errorf("status = %v, want verified", submission["status"])
	}
	if qty, _ := submission["verified_qty_kg"].(float64); qty != 4.2 {
		t.Errorf("verified_qty_kg = %v, want 4.2", submission["verified_qty_kg"])
	}
	if submission["verified_at"] == nil {
		t.Error("verified_at was not stamped")
	}
	if submission["collector_id"] == nil {
		t.Error("collector_id was not recorded")
	}

	// The balance the user actually sees.
	if got := f.balance(t); got != 105 {
		t.Errorf("/me balance = %d, want 105", got)
	}

	// The ledger is the source of truth; users.points_balance is a read cache.
	var entries, total int64
	if err := f.conn.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(points_delta), 0) FROM points_ledger WHERE submission_id = ?`,
		1,
	).Scan(&entries, &total); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if entries != 1 {
		t.Errorf("ledger has %d entries, want 1", entries)
	}
	if total != 105 {
		t.Errorf("ledger total = %d, want 105", total)
	}

	var reason string
	if err := f.conn.QueryRow(
		`SELECT reason FROM points_ledger WHERE submission_id = ?`, 1,
	).Scan(&reason); err != nil {
		t.Fatalf("read reason: %v", err)
	}
	if reason != "submission_verified" {
		t.Errorf("reason = %q, want submission_verified", reason)
	}
}

// TestVerifyRoundsPointsRatherThanTruncating guards against 4.98kg of PET being
// worth the same as 4.0kg.
func TestVerifyRoundsPointsRatherThanTruncating(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "aluminum", 1.0) // aluminum is 40 pts/kg

	// 1.019kg × 40 = 40.76 → 41, not 40.
	recorder := doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify", map[string]any{
		"verified_qty_kg": 1.019,
	}, f.collector)

	if awarded, _ := decode(t, recorder)["points_awarded"].(float64); awarded != 41 {
		t.Errorf("points_awarded = %v, want 41", decode(t, recorder)["points_awarded"])
	}
}

// TestVerifyCreditsOwnerNotCollector: the collector does the work, the recycler
// gets the points.
func TestVerifyCreditsOwnerNotCollector(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "pet", 4.0)

	doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify", map[string]any{
		"verified_qty_kg": 4.0,
	}, f.collector)

	var collectorBalance int64
	if err := f.conn.QueryRow(
		`SELECT points_balance FROM users WHERE role = 'collector'`,
	).Scan(&collectorBalance); err != nil {
		t.Fatalf("read collector balance: %v", err)
	}
	if collectorBalance != 0 {
		t.Errorf("collector balance = %d, want 0 — points went to the wrong account", collectorBalance)
	}
	if got := f.balance(t); got != 100 {
		t.Errorf("recycler balance = %d, want 100", got)
	}
}

// TestVerifyIsNotRepeatable is the double-credit guard. A second verify must be
// rejected and must not move the balance.
func TestVerifyIsNotRepeatable(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "pet", 4.0)

	first := doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify", map[string]any{
		"verified_qty_kg": 4.0,
	}, f.collector)
	if first.Code != http.StatusOK {
		t.Fatalf("first verify: status %d, body %s", first.Code, first.Body)
	}

	second := doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify", map[string]any{
		"verified_qty_kg": 4.0,
	}, f.collector)
	if second.Code != http.StatusConflict {
		t.Fatalf("second verify: status %d, want 409; body %s", second.Code, second.Body)
	}

	if got := f.balance(t); got != 100 {
		t.Errorf("balance = %d, want 100 — points were credited twice", got)
	}

	var entries int64
	if err := f.conn.QueryRow(
		`SELECT COUNT(*) FROM points_ledger WHERE submission_id = 1`,
	).Scan(&entries); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if entries != 1 {
		t.Errorf("ledger has %d entries, want 1", entries)
	}
}

// TestConcurrentVerifyCreditsExactlyOnce is the one that matters most: two
// collectors hitting verify at the same instant must produce one success, one
// conflict, and one ledger entry. A read-then-write guard would fail this.
func TestConcurrentVerifyCreditsExactlyOnce(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "pet", 4.0)

	const attempts = 8

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		conflicts int
		others    []int
	)

	start := make(chan struct{})
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all goroutines together
			recorder := doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify",
				map[string]any{"verified_qty_kg": 4.0}, f.collector)

			mu.Lock()
			defer mu.Unlock()
			switch recorder.Code {
			case http.StatusOK:
				successes++
			case http.StatusConflict:
				conflicts++
			default:
				others = append(others, recorder.Code)
			}
		}()
	}
	close(start)
	wg.Wait()

	if successes != 1 {
		t.Errorf("successes = %d, want exactly 1", successes)
	}
	if conflicts != attempts-1 {
		t.Errorf("conflicts = %d, want %d", conflicts, attempts-1)
	}
	if len(others) > 0 {
		t.Errorf("unexpected statuses: %v", others)
	}

	if got := f.balance(t); got != 100 {
		t.Errorf("balance = %d, want 100 — concurrent verifies double-credited", got)
	}

	var entries int64
	if err := f.conn.QueryRow(
		`SELECT COUNT(*) FROM points_ledger WHERE submission_id = 1`,
	).Scan(&entries); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if entries != 1 {
		t.Errorf("ledger has %d entries, want exactly 1", entries)
	}
}

// TestVerifyRequiresCollectorRole: a user must not be able to verify their own
// submission and award themselves points.
func TestVerifyRequiresCollectorRole(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "pet", 4.0)

	recorder := doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify", map[string]any{
		"verified_qty_kg": 4.0,
	}, f.userToken)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body %s", recorder.Code, recorder.Body)
	}
	if got := f.balance(t); got != 0 {
		t.Errorf("balance = %d, want 0 — a user self-credited", got)
	}
}

// TestVerifyAppliesCollectorMaterialCorrection covers the collector overriding a
// wrong material type: the points must use the corrected rate, not the submitted
// one. This is also the mechanism that keeps the AI advisory in Phase 2.5.
func TestVerifyAppliesCollectorMaterialCorrection(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "pet", 2.0) // claimed PET at 25 pts/kg

	// Collector finds it is actually aluminium, 40 pts/kg → 2kg = 80.
	recorder := doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify", map[string]any{
		"verified_qty_kg": 2.0,
		"material_type":   "aluminum",
	}, f.collector)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", recorder.Code, recorder.Body)
	}

	body := decode(t, recorder)
	if awarded, _ := body["points_awarded"].(float64); awarded != 80 {
		t.Errorf("points_awarded = %v, want 80", body["points_awarded"])
	}

	rate, _ := body["rate_applied"].(map[string]any)
	if rate["material_type"] != "aluminum" {
		t.Errorf("rate_applied.material_type = %v, want aluminum", rate["material_type"])
	}

	submission, _ := body["submission"].(map[string]any)
	if submission["material_type"] != "aluminum" {
		t.Errorf("submission material_type = %v, want aluminum", submission["material_type"])
	}
}

func TestVerifyRejectDoesNotCreditPoints(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "pet", 4.0)

	recorder := doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify", map[string]any{
		"status": "rejected",
	}, f.collector)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", recorder.Code, recorder.Body)
	}

	submission, _ := decode(t, recorder)["submission"].(map[string]any)
	if submission["status"] != "rejected" {
		t.Errorf("status = %v, want rejected", submission["status"])
	}
	if got := f.balance(t); got != 0 {
		t.Errorf("balance = %d, want 0 — a rejection credited points", got)
	}
}

// TestCollectedIsANonCreditingIntermediateStep covers the FR-5 lifecycle:
// pending → collected → verified, with points only at the end.
func TestCollectedIsANonCreditingIntermediateStep(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "pet", 4.0)

	collected := doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify", map[string]any{
		"status": "collected",
	}, f.collector)
	if collected.Code != http.StatusOK {
		t.Fatalf("collect: status %d, body %s", collected.Code, collected.Body)
	}
	submission, _ := decode(t, collected)["submission"].(map[string]any)
	if submission["status"] != "collected" {
		t.Errorf("status = %v, want collected", submission["status"])
	}
	if got := f.balance(t); got != 0 {
		t.Errorf("balance = %d, want 0 — collection credited points", got)
	}

	// Then verified from collected, which does credit.
	verified := doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify", map[string]any{
		"verified_qty_kg": 4.0,
	}, f.collector)
	if verified.Code != http.StatusOK {
		t.Fatalf("verify: status %d, body %s", verified.Code, verified.Body)
	}
	if got := f.balance(t); got != 100 {
		t.Errorf("balance = %d, want 100", got)
	}
}

// TestVerifiedSubmissionCannotBeRejected: points already credited (and possibly
// spent) must not be strippable by a later status change.
func TestVerifiedSubmissionCannotBeRejected(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "pet", 4.0)

	doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify",
		map[string]any{"verified_qty_kg": 4.0}, f.collector)

	recorder := doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify",
		map[string]any{"status": "rejected"}, f.collector)

	if recorder.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", recorder.Code)
	}
	if got := f.balance(t); got != 100 {
		t.Errorf("balance = %d, want 100", got)
	}
}

func TestVerifyValidation(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "pet", 4.0)

	cases := map[string]struct {
		body  map[string]any
		field string
	}{
		"missing weight":  {body: map[string]any{}, field: "verified_qty_kg"},
		"zero weight":     {body: map[string]any{"verified_qty_kg": 0}, field: "verified_qty_kg"},
		"negative weight": {body: map[string]any{"verified_qty_kg": -1}, field: "verified_qty_kg"},
		"bad status":      {body: map[string]any{"status": "sideways"}, field: "status"},
		"bad material": {
			body:  map[string]any{"verified_qty_kg": 4.0, "material_type": "unobtanium"},
			field: "material_type",
		},
	}

	for name, tc := range cases {
		recorder := doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify", tc.body, f.collector)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400; body %s", name, recorder.Code, recorder.Body)
			continue
		}
		detail, _ := decode(t, recorder)["error"].(map[string]any)
		fields, _ := detail["fields"].(map[string]any)
		if fields[tc.field] == nil {
			t.Errorf("%s: expected a %q field error, got %v", name, tc.field, fields)
		}
	}

	// Nothing above should have changed anything.
	if got := f.balance(t); got != 0 {
		t.Errorf("balance = %d, want 0", got)
	}
}

func TestVerifyUnknownSubmissionIsNotFound(t *testing.T) {
	f := newSubmissionFixture(t)

	for _, path := range []string{"/submissions/999/verify", "/submissions/abc/verify"} {
		recorder := doJSON(t, f.router, http.MethodPatch, path,
			map[string]any{"verified_qty_kg": 4.0}, f.collector)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, recorder.Code)
		}
	}
}

// TestGetSubmissionScopesToOwner: another user's submission answers exactly as a
// missing one does, so ids cannot be probed.
func TestGetSubmissionScopesToOwner(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "pet", 4.0)

	otherToken, _ := registerUser(t, f.router, "+254733333333", "Someone Else")

	mine := doJSON(t, f.router, http.MethodGet, "/submissions/1", nil, f.userToken)
	if mine.Code != http.StatusOK {
		t.Fatalf("owner: status %d, want 200", mine.Code)
	}

	theirs := doJSON(t, f.router, http.MethodGet, "/submissions/1", nil, otherToken)
	if theirs.Code != http.StatusNotFound {
		t.Errorf("other user: status %d, want 404", theirs.Code)
	}

	missing := doJSON(t, f.router, http.MethodGet, "/submissions/424242", nil, otherToken)
	if missing.Body.String() != theirs.Body.String() {
		t.Errorf("not-mine and not-found differ:\n  not mine: %s\n  missing:  %s",
			theirs.Body, missing.Body)
	}
}

// TestCollectorCanReadAnySubmission — the queue would be useless otherwise.
func TestCollectorCanReadAnySubmission(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "pet", 4.0)

	recorder := doJSON(t, f.router, http.MethodGet, "/submissions/1", nil, f.collector)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", recorder.Code, recorder.Body)
	}
	if decode(t, recorder)["user"] == nil {
		t.Error("submitter reference is missing — the collector cannot name who to meet")
	}
}

// TestListScopesUsersToTheirOwn: a user cannot widen the filter to see everyone.
func TestListScopesUsersToTheirOwn(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "pet", 4.0)

	otherToken, other := registerUser(t, f.router, "+254733333333", "Someone Else")
	otherID := int64(other["id"].(float64))

	recorder := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
		"material_type":    "cardboard",
		"estimated_qty_kg": 2.0,
	}, otherToken)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("other submit: %s", recorder.Body)
	}

	// Even asking explicitly for someone else's must not widen the scope.
	list := doJSON(t, f.router, http.MethodGet, "/submissions?user_id=1", nil, otherToken)
	if list.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", list.Code)
	}

	body := decode(t, list)
	if total, _ := body["total"].(float64); total != 1 {
		t.Errorf("total = %v, want 1", body["total"])
	}
	items, _ := body["submissions"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if userID, _ := item["user_id"].(float64); int64(userID) != otherID {
			t.Errorf("listing leaked submission from user %v", item["user_id"])
		}
	}
}

// TestListPendingIsTheCollectorQueue.
func TestListPendingIsTheCollectorQueue(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "pet", 4.0)
	f.submit(t, "cardboard", 3.0)
	f.submit(t, "food_waste", 12.0)

	// Verify one, so it should drop out of the pending queue.
	doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify",
		map[string]any{"verified_qty_kg": 4.0}, f.collector)

	pending := doJSON(t, f.router, http.MethodGet, "/submissions?status=pending", nil, f.collector)
	if pending.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", pending.Code, pending.Body)
	}

	body := decode(t, pending)
	if total, _ := body["total"].(float64); total != 2 {
		t.Errorf("pending total = %v, want 2", body["total"])
	}

	items, _ := body["submissions"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["status"] != "pending" {
			t.Errorf("non-pending submission in queue: %v", item["status"])
		}
	}

	verified := doJSON(t, f.router, http.MethodGet, "/submissions?status=verified", nil, f.collector)
	if total, _ := decode(t, verified)["total"].(float64); total != 1 {
		t.Errorf("verified total = %v, want 1", decode(t, verified)["total"])
	}
}

func TestListRejectsUnknownStatusFilter(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := doJSON(t, f.router, http.MethodGet, "/submissions?status=sideways", nil, f.collector)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", recorder.Code, recorder.Body)
	}
}

// TestListReturnsEmptyArrayNotNull — a client iterating the field should not have
// to special-case a missing list.
func TestListReturnsEmptyArrayNotNull(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := doJSON(t, f.router, http.MethodGet, "/submissions", nil, f.userToken)
	body := decode(t, recorder)

	items, ok := body["submissions"].([]any)
	if !ok {
		t.Fatalf("submissions is not an array: %s", recorder.Body)
	}
	if len(items) != 0 {
		t.Errorf("got %d items, want 0", len(items))
	}
	if total, _ := body["total"].(float64); total != 0 {
		t.Errorf("total = %v, want 0", body["total"])
	}
}

// TestPointsAwardedAppearsAfterVerification: the status screen reads this to show
// what a submission earned.
func TestPointsAwardedAppearsAfterVerification(t *testing.T) {
	f := newSubmissionFixture(t)
	f.submit(t, "food_waste", 20.0) // 2 pts/kg → 40

	doJSON(t, f.router, http.MethodPatch, "/submissions/1/verify",
		map[string]any{"verified_qty_kg": 20.0}, f.collector)

	recorder := doJSON(t, f.router, http.MethodGet, "/submissions/1", nil, f.userToken)
	body := decode(t, recorder)

	if awarded, _ := body["points_awarded"].(float64); awarded != 40 {
		t.Errorf("points_awarded = %v, want 40", body["points_awarded"])
	}
}

// TestCollectorMayNotCreateSubmission: a collector verifies what other people
// hand over and does not log its own recycling, so the app gives that account no
// Recycle tab. The server refuses the calls behind it rather than trusting the
// hidden tab, since a client is never the authority on what a role may do.
//
// Admin is asserted in the same test on purpose. It inherits collector
// everywhere else — `isCollector` and RequireRole both admit it — so if this
// denial ever started inheriting too, the one account that has to walk the whole
// demo would silently lose the submit flow. That regression would not show up in
// any collector-only assertion.
func TestCollectorMayNotCreateSubmission(t *testing.T) {
	f := newSubmissionFixture(t)

	adminToken, admin := registerUser(t, f.router, "+254712000004", "Zoa Operations")
	if _, err := f.conn.Exec(
		`UPDATE users SET role = 'admin' WHERE id = ?`,
		int64(admin["id"].(float64)),
	); err != nil {
		t.Fatalf("promote admin: %v", err)
	}

	body := map[string]any{
		"material_type":    "pet",
		"estimated_qty_kg": 4.2,
		"location":         "Kilimani drop-off point",
	}

	recorder := doJSON(t, f.router, http.MethodPost, "/submissions", body, f.collector)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("collector create = %d, want 403 — a collector must not log its own recycling",
			recorder.Code)
	}

	recorder = doJSON(t, f.router, http.MethodPost, "/submissions", body, adminToken)
	if recorder.Code != http.StatusCreated {
		t.Errorf("admin create = %d, want 201 — admin must keep every flow for the demo",
			recorder.Code)
	}
}

// TestCollectorMayNotClassify: classification is the suggestion step inside the
// submit flow, so it is denied to the same account that cannot submit. Left open
// it would be a live upload path on a screen that account can no longer reach.
func TestCollectorMayNotClassify(t *testing.T) {
	f := newSubmissionFixture(t)

	recorder := postPhoto(t, f, "pet_bottle_01.jpg", jpegBytes(t, 16, 16), f.collector)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("collector classify = %d, want 403", recorder.Code)
	}
}
