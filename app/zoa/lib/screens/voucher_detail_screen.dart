/// Voucher Detail — one reward, and whether it can be claimed yet (Phase 3),
/// plus the redemption itself (Phase 4).
///
/// Affordability is the server's `affordable` flag and is never recomputed here:
/// the redemption deducts against the same comparison, so a second opinion in the
/// client would eventually enable a button the server refuses.
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_models.dart';
import '../state/auth_controller.dart';
import '../state/redemptions_controller.dart';
import '../state/vouchers_controller.dart';
import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';
import '../util/format.dart';
import '../widgets/fade_slide_in.dart';
import '../widgets/zoa_empty_state.dart';
import '../widgets/zoa_ui.dart';
import 'redemption_confirmation_screen.dart';

class VoucherDetailScreen extends StatefulWidget {
  const VoucherDetailScreen({super.key, required this.voucherId});

  final int voucherId;

  @override
  State<VoucherDetailScreen> createState() => _VoucherDetailScreenState();
}

class _VoucherDetailScreenState extends State<VoucherDetailScreen> {
  Voucher? _voucher;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _load());
  }

  Future<void> _load() async {
    final loaded =
        await context.read<VouchersController>().refreshOne(widget.voucherId);

    if (!mounted) return;
    setState(() {
      // Keep the last good voucher if a refresh fails; only the first load is
      // allowed to leave this null and fall through to the error state.
      if (loaded != null || _voucher == null) _voucher = loaded;
      _loading = false;
    });
  }

  /// Confirms, redeems, and shows the code.
  ///
  /// The confirmation step exists because this spends a balance the user worked
  /// for and cannot undo — points are not refundable once a code is issued.
  Future<void> _redeem(Voucher voucher) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => _ConfirmRedeemDialog(voucher: voucher),
    );
    if (confirmed != true || !mounted) return;

    final redemptions = context.read<RedemptionsController>();
    final result = await redemptions.redeem(voucher.id);

    if (!mounted) return;

    if (result == null) {
      // Nothing was spent — the server's transaction refuses before deducting —
      // so the honest move is to say why and re-read the voucher, which may now be
      // out of stock or unaffordable.
      final message = redemptions.error?.message ?? 'Could not redeem this reward.';
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(message, style: ZoaType.bodySm.copyWith(color: ZoaColors.paper)),
          backgroundColor: ZoaColors.forestDeep,
        ),
      );
      await _load();
      return;
    }

    // Two refreshes, because they answer different questions. `GET /vouchers/:id`
    // returns a voucher but no balance, so reloading the catalogue is the only way
    // to move the "You have" figure this screen shows; reloading the single voucher
    // is what picks up the stock it just consumed. AuthController owns the balance
    // in the app bar, which is a third copy.
    await context.read<AuthController>().refresh();
    if (!mounted) return;
    await context.read<VouchersController>().load();
    if (!mounted) return;

    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => RedemptionConfirmationScreen(
          redemption: result.redemption,
          voucher: result.voucher,
          pointsSpent: result.pointsSpent,
          pointsBalance: result.pointsBalance,
        ),
      ),
    );
    if (!mounted) return;
    await _load();
  }

  @override
  Widget build(BuildContext context) {
    final balance = context.watch<VouchersController>().pointsBalance;

    return Scaffold(
      backgroundColor: ZoaColors.paper,
      appBar: AppBar(
        backgroundColor: ZoaColors.paper,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        foregroundColor: ZoaColors.forestDeep,
        title: Text('Reward', style: ZoaType.label),
      ),
      body: SafeArea(child: _body(context, balance)),
    );
  }

  Widget _body(BuildContext context, int balance) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator(color: ZoaColors.leaf));
    }

    final voucher = _voucher;
    if (voucher == null) {
      return ZoaErrorState(
        title: 'Could not load this reward',
        message: context.read<VouchersController>().error?.message ??
            'This reward is no longer available.',
        onRetry: _load,
      );
    }

    final affordable = voucher.affordable;
    final shortfall = voucher.shortfallFrom(balance);

    return RefreshIndicator(
      onRefresh: _load,
      color: ZoaColors.forestDeep,
      backgroundColor: ZoaColors.paperCard,
      child: ZoaPage(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: FadeSlideIn.staggered([
            const SizedBox(height: ZoaSpace.sm),
            Text(
              voucher.partner.name.toUpperCase(),
              style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft),
            ),
            const SizedBox(height: ZoaSpace.sm),
            Text(voucher.title, style: ZoaType.h2),
            const SizedBox(height: ZoaSpace.md),
            Text(
              voucher.discountLabel,
              style: ZoaType.label.copyWith(color: ZoaColors.leaf),
            ),
            const SizedBox(height: ZoaSpace.xl),
            _CostPanel(
              voucher: voucher,
              balance: balance,
              affordable: affordable,
              shortfall: shortfall,
            ),
            const SizedBox(height: ZoaSpace.lg),
            _DetailRows(voucher: voucher),
            const SizedBox(height: ZoaSpace.xl),
            _RedeemAction(
              affordable: affordable,
              shortfall: shortfall,
              onRedeem: () => _redeem(voucher),
            ),
            const SizedBox(height: ZoaSpace.xl),
          ]),
        ),
      ),
    );
  }
}

/// Cost against balance — the one comparison the screen exists to make.
class _CostPanel extends StatelessWidget {
  const _CostPanel({
    required this.voucher,
    required this.balance,
    required this.affordable,
    required this.shortfall,
  });

  final Voucher voucher;
  final int balance;
  final bool affordable;
  final int shortfall;

  @override
  Widget build(BuildContext context) {
    return ZoaCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Costs', style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft)),
                  const SizedBox(height: 2),
                  Row(
                    crossAxisAlignment: CrossAxisAlignment.end,
                    children: [
                      Text(
                        formatPoints(voucher.pointsCost),
                        style: ZoaType.pointsHero.copyWith(fontSize: 28),
                      ),
                      const SizedBox(width: 4),
                      Padding(
                        padding: const EdgeInsets.only(bottom: 4),
                        child: Text('pts', style: ZoaType.tag),
                      ),
                    ],
                  ),
                ],
              ),
              const Spacer(),
              Column(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  Text('You have', style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft)),
                  const SizedBox(height: 2),
                  Text(
                    '${formatPoints(balance)} pts',
                    style: ZoaType.pointsCost.copyWith(
                      fontSize: 15,
                      color: affordable ? ZoaColors.leaf : ZoaColors.inkSoft,
                    ),
                  ),
                ],
              ),
            ],
          ),
          if (!affordable) ...[
            const SizedBox(height: ZoaSpace.md),
            ClipRRect(
              borderRadius: BorderRadius.circular(3),
              child: LinearProgressIndicator(
                value: voucher.progressFrom(balance),
                minHeight: 6,
                backgroundColor: ZoaColors.paper,
                valueColor: const AlwaysStoppedAnimation(ZoaColors.gold),
              ),
            ),
            const SizedBox(height: ZoaSpace.sm),
            Text(
              // Framed as a concrete next step, not a scolding: PET pays 25/kg, so
              // the shortfall is also a rough weight the user can act on.
              '${formatPoints(shortfall)} points to go — about '
              '${(shortfall / 25).ceil()} kg more of PET bottles.',
              style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
            ),
          ],
        ],
      ),
    );
  }
}

class _DetailRows extends StatelessWidget {
  const _DetailRows({required this.voucher});

  final Voucher voucher;

  @override
  Widget build(BuildContext context) {
    final stock = voucher.stockRemaining;

    return ZoaCard(
      child: Column(
        children: [
          _row('Discount', voucher.discountLabel),
          const Divider(height: ZoaSpace.lg, color: ZoaColors.line),
          _row('Valid for', '${voucher.expiryDays} days after you redeem'),
          const Divider(height: ZoaSpace.lg, color: ZoaColors.line),
          // Unlimited stock is stated as such rather than shown as a number:
          // stock_remaining is null for these, and rendering that as "0" would be
          // exactly backwards.
          _row('Availability', stock == null ? 'Always available' : '$stock left'),
        ],
      ),
    );
  }

  Widget _row(String label, String value) {
    return Row(
      children: [
        Expanded(
          child: Text(label, style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft)),
        ),
        Flexible(
          child: Text(
            value,
            style: ZoaType.bodySm,
            textAlign: TextAlign.right,
          ),
        ),
      ],
    );
  }
}

/// The redeem button.
///
/// Two states, and the copy differs for each so the reason a tap is unavailable is
/// always on screen. When the user cannot afford it, the shortfall replaces the
/// label rather than the button simply greying out — a disabled control with no
/// explanation is the thing this screen exists to avoid.
class _RedeemAction extends StatelessWidget {
  const _RedeemAction({
    required this.affordable,
    required this.shortfall,
    required this.onRedeem,
  });

  final bool affordable;
  final int shortfall;
  final VoidCallback onRedeem;

  @override
  Widget build(BuildContext context) {
    if (!affordable) {
      return Column(
        children: [
          ZoaPrimaryButton(
            label: '${formatPoints(shortfall)} points short',
            icon: Icons.lock_outline,
            onPressed: null,
          ),
          const SizedBox(height: ZoaSpace.sm),
          Text(
            'Log another collection to close the gap.',
            style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
            textAlign: TextAlign.center,
          ),
        ],
      );
    }

    // Watched rather than read: the button has to spin while the redemption
    // transaction is in flight, since a second tap would be a second request.
    final submitting = context.watch<RedemptionsController>().submitting;

    return Column(
      children: [
        ZoaPrimaryButton(
          label: 'Redeem this reward',
          icon: Icons.confirmation_number_outlined,
          loading: submitting,
          onPressed: submitting ? null : onRedeem,
        ),
        const SizedBox(height: ZoaSpace.sm),
        Text(
          'You get a code and a QR to show at the till. Points are spent when the '
          'code is issued, not when you use it.',
          style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
          textAlign: TextAlign.center,
        ),
      ],
    );
  }
}

/// Last check before points leave the balance.
///
/// Spells out the cost and what is left, because this is not reversible: once a
/// code is issued the points are gone whether or not the code is ever presented.
class _ConfirmRedeemDialog extends StatelessWidget {
  const _ConfirmRedeemDialog({required this.voucher});

  final Voucher voucher;

  @override
  Widget build(BuildContext context) {
    final balance = context.read<VouchersController>().pointsBalance;

    return AlertDialog(
      backgroundColor: ZoaColors.paperCard,
      surfaceTintColor: Colors.transparent,
      shape: const RoundedRectangleBorder(borderRadius: ZoaRadius.allMd),
      title: Text('Redeem this reward?', style: ZoaType.h3),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            '${voucher.title} at ${voucher.partner.name}.',
            style: ZoaType.bodySm.copyWith(color: ZoaColors.ink),
          ),
          const SizedBox(height: ZoaSpace.md),
          Row(
            children: [
              Text('Costs', style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft)),
              const Spacer(),
              Text('${formatPoints(voucher.pointsCost)} pts', style: ZoaType.pointsCost),
            ],
          ),
          const SizedBox(height: ZoaSpace.xs),
          Row(
            children: [
              Text('Left after', style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft)),
              const Spacer(),
              Text(
                '${formatPoints(balance - voucher.pointsCost)} pts',
                style: ZoaType.mono.copyWith(fontSize: 13, color: ZoaColors.ink),
              ),
            ],
          ),
          const SizedBox(height: ZoaSpace.md),
          Text(
            'The code is valid for ${voucher.expiryDays} days and can be used once. '
            'Points are not refundable.',
            style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
          ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(false),
          child: Text(
            'Not yet',
            style: ZoaType.bodySm.copyWith(
              color: ZoaColors.inkSoft,
              fontWeight: FontWeight.w600,
            ),
          ),
        ),
        TextButton(
          onPressed: () => Navigator.of(context).pop(true),
          child: Text(
            'Redeem',
            style: ZoaType.bodySm.copyWith(
              color: ZoaColors.forestDeep,
              fontWeight: FontWeight.w700,
            ),
          ),
        ),
      ],
    );
  }
}
