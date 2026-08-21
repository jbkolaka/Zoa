/// Redemption Confirmation — the code and QR to show at the till (Phase 4).
///
/// This is the screen the whole product builds towards, so it is deliberately the
/// most formal one in the app: the code is the largest thing on it, the QR sits
/// beside it on a white field, and the partner and expiry are stated plainly.
/// UI/UX §1.3 asks redemption to look "official and hard to fake" — a discount a
/// cashier does not trust is a discount the user does not get.
///
/// Takes its data as constructor arguments rather than reading a controller, for
/// two reasons: it is reached from two places (straight after redeeming, and from
/// history), and it makes the screen directly widget-testable.
library;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:qr_flutter/qr_flutter.dart';

import '../api/api_models.dart';
import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';
import '../util/format.dart';
import '../widgets/fade_slide_in.dart';
import '../widgets/redemption_status.dart';
import '../widgets/zoa_ui.dart';

class RedemptionConfirmationScreen extends StatelessWidget {
  const RedemptionConfirmationScreen({
    super.key,
    required this.redemption,
    required this.voucher,
    this.pointsSpent,
    this.pointsBalance,
  });

  final Redemption redemption;
  final Voucher voucher;

  /// What it cost, shown only when arriving straight from a redemption. Omitted
  /// when the screen is reopened from history, where the deduction is old news.
  final int? pointsSpent;

  /// The balance the server reported after deducting.
  final int? pointsBalance;

  @override
  Widget build(BuildContext context) {
    final justRedeemed = pointsSpent != null;

    return Scaffold(
      backgroundColor: ZoaColors.paper,
      appBar: AppBar(
        backgroundColor: ZoaColors.paper,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        foregroundColor: ZoaColors.forestDeep,
        title: Text('Your code', style: ZoaType.label),
      ),
      body: SafeArea(
        child: ZoaPage(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: FadeSlideIn.staggered([
              if (justRedeemed) ...[
                const SizedBox(height: ZoaSpace.sm),
                _RedeemedBanner(
                  pointsSpent: pointsSpent!,
                  pointsBalance: pointsBalance,
                ),
                const SizedBox(height: ZoaSpace.lg),
              ],
              _CodePanel(redemption: redemption, voucher: voucher),
              const SizedBox(height: ZoaSpace.lg),
              _Instructions(redemption: redemption),
              const SizedBox(height: ZoaSpace.lg),
              ZoaGhostButton(
                label: 'Done',
                onPressed: () => Navigator.of(context).pop(),
              ),
              const SizedBox(height: ZoaSpace.xl),
            ]),
          ),
        ),
      ),
    );
  }
}

/// Confirms the spend, once, at the top — so the points leaving the balance is
/// acknowledged rather than silently discovered later.
class _RedeemedBanner extends StatelessWidget {
  const _RedeemedBanner({required this.pointsSpent, required this.pointsBalance});

  final int pointsSpent;
  final int? pointsBalance;

  @override
  Widget build(BuildContext context) {
    final balance = pointsBalance;

    return ZoaCard(
      accent: true,
      padding: const EdgeInsets.all(ZoaSpace.lg),
      child: Row(
        children: [
          const Icon(Icons.check_circle, size: 20, color: ZoaColors.goldDeep),
          const SizedBox(width: ZoaSpace.md),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Redeemed', style: ZoaType.label),
                const SizedBox(height: 2),
                Text(
                  balance == null
                      ? '${formatPoints(pointsSpent)} points spent.'
                      : '${formatPoints(pointsSpent)} points spent · '
                          '${formatPoints(balance)} left',
                  style: ZoaType.bodySm,
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// The code itself, the QR, and what the discount is for.
class _CodePanel extends StatelessWidget {
  const _CodePanel({required this.redemption, required this.voucher});

  final Redemption redemption;
  final Voucher voucher;

  @override
  Widget build(BuildContext context) {
    return ZoaCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            voucher.partner.name.toUpperCase(),
            style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: ZoaSpace.xs),
          Text(voucher.title, style: ZoaType.h3, textAlign: TextAlign.center),
          const SizedBox(height: ZoaSpace.xs),
          Text(
            voucher.discountLabel,
            style: ZoaType.label.copyWith(color: ZoaColors.leaf),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: ZoaSpace.lg),
          const Divider(height: 1, color: ZoaColors.line),
          const SizedBox(height: ZoaSpace.lg),
          _StatusRow(redemption: redemption),
          const SizedBox(height: ZoaSpace.lg),
          _QrPanel(payload: redemption.qrPayload),
          const SizedBox(height: ZoaSpace.lg),
          _Code(code: redemption.code, spent: !redemption.isRedeemable),
        ],
      ),
    );
  }
}

/// Status and expiry together: an icon, a word and a date, never colour alone.
class _StatusRow extends StatelessWidget {
  const _StatusRow({required this.redemption});

  final Redemption redemption;

  @override
  Widget build(BuildContext context) {
    final look = RedemptionLook.of(redemption);

    // The used and expired cases already say everything in the label, so only a
    // live code spends a line on its deadline.
    final detail = redemption.isUsed
        ? 'Accepted on ${formatDate(redemption.usedAt ?? redemption.issuedAt)}'
        : redemption.isRedeemable
            ? 'Valid until ${formatDate(redemption.expiry)}'
            : 'Expired on ${formatDate(redemption.expiry)}';

    return Row(
      children: [
        Icon(look.icon, size: 18, color: look.color),
        const SizedBox(width: ZoaSpace.sm),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                redemption.statusLabel,
                style: ZoaType.label.copyWith(color: look.color),
              ),
              const SizedBox(height: 2),
              Text(detail, style: ZoaType.bodySm),
            ],
          ),
        ),
      ],
    );
  }
}

/// The QR, on a white field.
///
/// White rather than the warm paper ground: scanners want maximum contrast, and
/// the design system's off-white is not worth a failed scan at a till. The
/// `errorStateBuilder` matters as much as the QR — if the payload could not be
/// encoded, the code below is still valid and the screen has to say so rather
/// than showing a blank square.
class _QrPanel extends StatelessWidget {
  const _QrPanel({required this.payload});

  final String payload;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Container(
        padding: const EdgeInsets.all(ZoaSpace.md),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: ZoaRadius.allSm,
          border: Border.all(color: ZoaColors.line),
        ),
        child: QrImageView(
          data: payload,
          version: QrVersions.auto,
          size: 176,
          backgroundColor: Colors.white,
          eyeStyle: const QrEyeStyle(
            eyeShape: QrEyeShape.square,
            color: ZoaColors.forestDeep,
          ),
          dataModuleStyle: const QrDataModuleStyle(
            dataModuleShape: QrDataModuleShape.square,
            color: ZoaColors.forestDeep,
          ),
          errorStateBuilder: (context, error) => SizedBox(
            width: 176,
            height: 176,
            child: Center(
              child: Text(
                'QR unavailable — read out the code below instead.',
                style: ZoaType.bodySm,
                textAlign: TextAlign.center,
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// The code, in mono, with a copy action.
///
/// Copy is not a convenience here: verification is manual code entry, and nobody
/// should have to retype 36 characters at a checkout.
class _Code extends StatelessWidget {
  const _Code({required this.code, required this.spent});

  final String code;

  /// Dims the code once it can no longer be used, so a spent code does not read
  /// as presentable.
  final bool spent;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Text(
          'REDEMPTION CODE',
          style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft),
        ),
        const SizedBox(height: ZoaSpace.sm),
        // Labelled with the unbroken code so a screen reader gets one string
        // rather than five fragments and four dashes.
        Semantics(
          label: code,
          excludeSemantics: true,
          child: _GroupedCode(code: code, spent: spent),
        ),
        const SizedBox(height: ZoaSpace.md),
        _CopyButton(code: code),
        if (spent) ...[
          const SizedBox(height: ZoaSpace.sm),
          Text(
            'This code can no longer be used.',
            style: ZoaType.bodySm.copyWith(color: ZoaColors.statusError),
            textAlign: TextAlign.center,
          ),
        ],
      ],
    );
  }
}

/// The code split at its dashes.
///
/// A 36-character UUID does not fit one line at a readable size on a 360dp phone,
/// and a code broken mid-group is a code read out wrong. Wrapping at the dashes
/// keeps every run short enough to say aloud and to check a character at a time,
/// and lets the type stay large enough to read across a counter.
class _GroupedCode extends StatelessWidget {
  const _GroupedCode({required this.code, required this.spent});

  final String code;
  final bool spent;

  @override
  Widget build(BuildContext context) {
    final style = ZoaType.code.copyWith(
      fontSize: 17,
      letterSpacing: 1.1,
      color: spent ? ZoaColors.inkSoft : ZoaColors.forestDeep,
    );

    final groups = code.split('-');

    return Wrap(
      alignment: WrapAlignment.center,
      crossAxisAlignment: WrapCrossAlignment.center,
      children: [
        for (var i = 0; i < groups.length; i++) ...[
          // The separator is its own child so a line can break directly after it
          // rather than inside the group that follows.
          if (i > 0)
            Text('-', style: style.copyWith(color: ZoaColors.inkSoft)),
          Text(groups[i], style: style),
        ],
      ],
    );
  }
}

class _CopyButton extends StatefulWidget {
  const _CopyButton({required this.code});

  final String code;

  @override
  State<_CopyButton> createState() => _CopyButtonState();
}

class _CopyButtonState extends State<_CopyButton> {
  bool _copied = false;

  Future<void> _copy() async {
    await Clipboard.setData(ClipboardData(text: widget.code));
    if (!mounted) return;

    // Confirmed in place rather than with a snackbar: the snackbar would cover the
    // bottom of the card, which is where the code is.
    setState(() => _copied = true);
    await Future<void>.delayed(const Duration(seconds: 2));
    if (mounted) setState(() => _copied = false);
  }

  @override
  Widget build(BuildContext context) {
    return TextButton.icon(
      onPressed: _copy,
      icon: Icon(
        _copied ? Icons.check : Icons.copy_all_outlined,
        size: 16,
        color: ZoaColors.forestDeep,
      ),
      label: Text(
        _copied ? 'Copied' : 'Copy code',
        style: ZoaType.bodySm.copyWith(
          color: ZoaColors.forestDeep,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }
}

/// What to do with the code. Present because the user is the one who has to
/// explain this at the counter.
class _Instructions extends StatelessWidget {
  const _Instructions({required this.redemption});

  final Redemption redemption;

  @override
  Widget build(BuildContext context) {
    if (!redemption.isRedeemable) {
      return const SizedBox.shrink();
    }

    return ZoaCard(
      padding: const EdgeInsets.all(ZoaSpace.lg),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const ZoaKicker('At the till'),
          const SizedBox(height: ZoaSpace.sm),
          Text(
            'Show this screen to the cashier. They scan the QR or type the code '
            'into the Zoa partner app, and the discount applies to your bill.',
            style: ZoaType.bodySm,
          ),
          const SizedBox(height: ZoaSpace.sm),
          Text(
            'The code works once. Keep it in Rewards → Your codes until you use it.',
            style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
          ),
        ],
      ),
    );
  }
}
