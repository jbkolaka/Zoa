/// Widget tests for the redemption confirmation screen.
///
/// The first screen in this project to be rendered rather than only analysed.
/// `flutter analyze` cannot tell whether a layout overflows, whether the code
/// still fits at a large text scale, or whether the QR package still paints — and
/// until Phase 4 nothing in `lib/screens/` had been laid out at all. This covers
/// the one screen the demo turns on, and the one that carries a new dependency.
library;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:qr_flutter/qr_flutter.dart';
import 'package:zoa/api/api_models.dart';
import 'package:zoa/screens/redemption_confirmation_screen.dart';
import 'package:zoa/theme/zoa_theme.dart';

const _code = '7f3c1a92-4b0e-4c7d-9e21-8a5f0b6d1c34';

Voucher _voucher() => const Voucher(
      id: 3,
      partnerId: 1,
      title: '10% off your basket',
      pointsCost: 420,
      discountType: DiscountType.percentage,
      discountValue: 10,
      expiryDays: 30,
      stockRemaining: 50,
      active: true,
      partner: VoucherPartner(
        id: 1,
        name: 'Naivas Supermarket',
        logoUrl: null,
        active: true,
      ),
      affordable: true,
    );

Redemption _redemption({
  String status = RedemptionStatus.issued,
  Duration expiresIn = const Duration(days: 29),
  DateTime? usedAt,
}) {
  final issuedAt = DateTime.now().subtract(const Duration(days: 1));
  return Redemption(
    id: 8,
    userId: 1,
    voucherId: 3,
    code: _code,
    status: status,
    issuedAt: issuedAt,
    expiry: DateTime.now().add(expiresIn),
    qrPayload: 'zoa://redeem/$_code',
    usedAt: usedAt,
  );
}

/// Pumps the screen at a given size and text scale.
///
/// 360x640 is the narrow end of the phones this is built for, and 1.4 is the
/// ceiling `main.dart` clamps the system font scale to — so together they are the
/// tightest layout the screen can legitimately be asked to survive.
Future<void> _pump(
  WidgetTester tester,
  Widget screen, {
  Size size = const Size(360, 640),
  double textScale = 1.0,
}) async {
  tester.view.devicePixelRatio = 1.0;
  tester.view.physicalSize = size;
  addTearDown(tester.view.reset);

  await tester.pumpWidget(
    MaterialApp(
      theme: buildZoaTheme(),
      home: Builder(
        builder: (context) => MediaQuery(
          data: MediaQuery.of(context).copyWith(
            textScaler: TextScaler.linear(textScale),
          ),
          child: screen,
        ),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('renders the code, the QR and the partner', (tester) async {
    await _pump(
      tester,
      RedemptionConfirmationScreen(
        redemption: _redemption(),
        voucher: _voucher(),
      ),
    );

    expect(tester.takeException(), isNull);

    // The QR has to actually paint — this is the whole reason the dependency is
    // here, and a silently-failing QR is a code no scanner can read.
    expect(find.byType(QrImageView), findsOneWidget);

    // The code is shown in its dash-separated groups, so it is matched group by
    // group rather than as one string.
    for (final group in _code.split('-')) {
      expect(find.text(group), findsOneWidget,
          reason: 'code group $group is missing');
    }

    expect(find.text('10% off your basket'), findsOneWidget);
    expect(find.text('NAIVAS SUPERMARKET'), findsOneWidget);
    expect(find.text('Ready to use'), findsOneWidget);
  });

  testWidgets('survives a narrow screen at the maximum text scale', (tester) async {
    // The real check: no RenderFlex overflow, which is exactly what analyze
    // cannot see and what a device would show as a yellow-and-black stripe.
    await _pump(
      tester,
      RedemptionConfirmationScreen(
        redemption: _redemption(),
        voucher: _voucher(),
        pointsSpent: 420,
        pointsBalance: 25,
      ),
      size: const Size(320, 568),
      textScale: 1.4,
    );

    expect(tester.takeException(), isNull,
        reason: 'the screen overflowed at 320dp and 1.4x text');
  });

  testWidgets('shows the spend only when it just happened', (tester) async {
    await _pump(
      tester,
      RedemptionConfirmationScreen(
        redemption: _redemption(),
        voucher: _voucher(),
        pointsSpent: 420,
        pointsBalance: 25,
      ),
    );
    expect(find.text('Redeemed'), findsOneWidget);
    expect(find.textContaining('420 points spent'), findsOneWidget);

    // Reopened from history, the deduction is old news and is not restated.
    await _pump(
      tester,
      RedemptionConfirmationScreen(
        redemption: _redemption(),
        voucher: _voucher(),
      ),
    );
    expect(find.text('Redeemed'), findsNothing);
  });

  testWidgets('a used code reads as used and drops the till instructions',
      (tester) async {
    await _pump(
      tester,
      RedemptionConfirmationScreen(
        redemption: _redemption(
          status: RedemptionStatus.used,
          usedAt: DateTime.now(),
        ),
        voucher: _voucher(),
      ),
    );

    expect(tester.takeException(), isNull);
    expect(find.text('Used'), findsOneWidget);
    expect(find.text('This code can no longer be used.'), findsOneWidget);
    // Telling someone to show a spent code at a till would send them to be
    // refused at a counter.
    expect(find.text('At the till'.toUpperCase()), findsNothing);
  });

  testWidgets('a code past its expiry reads as expired even while status is issued',
      (tester) async {
    // The server only transitions a row to `expired` when someone tries to verify
    // it, so history can legitimately hand this screen an `issued` code that is
    // already stale. It must not be presented as usable.
    await _pump(
      tester,
      RedemptionConfirmationScreen(
        redemption: _redemption(expiresIn: const Duration(days: -2)),
        voucher: _voucher(),
      ),
    );

    expect(tester.takeException(), isNull);
    expect(find.text('Expired'), findsOneWidget);
    expect(find.text('Ready to use'), findsNothing);
  });

  testWidgets('copying the code confirms in place', (tester) async {
    // The screen awaits Clipboard.setData before it flips the label. Nothing
    // answers SystemChannels.platform in a test binding, so without a mock
    // handler that await never returns and _copied stays false.
    final copied = <MethodCall>[];
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        copied.add(call);
        return null;
      },
    );
    addTearDown(() => tester.binding.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, null));

    await _pump(
      tester,
      RedemptionConfirmationScreen(
        redemption: _redemption(),
        voucher: _voucher(),
      ),
    );

    expect(find.text('Copy code'), findsOneWidget);

    // The button sits below the fold at this viewport, and ZoaPage scrolls. A
    // bare tap() would fire at a point outside the viewport, miss the button
    // (warnIfMissed) and leave _copied false.
    await tester.ensureVisible(find.text('Copy code'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Copy code'));
    // Two frames: _copy() awaits the clipboard channel before it setStates, so
    // the label has not changed yet on the first.
    await tester.pump();
    await tester.pump();

    expect(tester.takeException(), isNull);
    expect(find.text('Copied'), findsOneWidget);

    // The code itself reached the clipboard, not just the label.
    final setData = copied.where((c) => c.method == 'Clipboard.setData');
    expect(setData, hasLength(1));
    expect((setData.single.arguments as Map)['text'], _code);

    // The confirmation reverts itself after two seconds. Let that timer fire —
    // pumpAndSettle() cannot be used above (it would run past the revert), so
    // without this the test ends with a pending timer.
    await tester.pump(const Duration(seconds: 2));
    expect(find.text('Copy code'), findsOneWidget);
  });
}
