/// Integration checks for the Phase 4 redemption endpoints against a real backend.
///
/// These exist for the one thing neither test suite can see on its own: that the
/// Dart models parse exactly what the Go handlers emit. The Go tests assert the
/// JSON they produce, the Dart analyzer checks the models compile, and a
/// disagreement between the two — a renamed key, a nested object where a sibling
/// was expected — shows up only as a null field on a real device.
///
/// The whole loop runs here, through the real client: submit → verify → points →
/// redeem → code-verify → refuse the second attempt.
///
/// They need a server, so they skip themselves when one is not listening rather
/// than failing a normal `flutter test` run:
///
/// ```sh
/// cd backend && DB_PATH=/tmp/zoa-e2e.db PORT=8097 \
///   ZOA_CLASSIFY_PROVIDER=mock go run ./cmd/api -seed-demo
/// cd app && flutter test test/api_client_redemptions_test.dart
/// ```
library;

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:zoa/api/api_client.dart';
import 'package:zoa/api/api_exception.dart';
import 'package:zoa/api/api_models.dart';

const _baseUrl = 'http://localhost:8097';
const _password = 'zoa1234';

/// Demo cast from `go run ./cmd/api -seed-demo`.
const _recycler = '+254712000001';
const _collector = '+254712000002';
const _partnerStaff = '+254712000003';

Future<bool> _serverIsUp() async {
  try {
    final client = HttpClient()..connectionTimeout = const Duration(seconds: 2);
    final request = await client.getUrl(Uri.parse('$_baseUrl/health'));
    final response = await request.close();
    await response.drain<void>();
    client.close();
    return response.statusCode == 200;
  } catch (_) {
    return false;
  }
}

void main() {
  late bool serverUp;

  setUpAll(() async {
    serverUp = await _serverIsUp();
    if (!serverUp) {
      // ignore: avoid_print
      print('SKIPPING: no backend on $_baseUrl — see this file\'s header.');
    }
  });

  Future<ApiClient> signedInAs(String phone) async {
    String? token;
    final client = ApiClient(baseUrl: _baseUrl, tokenProvider: () => token);
    final session = await client.login(phoneNumber: phone, password: _password);
    token = session.token;
    return client;
  }

  /// Earns the recycler enough points to afford [needed], through the real
  /// submission → collector-verify path rather than by seeding a balance.
  ///
  /// PET pays 25/kg, so the weight is derived from the shortfall — the test does
  /// not assume a particular starting balance, because this runs against a
  /// long-lived demo database that other runs have already spent from.
  Future<void> earnAtLeast(ApiClient user, ApiClient collector, int needed) async {
    final before = (await user.me()).pointsBalance;
    if (before >= needed) return;

    final kg = ((needed - before) / 25).ceil().toDouble();
    final submission = await user.createSubmission(
      materialType: 'pet',
      estimatedQtyKg: kg,
      location: 'Kilimani drop-off point',
    );

    final verified = await collector.verifySubmission(
      submission.id,
      verifiedQtyKg: kg,
    );
    expect(verified.pointsBalance, greaterThanOrEqualTo(needed),
        reason: 'the collector verify did not credit enough to redeem');
  }

  test('the whole redemption loop round-trips through the client', () async {
    if (!serverUp) return;

    final user = await signedInAs(_recycler);
    final collector = await signedInAs(_collector);
    final partner = await signedInAs(_partnerStaff);

    // Pick the cheapest voucher the catalogue offers rather than a hardcoded id,
    // so the seed can change without breaking this.
    final catalogue = await user.vouchers();
    expect(catalogue.vouchers, isNotEmpty, reason: 'the seeded catalogue is empty');
    final voucher = catalogue.vouchers.first;

    await earnAtLeast(user, collector, voucher.pointsCost);

    final balanceBefore = (await user.me()).pointsBalance;

    // --- redeem ---
    final result = await user.createRedemption(voucherId: voucher.id);

    expect(result.pointsSpent, voucher.pointsCost);
    expect(result.pointsBalance, balanceBefore - voucher.pointsCost,
        reason: 'the server did not deduct exactly the voucher cost');

    final redemption = result.redemption;
    // A canonical UUID. If the key were misnamed this would be empty, which is the
    // failure this file exists to catch.
    expect(redemption.code, hasLength(36));
    expect(redemption.code.split('-'), hasLength(5));
    expect(redemption.qrPayload, 'zoa://redeem/${redemption.code}');
    expect(redemption.status, RedemptionStatus.issued);
    expect(redemption.isRedeemable, isTrue);

    // Nullable columns must arrive as null, not as a zero value.
    expect(redemption.usedAt, isNull);
    expect(redemption.verifiedBy, isNull);

    // Expiry is derived server-side from issued_at + expiry_days, and must survive
    // the round trip as a real date rather than the epoch fallback.
    expect(redemption.expiry.isAfter(redemption.issuedAt), isTrue);
    expect(redemption.expiry.difference(redemption.issuedAt).inDays,
        voucher.expiryDays);

    // The voucher arrives as a sibling of the redemption on a create response.
    expect(result.voucher.id, voucher.id);
    expect(result.voucher.partner.name, isNotEmpty);

    // --- history ---
    final history = await user.redemptions();
    final mine = history.where((r) => r.code == redemption.code).toList();
    expect(mine, hasLength(1), reason: 'the new code is not in the history');

    // ...where the voucher is *embedded* instead, which is the shape difference
    // most likely to break silently.
    expect(mine.single.voucher, isNotNull,
        reason: 'the listing must embed the voucher so history renders in one call');
    expect(mine.single.voucher!.partner.name, isNotEmpty);
    expect(mine.single.qrPayload, redemption.qrPayload);

    // --- partner verifies ---
    final verification = await partner.verifyRedemption(redemption.code);

    expect(verification.status, RedemptionStatus.used);
    expect(verification.redemption.usedAt, isNotNull);
    expect(verification.redemption.verifiedBy, isNotNull);
    expect(verification.userName, isNotEmpty,
        reason: 'the cashier needs the customer name');
    expect(verification.message, contains('off'),
        reason: 'the message should name the discount to apply');
    expect(verification.voucher.id, voucher.id);

    // --- and refuses the second attempt ---
    // The anti-double-spend guarantee, seen from the client: a 409 arrives as an
    // ApiException carrying the conflict code, not as a silent success.
    await expectLater(
      partner.verifyRedemption(redemption.code),
      throwsA(
        isA<ApiException>().having((e) => e.code, 'code', ApiErrorCode.conflict),
      ),
    );

    // The history now reflects it, without the client having patched anything
    // locally.
    final after = await user.redemptions();
    final spent = after.firstWhere((r) => r.code == redemption.code);
    expect(spent.isUsed, isTrue);
    expect(spent.isRedeemable, isFalse);
    expect(spent.statusLabel, 'Used');
  });

  test('an unknown code is a not_found, not a crash', () async {
    if (!serverUp) return;

    final partner = await signedInAs(_partnerStaff);

    await expectLater(
      partner.verifyRedemption('7f3c1a92-0000-0000-0000-8a5f0b6d1c34'),
      throwsA(
        isA<ApiException>().having((e) => e.code, 'code', ApiErrorCode.notFound),
      ),
    );
  });

  test('a recycler cannot verify a code', () async {
    if (!serverUp) return;

    final user = await signedInAs(_recycler);

    // Role is enforced server-side. Reaching for it as a plain user is a 403, and
    // the ordering matters: the role check has to happen before the code lookup,
    // or an unknown code would leak a 404 to an unauthorised caller.
    await expectLater(
      user.verifyRedemption('7f3c1a92-0000-0000-0000-8a5f0b6d1c34'),
      throwsA(
        isA<ApiException>().having((e) => e.code, 'code', ApiErrorCode.forbidden),
      ),
    );
  });

  test('redeeming beyond the balance is refused without spending', () async {
    if (!serverUp) return;

    final user = await signedInAs(_recycler);
    final catalogue = await user.vouchers();

    // The most expensive voucher, which the demo recycler will not be able to
    // afford unless a run has deliberately over-earned.
    final dearest = catalogue.vouchers.last;
    final balance = (await user.me()).pointsBalance;
    if (balance >= dearest.pointsCost) return; // nothing to prove this run

    await expectLater(
      user.createRedemption(voucherId: dearest.id),
      throwsA(
        isA<ApiException>().having((e) => e.code, 'code', ApiErrorCode.conflict),
      ),
    );

    expect((await user.me()).pointsBalance, balance,
        reason: 'a refused redemption must not deduct anything');
  });

  test('normaliseRedemptionCode accepts what a cashier actually pastes', () {
    // Pure client-side, so this one needs no server: the field has to cope with a
    // scanned QR payload, a padded paste, and a code typed in capitals.
    const code = '7f3c1a92-4b0e-4c7d-9e21-8a5f0b6d1c34';

    expect(normaliseRedemptionCode(code), code);
    expect(normaliseRedemptionCode('  $code\n'), code);
    expect(normaliseRedemptionCode('zoa://redeem/$code'), code);
    expect(normaliseRedemptionCode(' zoa://redeem/$code '), code);
    expect(normaliseRedemptionCode(code.toUpperCase()), code);
    expect(normaliseRedemptionCode(''), isEmpty);
  });
}
