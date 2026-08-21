/// Integration checks for [ApiClient] against a real backend.
///
/// These exist for one reason: the multipart encoding on the Dart side and
/// `r.FormFile("photo")` on the Go side have to agree, and nothing else in the
/// toolchain checks that. `flutter analyze` cannot see it, the Go handler tests
/// build their own multipart bodies, and a mismatch shows up only as a 400 on a
/// real device.
///
/// They need a server, so they skip themselves when one is not listening rather
/// than failing a normal `flutter test` run:
///
/// ```sh
/// cd backend && DB_PATH=/tmp/zoa-e2e.db PORT=8097 \
///   ZOA_CLASSIFY_PROVIDER=mock go run ./cmd/api -seed-demo
/// cd app && flutter test test/api_client_classify_test.dart
/// ```
library;

import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:zoa/api/api_client.dart';
import 'package:zoa/api/api_models.dart';

const _baseUrl = 'http://localhost:8097';

/// Demo recycler from `go run ./cmd/api -seed-demo`.
const _phone = '+254712000001';
const _password = 'zoa1234';

/// The smallest byte sequence Go's http.DetectContentType calls a JPEG.
///
/// The server sniffs the media type but never decodes the image, and the mock
/// classifier hashes the bytes rather than reading pixels, so valid magic bytes
/// are enough. A real photo would test nothing further here.
final _jpegBytes = Uint8List.fromList([
  0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
  ...List<int>.filled(64, 0x20),
  0xFF, 0xD9,
]);

Future<bool> _serverIsUp() async {
  try {
    final client = HttpClient()
      ..connectionTimeout = const Duration(seconds: 2);
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

  Future<ApiClient> signedInClient() async {
    String? token;
    final client = ApiClient(baseUrl: _baseUrl, tokenProvider: () => token);
    final session = await client.login(phoneNumber: _phone, password: _password);
    token = session.token;
    return client;
  }

  test('classify uploads a photo and parses the prediction', () async {
    if (!serverUp) return;

    final client = await signedInClient();

    final result = await client.classify(
      photoBytes: _jpegBytes,
      // The mock keys off the filename, so this pins the whole round trip to a
      // known answer rather than to whatever the hash happens to pick.
      filename: 'pet_bottle_01.jpg',
    );

    // If the multipart encoding were wrong, the server would 400 and this would
    // have thrown before reaching any of these.
    expect(result.degraded, isFalse, reason: result.reason ?? 'no reason given');
    expect(result.predictedCategory, 'pet');
    expect(result.label, 'PET bottles');
    expect(result.group, 'plastics');
    expect(result.requiresSourceType, isFalse);
    expect(result.predictedConfidence, greaterThan(0));
    expect(result.predictedConfidence, lessThanOrEqualTo(1));
    expect(result.confidencePercent, inInclusiveRange(1, 100));
  });

  test('classify flags organics as needing a source type', () async {
    if (!serverUp) return;

    final client = await signedInClient();

    final result = await client.classify(
      photoBytes: _jpegBytes,
      filename: 'food_waste_kitchen.jpg',
    );

    expect(result.degraded, isFalse, reason: result.reason ?? 'no reason given');
    expect(result.predictedCategory, 'food_waste');
    expect(result.group, 'organic');
    expect(result.requiresSourceType, isTrue);
  });

  test('a non-image upload is rejected rather than silently classified', () async {
    if (!serverUp) return;

    final client = await signedInClient();

    await expectLater(
      client.classify(
        photoBytes: Uint8List.fromList('%PDF-1.7 not an image'.codeUnits),
        filename: 'receipt.jpg',
      ),
      throwsA(isA<Object>()),
    );
  });

  test('submitting carries the prediction alongside the confirmed material',
      () async {
    if (!serverUp) return;

    final client = await signedInClient();

    // The user overrides the AI: confirmed hdpe, predicted pet. Both must be
    // stored, because the disagreement is the accuracy metric (FR-22).
    final submission = await client.createSubmission(
      materialType: 'hdpe',
      estimatedQtyKg: 3.5,
      predictedCategory: 'pet',
      predictedConfidence: 0.71,
    );

    expect(submission.materialType, 'hdpe');
    expect(submission.status, 'pending');
  });

  test('an organic submission without a source type is rejected', () async {
    if (!serverUp) return;

    final client = await signedInClient();

    await expectLater(
      client.createSubmission(materialType: 'food_waste', estimatedQtyKg: 12),
      throwsA(isA<Object>()),
    );
  });

  test('an organic submission with a source type succeeds', () async {
    if (!serverUp) return;

    final client = await signedInClient();

    final submission = await client.createSubmission(
      materialType: 'food_waste',
      estimatedQtyKg: 12,
      sourceType: 'hotel',
      predictedCategory: 'food_waste',
      predictedConfidence: 0.88,
    );

    expect(submission.materialType, 'food_waste');
    expect(submission.status, 'pending');
  });

  // --- Phase 3: voucher catalogue ---

  test('the catalogue arrives seeded, with partners embedded', () async {
    if (!serverUp) return;

    final client = await signedInClient();
    final catalogue = await client.vouchers();

    expect(catalogue.vouchers, isNotEmpty,
        reason: 'migration 004 should seed the catalogue');

    for (final voucher in catalogue.vouchers) {
      expect(voucher.title, isNotEmpty);
      expect(voucher.pointsCost, greaterThan(0));
      expect(voucher.partner.name, isNotEmpty,
          reason: 'the partner must be embedded, not fetched separately');
      expect(
        voucher.discountType,
        anyOf(DiscountType.percentage, DiscountType.fixed),
      );
    }
  });

  test('the catalogue is cheapest-first', () async {
    if (!serverUp) return;

    final client = await signedInClient();
    final catalogue = await client.vouchers();

    final costs = catalogue.vouchers.map((v) => v.pointsCost).toList();
    final sorted = [...costs]..sort();
    expect(costs, sorted, reason: 'the server orders by points_cost ascending');
  });

  test('affordability agrees with the balance the server reports', () async {
    if (!serverUp) return;

    final client = await signedInClient();
    final catalogue = await client.vouchers();

    for (final voucher in catalogue.vouchers) {
      expect(
        voucher.affordable,
        catalogue.pointsBalance >= voucher.pointsCost,
        reason: 'voucher ${voucher.id} costs ${voucher.pointsCost} at '
            'balance ${catalogue.pointsBalance}',
      );
    }
  });

  test('the affordable filter never returns something out of reach', () async {
    if (!serverUp) return;

    final client = await signedInClient();
    final affordable = await client.vouchers(affordableOnly: true);

    for (final voucher in affordable.vouchers) {
      expect(voucher.affordable, isTrue);
      expect(voucher.pointsCost, lessThanOrEqualTo(affordable.pointsBalance));
    }
  });

  test('the partner filter returns only that partner', () async {
    if (!serverUp) return;

    final client = await signedInClient();
    final partners = await client.partners();
    expect(partners, isNotEmpty);

    final target = partners.first;
    final filtered = await client.vouchers(partnerId: target.id);

    expect(filtered.vouchers, isNotEmpty);
    for (final voucher in filtered.vouchers) {
      expect(voucher.partnerId, target.id);
    }
  });

  test('a voucher resolves by id with the same shape as a list entry', () async {
    if (!serverUp) return;

    final client = await signedInClient();
    final catalogue = await client.vouchers();
    final wanted = catalogue.vouchers.first;

    final detail = await client.voucher(wanted.id);

    expect(detail.id, wanted.id);
    expect(detail.title, wanted.title);
    expect(detail.pointsCost, wanted.pointsCost);
    expect(detail.partner.name, wanted.partner.name);
  });

  test('an unknown voucher id throws rather than returning an empty object',
      () async {
    if (!serverUp) return;

    final client = await signedInClient();

    await expectLater(client.voucher(99999), throwsA(isA<Object>()));
  });

  test('unlimited stock survives as null, not zero', () async {
    if (!serverUp) return;

    final client = await signedInClient();
    final catalogue = await client.vouchers();

    // The seed includes an unlimited-stock voucher on purpose. Parsing it as 0
    // would render "0 left" for something always available.
    final unlimited =
        catalogue.vouchers.where((v) => v.stockRemaining == null).toList();
    expect(unlimited, isNotEmpty,
        reason: 'the seed should exercise the unlimited-stock case');

    for (final voucher in unlimited) {
      expect(voucher.scarcityLabel, isNull,
          reason: 'unlimited stock must not read as scarce');
    }
  });

  test('discount labels render both discount types', () async {
    if (!serverUp) return;

    final client = await signedInClient();
    final catalogue = await client.vouchers();

    for (final voucher in catalogue.vouchers) {
      final label = voucher.discountLabel;
      expect(label, isNotEmpty);
      // No trailing ".0" on whole numbers — "KSh 100 off", not "KSh 100.0 off".
      expect(label, isNot(contains('.0')));

      if (voucher.discountType == DiscountType.percentage) {
        expect(label, contains('%'));
      } else {
        expect(label, contains('KSh'));
      }
    }
  });
}
