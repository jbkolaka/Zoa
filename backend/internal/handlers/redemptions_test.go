package handlers_test

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/models"
	"zoa/backend/internal/store"
)

// Seeded vouchers referenced by title rather than by id, so these tests do not
// depend on 004_seed_partners_vouchers.sql having produced a particular insert
// order.
const (
	// 150 points, KSh 100 off, 30 days, stock 200.
	voucherCheap = "KSh 100 off your shopping"

	// 420 points, 10% off, 30 days, stock 50 — the percentage-discount case.
	voucherPercent = "10% off your basket"

	// 200 points, 60 days, stock NULL — the unlimited-stock case.
	voucherUnlimited = "Free tea or coffee"

	// active = 0, so it must be unredeemable.
	voucherInactive = "Expired launch promo"
)

// redemptionFixture is a recycler, a partner cashier, and their tokens.
type redemptionFixture struct {
	router *gin.Engine
	conn   *sql.DB

	userToken string
	userID    int64

	partnerToken string
	partnerID    int64
}

func newRedemptionFixture(t *testing.T) *redemptionFixture {
	t.Helper()

	router, conn := newTestRouterWithDB(t)

	userToken, user := registerUser(t, router, "+254712345678", "Amina Wanjiru")
	partnerToken, partner := registerUser(t, router, "+254712000003", "Naivas Till 4")

	partnerID := int64(partner["id"].(float64))

	// register only ever creates role `user`, so the cashier is promoted directly —
	// the same thing an admin would do in the real product.
	if _, err := conn.Exec(
		`UPDATE users SET role = ? WHERE id = ?`, models.RolePartnerStaff, partnerID,
	); err != nil {
		t.Fatalf("promote partner staff: %v", err)
	}

	return &redemptionFixture{
		router:       router,
		conn:         conn,
		userToken:    userToken,
		userID:       int64(user["id"].(float64)),
		partnerToken: partnerToken,
		partnerID:    partnerID,
	}
}

// credit grants the recycler points.
//
// Both halves of a real credit are written — the ledger row and the balance
// cache — because the invariant tests below assert the two agree, and a fixture
// that moved only the balance would make that check vacuous.
func (f *redemptionFixture) credit(t *testing.T, points int64) {
	t.Helper()

	tx, err := f.conn.Begin()
	if err != nil {
		t.Fatalf("begin credit: %v", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO points_ledger (user_id, points_delta, reason) VALUES (?, ?, ?)`,
		f.userID, points, models.ReasonSubmissionVerified,
	); err != nil {
		t.Fatalf("insert ledger: %v", err)
	}
	if _, err := tx.Exec(
		`UPDATE users SET points_balance = points_balance + ? WHERE id = ?`,
		points, f.userID,
	); err != nil {
		t.Fatalf("update balance: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit credit: %v", err)
	}
}

// voucherID resolves a seeded voucher by title.
func (f *redemptionFixture) voucherID(t *testing.T, title string) int64 {
	t.Helper()

	var id int64
	if err := f.conn.QueryRow(
		`SELECT id FROM vouchers WHERE title = ?`, title,
	).Scan(&id); err != nil {
		t.Fatalf("look up voucher %q: %v", title, err)
	}
	return id
}

// balance reads the recycler's current balance via /me, so the test sees what a
// client would rather than reading the column directly.
func (f *redemptionFixture) balance(t *testing.T) int64 {
	t.Helper()

	recorder := doJSON(t, f.router, http.MethodGet, "/me", nil, f.userToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("/me: status %d", recorder.Code)
	}
	return int64(decode(t, recorder)["points_balance"].(float64))
}

// redeem spends points on a voucher, failing the test if it does not succeed.
func (f *redemptionFixture) redeem(t *testing.T, title string) map[string]any {
	t.Helper()

	recorder := doJSON(t, f.router, http.MethodPost, "/redemptions",
		map[string]any{"voucher_id": f.voucherID(t, title)}, f.userToken)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("redeem %q: status %d, want 201; body %s", title, recorder.Code, recorder.Body)
	}
	return decode(t, recorder)
}

// stockOf reads a voucher's remaining stock. The pointer distinguishes an
// unlimited voucher (NULL) from a sold-out one (0).
func (f *redemptionFixture) stockOf(t *testing.T, title string) *int64 {
	t.Helper()

	var stock *int64
	if err := f.conn.QueryRow(
		`SELECT stock_remaining FROM vouchers WHERE title = ?`, title,
	).Scan(&stock); err != nil {
		t.Fatalf("read stock of %q: %v", title, err)
	}
	return stock
}

func (f *redemptionFixture) statusOf(t *testing.T, code string) string {
	t.Helper()

	var status string
	if err := f.conn.QueryRow(
		`SELECT status FROM redemptions WHERE redemption_code = ?`, code,
	).Scan(&status); err != nil {
		t.Fatalf("read status of %q: %v", code, err)
	}
	return status
}

func (f *redemptionFixture) countRedemptions(t *testing.T) int64 {
	t.Helper()

	var count int64
	if err := f.conn.QueryRow(`SELECT COUNT(*) FROM redemptions`).Scan(&count); err != nil {
		t.Fatalf("count redemptions: %v", err)
	}
	return count
}

// --- POST /redemptions ---

func TestRedeemIssuesCodeAndDeductsPoints(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 500)

	body := f.redeem(t, voucherCheap)

	code, _ := body["code"].(string)
	// A v4 UUID in canonical form: 36 characters, four dashes. The exact format
	// matters because the code is read aloud and retyped at a till.
	if len(code) != 36 || strings.Count(code, "-") != 4 {
		t.Errorf("code = %q, want a canonical 36-character UUID", code)
	}

	if want := store.QRPayloadPrefix + code; body["qr_payload"] != want {
		t.Errorf("qr_payload = %v, want %q", body["qr_payload"], want)
	}

	if got := int64(body["points_spent"].(float64)); got != 150 {
		t.Errorf("points_spent = %d, want 150", got)
	}
	if got := int64(body["points_balance"].(float64)); got != 350 {
		t.Errorf("points_balance = %d, want 350", got)
	}

	redemption := body["redemption"].(map[string]any)
	if redemption["status"] != models.RedemptionIssued {
		t.Errorf("status = %v, want %q", redemption["status"], models.RedemptionIssued)
	}
	// Nullable columns must serialise as null, never as a zero value — "not yet
	// used" has to stay distinguishable from "used by user 0".
	if redemption["used_at"] != nil {
		t.Errorf("used_at = %v, want null on a fresh code", redemption["used_at"])
	}
	if redemption["verified_by"] != nil {
		t.Errorf("verified_by = %v, want null on a fresh code", redemption["verified_by"])
	}

	// Expiry is derived from issued_at and the voucher's expiry_days (30 for this
	// voucher), never stored.
	issuedAt := parseTime(t, redemption["issued_at"])
	expiry := parseTime(t, body["expiry"])
	if want := issuedAt.AddDate(0, 0, 30); !expiry.Equal(want) {
		t.Errorf("expiry = %s, want %s (issued_at + 30 days)", expiry, want)
	}

	// The ledger is the source of truth; the spend must be recorded there as a
	// negative delta tied to this redemption.
	var delta int64
	var reason string
	var redemptionID *int64
	if err := f.conn.QueryRow(
		`SELECT points_delta, reason, redemption_id FROM points_ledger
		  WHERE points_delta < 0`,
	).Scan(&delta, &reason, &redemptionID); err != nil {
		t.Fatalf("read spend ledger row: %v", err)
	}
	if delta != -150 {
		t.Errorf("points_delta = %d, want -150", delta)
	}
	if reason != models.ReasonVoucherRedeemed {
		t.Errorf("reason = %q, want %q", reason, models.ReasonVoucherRedeemed)
	}
	if redemptionID == nil {
		t.Error("redemption_id is null — the spend is not traceable to a code")
	}

	if got := f.balance(t); got != 350 {
		t.Errorf("balance = %d, want 350", got)
	}
	if got := f.stockOf(t, voucherCheap); got == nil || *got != 199 {
		t.Errorf("stock = %v, want 199", got)
	}
}

func TestRedeemRefusesInsufficientPoints(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 100) // the voucher costs 150

	recorder := doJSON(t, f.router, http.MethodPost, "/redemptions",
		map[string]any{"voucher_id": f.voucherID(t, voucherCheap)}, f.userToken)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body %s", recorder.Code, recorder.Body)
	}

	// The contract requires this token in the message, so a client can tell it
	// apart from the out-of-stock 409 on the same endpoint.
	message := decode(t, recorder)["error"].(map[string]any)["message"].(string)
	if !strings.Contains(message, "insufficient_points") {
		t.Errorf("message = %q, want it to contain insufficient_points", message)
	}

	if got := f.balance(t); got != 100 {
		t.Errorf("balance = %d, want 100 — a refused redemption must not deduct", got)
	}
	if got := f.countRedemptions(t); got != 0 {
		t.Errorf("%d redemptions exist, want 0 — no code should have been issued", got)
	}
}

func TestRedeemRefusesInactiveVoucher(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 500)

	recorder := doJSON(t, f.router, http.MethodPost, "/redemptions",
		map[string]any{"voucher_id": f.voucherID(t, voucherInactive)}, f.userToken)

	// 404 rather than 409: an inactive offer answers exactly as a nonexistent one
	// does, so it cannot be found by probing ids.
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body %s", recorder.Code, recorder.Body)
	}
	if got := f.balance(t); got != 500 {
		t.Errorf("balance = %d, want 500", got)
	}
}

// TestRedeemRollsBackWhenStockRunsOut is really a transaction test: the
// deduction happens *before* the stock check, so a 409 here can only be correct
// if the whole transaction rolled back.
func TestRedeemRollsBackWhenStockRunsOut(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 500)

	if _, err := f.conn.Exec(
		`UPDATE vouchers SET stock_remaining = 0 WHERE title = ?`, voucherCheap,
	); err != nil {
		t.Fatalf("empty the stock: %v", err)
	}

	recorder := doJSON(t, f.router, http.MethodPost, "/redemptions",
		map[string]any{"voucher_id": f.voucherID(t, voucherCheap)}, f.userToken)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body %s", recorder.Code, recorder.Body)
	}
	if got := f.balance(t); got != 500 {
		t.Errorf("balance = %d, want 500 — the deduction was not rolled back", got)
	}
	if got := f.countRedemptions(t); got != 0 {
		t.Errorf("%d redemptions exist, want 0", got)
	}
}

// TestRedeemLeavesUnlimitedStockUnlimited pins the NULL - 1 = NULL behaviour the
// single stock statement relies on. Decrementing an unlimited voucher to 0 would
// silently retire it after one redemption.
func TestRedeemLeavesUnlimitedStockUnlimited(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 500)

	f.redeem(t, voucherUnlimited)

	if got := f.stockOf(t, voucherUnlimited); got != nil {
		t.Errorf("stock = %d, want NULL — unlimited must stay unlimited", *got)
	}
}

func TestRedeemRequiresAVoucherID(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 500)

	recorder := doJSON(t, f.router, http.MethodPost, "/redemptions",
		map[string]any{}, f.userToken)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", recorder.Code, recorder.Body)
	}
	fields := decode(t, recorder)["error"].(map[string]any)["fields"].(map[string]any)
	if fields["voucher_id"] == nil {
		t.Errorf("no voucher_id field error: %v", fields)
	}
}

// TestConcurrentRedeemDeductsExactlyOnce is the one that matters most on this
// endpoint: a user with exactly one voucher's worth of points who taps redeem
// eight times must end up with one code and a zero balance, never a negative one.
func TestConcurrentRedeemDeductsExactlyOnce(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 150) // exactly the cost of one voucherCheap

	voucherID := f.voucherID(t, voucherCheap)

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
			recorder := doJSON(t, f.router, http.MethodPost, "/redemptions",
				map[string]any{"voucher_id": voucherID}, f.userToken)

			mu.Lock()
			defer mu.Unlock()
			switch recorder.Code {
			case http.StatusCreated:
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

	if got := f.balance(t); got != 0 {
		t.Errorf("balance = %d, want 0 — concurrent redeems double-spent", got)
	}
	if got := f.countRedemptions(t); got != 1 {
		t.Errorf("%d redemptions issued, want exactly 1", got)
	}

	var spends int64
	if err := f.conn.QueryRow(
		`SELECT COUNT(*) FROM points_ledger WHERE points_delta < 0`,
	).Scan(&spends); err != nil {
		t.Fatalf("count spend rows: %v", err)
	}
	if spends != 1 {
		t.Errorf("ledger has %d spend rows, want exactly 1", spends)
	}

	if got := f.stockOf(t, voucherCheap); got == nil || *got != 199 {
		t.Errorf("stock = %v, want 199 — stock moved more than once", got)
	}
}

// --- GET /redemptions ---

func TestRedemptionListIsOwnerScopedAndEmbedsVoucher(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 500)
	f.redeem(t, voucherCheap)

	recorder := doJSON(t, f.router, http.MethodGet, "/redemptions", nil, f.userToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", recorder.Code, recorder.Body)
	}

	items := decode(t, recorder)["redemptions"].([]any)
	if len(items) != 1 {
		t.Fatalf("got %d redemptions, want 1", len(items))
	}

	// The voucher and its partner travel with each item so "My Redemptions"
	// renders in one call.
	item := items[0].(map[string]any)
	voucher, ok := item["voucher"].(map[string]any)
	if !ok {
		t.Fatalf("no voucher embedded in the listing: %v", item)
	}
	if voucher["title"] != voucherCheap {
		t.Errorf("voucher title = %v, want %q", voucher["title"], voucherCheap)
	}
	if partner, ok := voucher["partner"].(map[string]any); !ok || partner["name"] == "" {
		t.Errorf("no partner embedded in the voucher: %v", voucher)
	}
	if item["qr_payload"] == nil || item["expiry"] == nil {
		t.Errorf("listing is missing the derived qr_payload/expiry: %v", item)
	}

	// A redemption code is bearer-like — anyone who can read it can spend it — so
	// another account's history must be invisible even though both are signed in.
	otherToken, _ := registerUser(t, f.router, "+254799999999", "Someone Else")
	recorder = doJSON(t, f.router, http.MethodGet, "/redemptions", nil, otherToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("other user status = %d, want 200", recorder.Code)
	}
	if others := decode(t, recorder)["redemptions"].([]any); len(others) != 0 {
		t.Errorf("another user sees %d redemptions, want 0", len(others))
	}
}

func TestRedemptionListIsEmptyArrayNotNull(t *testing.T) {
	f := newRedemptionFixture(t)

	recorder := doJSON(t, f.router, http.MethodGet, "/redemptions", nil, f.userToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	// The client iterates this without a null check.
	if !strings.Contains(recorder.Body.String(), `"redemptions":[]`) {
		t.Errorf("empty history did not serialise as []: %s", recorder.Body)
	}
}

// --- POST /redemptions/:code/verify ---

func TestVerifyMarksCodeUsed(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 500)

	code := f.redeem(t, voucherPercent)["code"].(string)

	recorder := doJSON(t, f.router, http.MethodPost,
		"/redemptions/"+code+"/verify", nil, f.partnerToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", recorder.Code, recorder.Body)
	}

	body := decode(t, recorder)
	if body["status"] != models.RedemptionUsed {
		t.Errorf("status = %v, want %q", body["status"], models.RedemptionUsed)
	}

	redemption := body["redemption"].(map[string]any)
	if redemption["used_at"] == nil {
		t.Error("used_at is null after a successful verify")
	}
	if got := int64(redemption["verified_by"].(float64)); got != f.partnerID {
		t.Errorf("verified_by = %d, want %d", got, f.partnerID)
	}

	// The cashier is confirming a person, so the owner's name comes back with it.
	if user := body["user"].(map[string]any); user["name"] != "Amina Wanjiru" {
		t.Errorf("user.name = %v, want Amina Wanjiru", user["name"])
	}

	// The message has to tell the cashier what to actually do.
	message := body["message"].(string)
	if !strings.Contains(message, "10% off") {
		t.Errorf("message = %q, want it to name the discount", message)
	}

	if got := f.statusOf(t, code); got != models.RedemptionUsed {
		t.Errorf("stored status = %q, want %q", got, models.RedemptionUsed)
	}
}

func TestVerifyRefusesASecondUse(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 500)

	code := f.redeem(t, voucherCheap)["code"].(string)
	path := "/redemptions/" + code + "/verify"

	if recorder := doJSON(t, f.router, http.MethodPost, path, nil, f.partnerToken); recorder.Code != http.StatusOK {
		t.Fatalf("first verify: status %d, want 200; body %s", recorder.Code, recorder.Body)
	}

	var firstUsedAt string
	if err := f.conn.QueryRow(
		`SELECT used_at FROM redemptions WHERE redemption_code = ?`, code,
	).Scan(&firstUsedAt); err != nil {
		t.Fatalf("read used_at: %v", err)
	}

	recorder := doJSON(t, f.router, http.MethodPost, path, nil, f.partnerToken)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("second verify: status %d, want 409; body %s", recorder.Code, recorder.Body)
	}

	message := decode(t, recorder)["error"].(map[string]any)["message"].(string)
	if !strings.Contains(message, "already used") {
		t.Errorf("message = %q, want it to say the code was already used", message)
	}

	// The second attempt must not have moved the record — used_at is the audit
	// trail of when the discount was actually given.
	var secondUsedAt string
	if err := f.conn.QueryRow(
		`SELECT used_at FROM redemptions WHERE redemption_code = ?`, code,
	).Scan(&secondUsedAt); err != nil {
		t.Fatalf("re-read used_at: %v", err)
	}
	if secondUsedAt != firstUsedAt {
		t.Errorf("used_at moved from %q to %q on a refused verify", firstUsedAt, secondUsedAt)
	}
}

// TestConcurrentVerifyUsesCodeExactlyOnce is the anti-double-spend guarantee the
// demo rests on: one code presented at two tills at the same instant is accepted
// exactly once.
func TestConcurrentVerifyUsesCodeExactlyOnce(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 500)

	code := f.redeem(t, voucherCheap)["code"].(string)
	path := "/redemptions/" + code + "/verify"

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
			<-start
			recorder := doJSON(t, f.router, http.MethodPost, path, nil, f.partnerToken)

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
		t.Errorf("successes = %d, want exactly 1 — the code was accepted more than once", successes)
	}
	if conflicts != attempts-1 {
		t.Errorf("conflicts = %d, want %d", conflicts, attempts-1)
	}
	if len(others) > 0 {
		t.Errorf("unexpected statuses: %v", others)
	}

	if got := f.statusOf(t, code); got != models.RedemptionUsed {
		t.Errorf("stored status = %q, want %q", got, models.RedemptionUsed)
	}
}

// TestVerifyExpiredCodeTransitionsIt covers the contract's one case where a
// failed request still writes: a stale code is moved to `expired` rather than
// left reading as `issued` forever.
func TestVerifyExpiredCodeTransitionsIt(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 500)

	// voucherCheap is valid for 30 days, so an issue date 60 days ago is well past
	// expiry regardless of when this test runs.
	code := f.redeem(t, voucherCheap)["code"].(string)
	if _, err := f.conn.Exec(
		`UPDATE redemptions SET issued_at = datetime('now', '-60 days')
		  WHERE redemption_code = ?`, code,
	); err != nil {
		t.Fatalf("backdate issued_at: %v", err)
	}

	path := "/redemptions/" + code + "/verify"
	recorder := doJSON(t, f.router, http.MethodPost, path, nil, f.partnerToken)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body %s", recorder.Code, recorder.Body)
	}

	message := decode(t, recorder)["error"].(map[string]any)["message"].(string)
	if !strings.Contains(message, "expired") {
		t.Errorf("message = %q, want it to say the code expired", message)
	}

	if got := f.statusOf(t, code); got != models.RedemptionExpired {
		t.Errorf("stored status = %q, want %q — the row was not transitioned",
			got, models.RedemptionExpired)
	}

	// And it stays refused on a second look, now via the stored status rather than
	// the date comparison.
	recorder = doJSON(t, f.router, http.MethodPost, path, nil, f.partnerToken)
	if recorder.Code != http.StatusConflict {
		t.Errorf("second verify: status %d, want 409", recorder.Code)
	}
}

func TestVerifyUnknownCodeIsNotFound(t *testing.T) {
	f := newRedemptionFixture(t)

	recorder := doJSON(t, f.router, http.MethodPost,
		"/redemptions/7f3c1a92-4b0e-4c7d-9e21-8a5f0b6d1c34/verify", nil, f.partnerToken)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body %s", recorder.Code, recorder.Body)
	}
}

// TestVerifyRequiresPartnerRole: a recycler must not be able to mark their own
// code used, which would let one code be presented at two tills.
func TestVerifyRequiresPartnerRole(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 500)

	code := f.redeem(t, voucherCheap)["code"].(string)

	recorder := doJSON(t, f.router, http.MethodPost,
		"/redemptions/"+code+"/verify", nil, f.userToken)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body %s", recorder.Code, recorder.Body)
	}
	if got := f.statusOf(t, code); got != models.RedemptionIssued {
		t.Errorf("stored status = %q, want it untouched at %q", got, models.RedemptionIssued)
	}
}

// TestAdminCanVerify: RequireRole admits admin implicitly, so one admin login can
// drive the whole demo without also holding a partner_staff account.
func TestAdminCanVerify(t *testing.T) {
	f := newRedemptionFixture(t)
	f.credit(t, 500)

	code := f.redeem(t, voucherCheap)["code"].(string)

	adminToken, admin := registerUser(t, f.router, "+254712000004", "Zoa Operations")
	if _, err := f.conn.Exec(`UPDATE users SET role = ? WHERE id = ?`,
		models.RoleAdmin, int64(admin["id"].(float64))); err != nil {
		t.Fatalf("promote admin: %v", err)
	}

	recorder := doJSON(t, f.router, http.MethodPost,
		"/redemptions/"+code+"/verify", nil, adminToken)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", recorder.Code, recorder.Body)
	}
}

// --- the whole loop ---

// TestLedgerMatchesBalanceAcrossEarnAndSpend walks the full path the demo rests
// on — submit → verify → points → redeem — and asserts the invariant that holds
// the whole design together: SUM(points_delta) equals points_balance.
func TestLedgerMatchesBalanceAcrossEarnAndSpend(t *testing.T) {
	f := newRedemptionFixture(t)

	// Earn for real rather than seeding a balance: PET pays 25/kg, so 8 kg is 200
	// points — exactly the cost of voucherUnlimited, which lands the balance on a
	// clean zero.
	collectorToken, collector := registerUser(t, f.router, "+254712000002", "Joseph Kariuki")
	if _, err := f.conn.Exec(`UPDATE users SET role = ? WHERE id = ?`,
		models.RoleCollector, int64(collector["id"].(float64))); err != nil {
		t.Fatalf("promote collector: %v", err)
	}

	recorder := doJSON(t, f.router, http.MethodPost, "/submissions", map[string]any{
		"material_type":    "pet",
		"estimated_qty_kg": 8.0,
	}, f.userToken)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("submit: status %d, body %s", recorder.Code, recorder.Body)
	}
	submissionID := int64(decode(t, recorder)["id"].(float64))

	recorder = doJSON(t, f.router, http.MethodPatch,
		"/submissions/"+itoa(submissionID)+"/verify",
		map[string]any{"verified_qty_kg": 8.0}, collectorToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("verify: status %d, body %s", recorder.Code, recorder.Body)
	}
	if got := f.balance(t); got != 200 {
		t.Fatalf("balance after earning = %d, want 200", got)
	}

	code := f.redeem(t, voucherUnlimited)["code"].(string)

	if got := f.balance(t); got != 0 {
		t.Errorf("balance after redeeming = %d, want 0", got)
	}

	var ledgerSum, storedBalance int64
	if err := f.conn.QueryRow(
		`SELECT COALESCE(SUM(points_delta), 0) FROM points_ledger WHERE user_id = ?`,
		f.userID,
	).Scan(&ledgerSum); err != nil {
		t.Fatalf("sum ledger: %v", err)
	}
	if err := f.conn.QueryRow(
		`SELECT points_balance FROM users WHERE id = ?`, f.userID,
	).Scan(&storedBalance); err != nil {
		t.Fatalf("read balance: %v", err)
	}
	if ledgerSum != storedBalance {
		t.Errorf("ledger sums to %d but points_balance is %d — the two have drifted",
			ledgerSum, storedBalance)
	}

	// And the code the loop produced is spendable exactly once.
	path := "/redemptions/" + code + "/verify"
	if recorder := doJSON(t, f.router, http.MethodPost, path, nil, f.partnerToken); recorder.Code != http.StatusOK {
		t.Fatalf("code-verify: status %d, want 200; body %s", recorder.Code, recorder.Body)
	}
	if recorder := doJSON(t, f.router, http.MethodPost, path, nil, f.partnerToken); recorder.Code != http.StatusConflict {
		t.Errorf("re-verify: status %d, want 409", recorder.Code)
	}
}

// --- helpers ---

// parseTime reads an RFC3339 timestamp out of a decoded JSON body.
func parseTime(t *testing.T, raw any) time.Time {
	t.Helper()

	text, ok := raw.(string)
	if !ok {
		t.Fatalf("expected a timestamp string, got %v", raw)
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatalf("parse %q: %v", text, err)
	}
	return parsed
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}
