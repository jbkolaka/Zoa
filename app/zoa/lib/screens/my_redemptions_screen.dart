/// My Redemptions — every code the user has taken out, and its state (Phase 4).
///
/// Reached from the "Your codes" card on the Rewards tab. Newest first, because a
/// code just redeemed is the one about to be used, and a live code is visually
/// distinct from a spent one: the status is an icon plus a word plus a date, never
/// a colour on its own (UI/UX §5).
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_models.dart';
import '../state/redemptions_controller.dart';
import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';
import '../util/format.dart';
import '../widgets/fade_slide_in.dart';
import '../widgets/loop_seal.dart';
import '../widgets/redemption_status.dart';
import '../widgets/zoa_empty_state.dart';
import '../widgets/zoa_ui.dart';
import 'redemption_confirmation_screen.dart';

class MyRedemptionsScreen extends StatefulWidget {
  const MyRedemptionsScreen({super.key});

  @override
  State<MyRedemptionsScreen> createState() => _MyRedemptionsScreenState();
}

class _MyRedemptionsScreenState extends State<MyRedemptionsScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) context.read<RedemptionsController>().load();
    });
  }

  Future<void> _open(Redemption redemption) async {
    final voucher = redemption.voucher;
    // The listing always embeds the voucher; without it there is no partner name
    // or discount to show, so there is nothing worth opening.
    if (voucher == null) return;

    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => RedemptionConfirmationScreen(
          redemption: redemption,
          voucher: voucher,
        ),
      ),
    );
    if (!mounted) return;
    // A partner may have accepted the code while it was on screen, so the status
    // is re-read rather than trusted from before the push.
    await context.read<RedemptionsController>().load();
  }

  @override
  Widget build(BuildContext context) {
    final redemptions = context.watch<RedemptionsController>();

    return Scaffold(
      backgroundColor: ZoaColors.paper,
      appBar: AppBar(
        backgroundColor: ZoaColors.paper,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        foregroundColor: ZoaColors.forestDeep,
        title: Text('Your codes', style: ZoaType.label),
      ),
      body: SafeArea(
        child: RefreshIndicator(
          onRefresh: () => context.read<RedemptionsController>().load(),
          color: ZoaColors.forestDeep,
          backgroundColor: ZoaColors.paperCard,
          child: _body(context, redemptions),
        ),
      ),
    );
  }

  Widget _body(BuildContext context, RedemptionsController redemptions) {
    if (redemptions.loading && redemptions.mine.isEmpty) {
      return const Center(child: CircularProgressIndicator(color: ZoaColors.leaf));
    }

    final error = redemptions.error;
    if (error != null && redemptions.mine.isEmpty) {
      return ZoaErrorState(
        title: 'Could not load your codes',
        message: error.message,
        onRetry: () => context.read<RedemptionsController>().load(),
      );
    }

    if (redemptions.mine.isEmpty) {
      return ZoaEmptyState(
        title: 'No codes yet',
        blurb: 'Redeem a reward and its code appears here, ready to show at the '
            'till.',
        stage: LoopStage.redeem,
      );
    }

    // Live codes first: a spent code is a receipt, and a receipt should not sit
    // above the thing the user actually needs to find at a counter.
    final live = redemptions.mine.where((r) => r.isRedeemable).toList();
    final past = redemptions.mine.where((r) => !r.isRedeemable).toList();

    return ZoaPage(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: FadeSlideIn.staggered([
          if (live.isNotEmpty) ...[
            const ZoaKicker('Ready to use'),
            const SizedBox(height: ZoaSpace.md),
            for (final redemption in live) ...[
              _RedemptionCard(redemption: redemption, onTap: () => _open(redemption)),
              const SizedBox(height: ZoaSpace.md),
            ],
          ],
          if (past.isNotEmpty) ...[
            if (live.isNotEmpty) const SizedBox(height: ZoaSpace.lg),
            const ZoaKicker('Used & expired'),
            const SizedBox(height: ZoaSpace.md),
            for (final redemption in past) ...[
              _RedemptionCard(redemption: redemption, onTap: () => _open(redemption)),
              const SizedBox(height: ZoaSpace.md),
            ],
          ],
          const SizedBox(height: ZoaSpace.xl),
        ]),
      ),
    );
  }
}

/// One code in the list.
class _RedemptionCard extends StatelessWidget {
  const _RedemptionCard({required this.redemption, required this.onTap});

  final Redemption redemption;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final voucher = redemption.voucher;
    final live = redemption.isRedeemable;
    final look = RedemptionLook.of(redemption);

    return Semantics(
      button: true,
      label: '${voucher?.title ?? 'Reward'} at ${voucher?.partner.name ?? 'a partner'}, '
          '${redemption.statusLabel}',
      child: ZoaCard(
        onTap: onTap,
        padding: const EdgeInsets.all(ZoaSpace.lg),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        voucher?.partner.name.toUpperCase() ?? 'PARTNER',
                        style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        voucher?.title ?? 'Reward',
                        style: ZoaType.label.copyWith(
                          color: live ? ZoaColors.forestDeep : ZoaColors.inkSoft,
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(width: ZoaSpace.sm),
                Icon(look.icon, size: 18, color: look.color),
              ],
            ),
            const SizedBox(height: ZoaSpace.md),
            Row(
              children: [
                // Only the leading block of the code: the full UUID does not fit a
                // list row, and this is for recognising which code is which, not
                // for reading out at a till.
                Text(
                  redemption.shortCode,
                  style: ZoaType.mono.copyWith(
                    fontSize: 13,
                    color: live ? ZoaColors.ink : ZoaColors.inkSoft,
                  ),
                ),
                const Spacer(),
                Text(
                  redemption.statusLabel,
                  style: ZoaType.bodySm.copyWith(
                    color: look.color,
                    fontWeight: FontWeight.w600,
                    fontSize: 12.5,
                  ),
                ),
              ],
            ),
            const SizedBox(height: ZoaSpace.xs),
            Text(
              _detail(redemption),
              style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft, fontSize: 12.5),
            ),
          ],
        ),
      ),
    );
  }

  static String _detail(Redemption redemption) {
    if (redemption.isUsed) {
      final usedAt = redemption.usedAt;
      return usedAt == null ? 'Accepted at a till' : 'Accepted ${formatDate(usedAt)}';
    }
    if (!redemption.isRedeemable) {
      return 'Expired ${formatDate(redemption.expiry)}';
    }

    final days = redemption.daysUntilExpiry;
    // Counted in days while that is still useful, then dated. "0 days left" reads
    // as broken; "expires today" does not.
    if (days <= 0) return 'Expires today';
    if (days == 1) return 'Expires tomorrow';
    if (days <= 7) return 'Expires in $days days';
    return 'Valid until ${formatDate(redemption.expiry)}';
  }
}
