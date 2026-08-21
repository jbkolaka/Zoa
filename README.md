# Zoa

Recycling rewards for Kenyan households and hotels. Recycle your waste, a collector verifies it, you earn points, and points become real discounts at partner retailers — no cash handling, no payout risk.

```
zoa-folder/
├── backend/     Go API + SQLite (the whole server)
├── app/         Flutter client
└── docs/        Specs, and the approved visual design system
```

## Build order

Phases come from `docs/07_Implementation_Plan.md` and are built strictly in sequence.

| Phase | Scope | State |
|---|---|---|
| 0 | Setup: scaffolds, schema, API contract, theme, health check | **done** |
| 1 | Auth & user core | **done** |
| 2 | Submission flow | **done** |
| 2.5 | AI material classification | **done** |
| 3 | Voucher catalog | **done** |
| 4 | Redemption & verification | **done** |
| 5 | Admin & polish | backend done · screen + polish outstanding |

The one thing that must always work end to end: **submission → verify → points → redeem → code-verify.**

> **Outstanding verification:** the Go side is fully verified — `gofmt`, `go vet` and
> `go test ./...` all green across 110 handler tests, including both 8-way concurrency
> tests. On the Dart side `flutter analyze` was clean at the end of Phase 4, but
> Phase 5's admin screen was added after that and has **not been analyzed**, and
> `flutter test` has **never run** at all. Run both before trusting the client.

## Running the backend

Requires Go 1.22+. No CGO, no database server — the SQLite driver is pure Go and migrations run on boot.

```sh
cd backend
cp .env.example .env      # optional; defaults work for dev
go run ./cmd/api          # listens on :8080
```

```sh
curl localhost:8080/health   # service + database state
curl localhost:8080/meta     # material taxonomy and points rates
```

### Demo accounts

Registration only ever creates role `user`, so the collector, partner and admin
accounts a walkthrough needs are seeded explicitly:

```sh
go run ./cmd/api -seed-demo
```

| Phone | Name | Role |
|---|---|---|
| `+254712000001` | Amina Wanjiru | `user` |
| `+254712000002` | Joseph Kariuki | `collector` |
| `+254712000003` | Naivas Till 4 | `partner_staff` |
| `+254712000004` | Zoa Operations | `admin` |

Shared password: `zoa1234`. Seeding is idempotent and never modifies an existing
account, so it is safe to re-run between rehearsals. Admin inherits every lower
role's access, so one admin login can drive the whole flow if needed.

Other tasks (`make help` lists them all):

```sh
make migrate    # apply migrations and exit
make inspect    # dump schema + row counts (no sqlite3 CLI needed)
make test
make docker-build && make docker-run
```

## AI material classification (Phase 2.5)

`POST /submissions/classify` takes a photo (multipart field `photo`, JPEG/PNG,
≤ 8 MB) and returns a predicted taxonomy category with a confidence score. It
**never blocks a submission** (FR-23): a missing key, a timeout, a refusal or a
garbled answer all return `200` with `"degraded": true`, and the client falls
through to manual material selection.

```sh
curl -F photo=@pet_bottle.jpg -H "Authorization: Bearer $TOKEN" \
     localhost:8080/submissions/classify
```

Three environment variables control it (all optional — see `.env.example`):

| Variable | Default | Meaning |
|---|---|---|
| `ZOA_CLASSIFY_PROVIDER` | auto | `claude`, `mock`, or `off`. Unset picks `claude` when `ANTHROPIC_API_KEY` is present, else `mock` |
| `ZOA_CLASSIFY_MODEL` | `claude-opus-5` | Vision model id |
| `ZOA_CLASSIFY_TIMEOUT` | `3s` | Per-call budget (TRD §3). Exceeding it degrades |

**`mock` is a first-class provider, not a stub.** The implementation plan calls
for a working-but-simplified AI step rather than cutting the feature if the
vision path is unavailable, so the mock predicts from the filename first
(`pet_bottle_01.jpg` → `pet`) and otherwise deterministically from a content
hash. Name your rehearsal photos after their categories and every run predicts
correctly, offline, with no key. Tests always use it, so `go test` never makes a
billed API call — reaching the network requires an explicit `claude`.

Two things to know before pointing this at the real API:

- **`ANTHROPIC_BASE_URL` silently reroutes traffic.** The SDK honours it, so a
  value left in the environment sends every photo to that host instead of to
  Anthropic. Check it before a demo — waste photos are user data.
- **Predictions are advisory and never auto-confirm.** `material_type` is what
  the user confirmed and what points are calculated from; `predicted_category`
  is stored beside it and is deliberately *not* overwritten when a collector
  corrects the material, because that disagreement is the accuracy metric
  (FR-22). Organic submissions must also declare `source_type`
  (`residential` / `hotel`, FR-24).

### Testing the client against a real backend

Two files in `app/test/` check the one thing neither test suite can see on its
own — that the Dart models parse exactly what the Go handlers emit:

- `api_client_classify_test.dart` — that Dart's multipart encoding and Go's
  `FormFile` agree.
- `api_client_redemptions_test.dart` — the whole loop through the real client:
  submit → collector-verify → points → redeem → code-verify → refuse the second
  attempt. It earns its own points rather than assuming a balance, so it is safe
  to re-run against a long-lived demo database.

Both skip themselves when no server is listening, so they are safe in a normal
`flutter test` run.

```sh
cd backend && DB_PATH=/tmp/zoa-e2e.db PORT=8097 \
  ZOA_CLASSIFY_PROVIDER=mock go run ./cmd/api -seed-demo
cd app && flutter test          # integration checks, or a skip notice
```

`test/redemption_confirmation_test.dart` needs no server: it renders the
redemption screen at 320dp and 1.4× text and asserts nothing overflows and the QR
actually paints. It is the only place a Zoa screen is laid out rather than merely
analysed — though see the Phase 4 note above: it has not been run yet, so treat it
as unproven until it has.

## Voucher catalogue (Phase 3)

`GET /vouchers` returns the catalogue with each partner embedded and
affordability resolved server-side against the caller's live balance:

```sh
curl -H "Authorization: Bearer $TOKEN" localhost:8080/vouchers
curl -H "Authorization: Bearer $TOKEN" 'localhost:8080/vouchers?affordable=true'
curl -H "Authorization: Bearer $TOKEN" 'localhost:8080/vouchers?partner_id=2'
curl -H "Authorization: Bearer $TOKEN" localhost:8080/partners
```

Migration `004` seeds three partners (Naivas, Quickmart, Java House) and seven
vouchers from 150 to 900 points. Point costs are set against real earning rates —
PET pays 25/kg, so the cheapest voucher is about 6 kg of bottles — because a
reward that reads as unreachable stops motivating.

Three rules the catalogue enforces, all of which Phase 4 will depend on:

- **`affordable` is computed server-side**, never in the client. Phase 4 deducts
  points against the same comparison, so a client that decided for itself would
  eventually enable a button the server refuses.
- **Inactive and out-of-stock vouchers are invisible**, and an inactive one 404s
  identically to a nonexistent id — so an offer that is not honourable cannot be
  found by probing. `stock_remaining` NULL means *unlimited*, not zero.
- **A voucher is only listable if its partner is also active**, so a retailer
  leaving the programme takes its whole offer list with it.

## Redemption & verification (Phase 4)

The other half of the loop: points become a code, and a partner accepts it once.

```sh
# spend points — one atomic transaction, server-side
curl -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
     -d '{"voucher_id":1}' localhost:8080/redemptions

# the caller's own codes, newest first, voucher and partner embedded
curl -H "Authorization: Bearer $TOKEN" localhost:8080/redemptions

# the till end — partner_staff or admin only
curl -X POST -H "Authorization: Bearer $PARTNER_TOKEN" \
     localhost:8080/redemptions/$CODE/verify
```

`POST /redemptions` is a single transaction: check balance → deduct → decrement
stock → generate a UUID → insert the redemption → write the ledger entry. Either
all of it lands or none does, so there is no state in which points are gone and no
code exists. Two guards carry the weight, both the compare-and-swap pattern from
`submission_store.go`:

- **Points:** `UPDATE users SET points_balance = points_balance - ? WHERE id = ?
  AND points_balance >= ?`. Eight simultaneous redemptions by a user who can
  afford exactly one produce one code and a zero balance, never a negative one.
- **The code:** `UPDATE redemptions SET status='used' … WHERE redemption_code = ?
  AND status='issued'`. One code presented at two tills at the same instant is
  accepted exactly once — the anti-double-spend guarantee the demo rests on
  (FR-16). Both cases have a concurrency test that fires 8 requests at once.

Four things worth knowing:

- **`expiry` is derived, never stored** (`issued_at + voucher.expiry_days`). The
  schema has no expiry column, and deriving it stops an admin editing
  `expiry_days` from retroactively rewriting codes already in users' hands.
- **Stock uses one statement for both cases.** `stock_remaining` NULL means
  unlimited, and SQLite evaluates `NULL - 1` as `NULL`, so an unlimited voucher
  stays unlimited rather than quietly retiring after one redemption.
- **A verify past expiry both fails and writes:** the row is transitioned to
  `expired` and the request answers `409`, so a stale code stops reading as
  `issued` the first time anyone looks at it.
- **An issued code stays verifiable even if its voucher is later deactivated.**
  The user already paid for it; a retailer leaving the programme must not strand
  someone standing at a till.

### Honest framing

Two limits worth stating plainly rather than hiding — see
`docs/07_Implementation_Plan.md` § Honest Framing:

- **Partner verification is manual code entry, by design.** There is no retailer
  POS integration in this build window. The app renders a QR (`zoa://redeem/<uuid>`)
  and the partner screen accepts a pasted payload or a typed code, but nothing
  scans it in-app. The check itself is real and one-shot.
- **Any partner-staff account can verify any code.** The schema has no link from
  a staff user to a `partners` row, so the endpoint cannot scope a cashier to
  their own shop. Adding `partners.staff_user_id` would be the fix; it is out of
  scope here.

## Admin (Phase 5)

Admin-only, and `RequireRole` admits admin to every lower role's routes too, so one
login drives the whole demo.

```sh
curl -H "Authorization: Bearer $ADMIN_TOKEN" localhost:8080/admin/stats

curl -H "Authorization: Bearer $ADMIN_TOKEN" localhost:8080/admin/vouchers
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
     -d '{"partner_id":1,"title":"KSh 300 off","points_cost":500,
          "discount_type":"fixed","discount_value":300,"expiry_days":30}' \
     localhost:8080/admin/vouchers
curl -X PATCH -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
     -d '{"stock_remaining":null}' localhost:8080/admin/vouchers/1   # null = unlimited
curl -X DELETE -H "Authorization: Bearer $ADMIN_TOKEN" localhost:8080/admin/vouchers/1
```

`GET /admin/stats` is the demo's closing slide: users by role, submissions by status
with the **verified** weight collected, the points ledger aggregated into
issued/spent/outstanding, redemptions by status, and the FR-22 classification
accuracy. In the app it is reached from **Profile → Platform statistics**, admin
only — a route rather than a seventh bottom-nav tab, since the plan calls it a
"minor" overview.

Four decisions worth knowing:

- **`classification.accuracy` is `null`, not `0`, before anything is verified.** An
  untested model is not a model that is wrong every time. It compares
  `predicted_category` against the collector-corrected `material_type` over verified
  submissions only — which is measurable precisely because a correction overwrites
  `material_type` and leaves the prediction alone.
- **`points.outstanding` must equal the sum of every `users.points_balance`.** That
  cross-check is the platform-scale version of the ledger invariant, and
  `TestStatsPointsMatchLedgerAndBalances` asserts it.
- **`DELETE` is a soft delete** (`active = 0`) on both partners and vouchers. A hard
  delete would break a code a user is already holding;
  `TestSoftDeleteKeepsIssuedCodesWorking` withdraws an offer and then verifies a code
  issued against it.
- **Admin listings include inactive rows**, unlike the public catalogue — otherwise a
  deactivated offer would be unreachable and impossible to switch back on. `PATCH`
  treats an explicit `null` as "clear this" and an absent key as "leave alone", which
  is how a voucher is made unlimited again.

### Still outstanding in Phase 5

The polish items the plan lists are largely already met: every screen built since
Phase 2 carries loading, error and empty states, and the "realistic demo data" it
asks for (partner names, sample users) is seeded by migration `004` and
`-seed-demo`. Partner logos are deliberately absent — the catalogue draws
lettermarks from the name instead, so there are no image assets to ship.

What genuinely remains is rehearsal: walking the demo script end to end twice, which
needs a person and a device.

## Running the app

Requires the Flutter SDK. It installs into `$HOME` without root — no package
manager, no `sudo`:

```sh
curl -LO https://storage.googleapis.com/flutter_infra_release/releases/stable/linux/flutter_linux_3.47.1-stable.tar.xz
tar -xJf flutter_linux_3.47.1-stable.tar.xz -C ~/
export PATH="$HOME/flutter/bin:$PATH"   # add to ~/.zshrc to persist
flutter --version                        # 3.47.1 ships Dart 3.13.1
```

Dart 3.13.1 satisfies the `>=3.3.0 <4.0.0` constraint in `pubspec.yaml`. That is
enough for `flutter pub get`, `flutter analyze` and `flutter test`; building an
APK additionally needs the Android SDK and a JDK.

The Flutter project carries its own `lib/`, `pubspec.yaml` and bundled fonts, but **not** the generated platform folders. Generate them once, in place:

```sh
cd app
flutter create . --platforms=android,ios --project-name zoa --org com.zoa
flutter pub get
flutter analyze   # must report no errors or warnings before you trust a change
flutter run
```

`flutter create .` on an existing directory fills in `android/`, `ios/` and friends without touching `lib/`, `pubspec.yaml` or `assets/`.

### Camera permissions

The Phase 2.5 photo step uses `image_picker`, which needs one declaration per
platform. `flutter create .` does not add these, so add them after generating the
platform folders — otherwise picking a photo fails at runtime (handled
gracefully: the app tells the user to choose the material by hand, but the assist
never works).

`android/app/src/main/AndroidManifest.xml`, inside `<manifest>`:

```xml
<uses-permission android:name="android.permission.CAMERA" />
```

`ios/Runner/Info.plist`, inside the top-level `<dict>`:

```xml
<key>NSCameraUsageDescription</key>
<string>Zoa uses the camera to identify the material you are recycling.</string>
<key>NSPhotoLibraryUsageDescription</key>
<string>Zoa reads a photo you choose to identify the material you are recycling.</string>
```

The default API host is `http://10.0.2.2:8080` — the Android emulator's route to the host machine. Override it for a physical device or desktop:

```sh
flutter run --dart-define=ZOA_API_BASE_URL=http://192.168.1.42:8080
```

## Conventions

- **Schema** follows `docs/06_Backend_Schema.md` exactly — table and column names are not renamed.
- **Endpoints** follow `docs/05_App_Flow.md` §2 exactly: bare paths, no `/api/v1` prefix. `docs/API_CONTRACT.md` holds the full request/response shapes.
- **Status transitions are server-side only.** The client triggers actions; the backend validates and transitions.
- **Points are integers**, always credited through `points_ledger` inside a transaction that also updates `users.points_balance`.
- **Phone numbers are normalised** to `+254XXXXXXXXX` on the way in, since the column is `UNIQUE` and doubles as the login identifier.
- **Design system** is `docs/zoa-website.html`. Colours, type and spacing are copied from it, never re-invented: forest green anchors, savanna gold means points, Fraunces for headlines, IBM Plex Sans for UI, IBM Plex Mono for anything data-like.
