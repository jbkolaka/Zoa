/// Voucher Catalogue — browse partner discounts (Phase 3).
///
/// The screen answers one question first: what can I get *right now*. So the
/// balance sits at the top, the list is cheapest-first, and an unaffordable
/// voucher shows how far off it is rather than just being greyed out — the gap is
/// the motivation to recycle again, which is the whole point of the programme.
///
/// Affordability comes from the server (`affordable` per voucher) and is never
/// recomputed here: Phase 4 deducts points against the same comparison, so a
/// second opinion in the client would eventually enable a button the server
/// refuses.
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_models.dart';
import '../state/redemptions_controller.dart';
import '../state/vouchers_controller.dart';
import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';
import '../util/format.dart';
import '../widgets/fade_slide_in.dart';
import '../widgets/loop_seal.dart';
import '../widgets/zoa_empty_state.dart';
import '../widgets/zoa_ui.dart';
import 'my_redemptions_screen.dart';
import 'voucher_detail_screen.dart';

class VoucherCatalogueScreen extends StatefulWidget {
  const VoucherCatalogueScreen({super.key});

  @override
  State<VoucherCatalogueScreen> createState() => _VoucherCatalogueScreenState();
}

class _VoucherCatalogueScreenState extends State<VoucherCatalogueScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      context.read<VouchersController>().load();
      // Loaded alongside the catalogue so the "Your codes" card can state a real
      // count on first paint rather than appearing after a beat.
      context.read<RedemptionsController>().load();
    });
  }

  Future<void> _refresh() async {
    await context.read<VouchersController>().load();
    if (!mounted) return;
    await context.read<RedemptionsController>().load();
  }

  Future<void> _openCodes() async {
    await Navigator.of(context).push(
      MaterialPageRoute(builder: (_) => const MyRedemptionsScreen()),
    );
    if (!mounted) return;
    await _refresh();
  }

  Future<void> _openDetail(Voucher voucher) async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => VoucherDetailScreen(voucherId: voucher.id),
      ),
    );
    if (!mounted) return;
    // The balance may have moved while the detail screen was open — a collector
    // verifying, or a redemption just made — so affordability is re-read rather
    // than trusted from before the push.
    await _refresh();
  }

  @override
  Widget build(BuildContext context) {
    final vouchers = context.watch<VouchersController>();
    final activeCodes = context.watch<RedemptionsController>().activeCount;

    return RefreshIndicator(
      onRefresh: _refresh,
      color: ZoaColors.forestDeep,
      backgroundColor: ZoaColors.paperCard,
      child: ZoaPage(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: FadeSlideIn.staggered([
            const SizedBox(height: ZoaSpace.sm),
            const ZoaEyebrow('Rewards'),
            const SizedBox(height: ZoaSpace.md),
            Text('Spend your\npoints', style: ZoaType.h2),
            const SizedBox(height: ZoaSpace.md),
            Text(
              'Discounts at partner shops. Redeem for a code and show it at the '
              'till — no cash changes hands.',
              style: ZoaType.bodySoft,
            ),
            const SizedBox(height: ZoaSpace.xl),
            _BalanceStrip(balance: vouchers.pointsBalance),
            const SizedBox(height: ZoaSpace.md),
            _YourCodesCard(activeCount: activeCodes, onTap: _openCodes),
            const SizedBox(height: ZoaSpace.lg),
            _Filters(
              affordableOnly: vouchers.affordableOnly,
              partnerId: vouchers.partnerId,
              partners: vouchers.partners,
              enabled: !vouchers.loading,
              onAffordableChanged: (value) =>
                  context.read<VouchersController>().setAffordableOnly(value),
              onPartnerChanged: (id) =>
                  context.read<VouchersController>().setPartner(id),
            ),
            const SizedBox(height: ZoaSpace.lg),
            ..._body(context, vouchers),
            const SizedBox(height: ZoaSpace.xl),
          ]),
        ),
      ),
    );
  }

  List<Widget> _body(BuildContext context, VouchersController vouchers) {
    // Only a first load blocks: a filter change keeps the old list on screen
    // underneath, so toggling a chip does not flash the whole page away.
    if (vouchers.loading && vouchers.vouchers.isEmpty) {
      return [
        const SizedBox(height: ZoaSpace.xl),
        const Center(child: CircularProgressIndicator(color: ZoaColors.leaf)),
      ];
    }

    final error = vouchers.error;
    if (error != null && vouchers.vouchers.isEmpty) {
      return [
        ZoaErrorState(
          title: 'Could not load rewards',
          message: error.message,
          onRetry: () => context.read<VouchersController>().load(),
        ),
      ];
    }

    if (vouchers.vouchers.isEmpty) {
      // The two empty cases are genuinely different and must not share copy: a
      // filter hiding everything is the user's own doing and reversible, while an
      // empty catalogue is ours to fix.
      return [
        ZoaEmptyState(
          title: vouchers.filtered ? 'Nothing matches yet' : 'No rewards yet',
          blurb: vouchers.filtered
              ? 'Clear the filters to see the full list, or recycle a little '
                  'more to unlock these.'
              : 'Partner discounts will appear here as they are added.',
          stage: LoopStage.redeem,
        ),
      ];
    }

    return [
      for (final voucher in vouchers.vouchers) ...[
        _VoucherCard(
          voucher: voucher,
          balance: vouchers.pointsBalance,
          onTap: () => _openDetail(voucher),
        ),
        const SizedBox(height: ZoaSpace.md),
      ],
    ];
  }
}

/// The balance, stated once at the top so every affordability verdict below has
/// a visible reason.
class _BalanceStrip extends StatelessWidget {
  const _BalanceStrip({required this.balance});

  final int balance;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(
        horizontal: ZoaSpace.lg,
        vertical: ZoaSpace.md,
      ),
      decoration: BoxDecoration(
        color: ZoaColors.forestDeep,
        borderRadius: ZoaRadius.allMd,
      ),
      child: Row(
        children: [
          const Icon(Icons.stars_rounded, color: ZoaColors.gold, size: 22),
          const SizedBox(width: ZoaSpace.md),
          Expanded(
            child: Text(
              'Your balance',
              style: ZoaType.bodySm.copyWith(color: ZoaColors.paper),
            ),
          ),
          Text(
            formatPoints(balance),
            style: ZoaType.pointsHero.copyWith(
              color: ZoaColors.gold,
              fontSize: 24,
            ),
          ),
          const SizedBox(width: ZoaSpace.xs),
          Text(
            'pts',
            style: ZoaType.tag.copyWith(color: ZoaColors.paper),
          ),
        ],
      ),
    );
  }
}

/// Way through to the codes the user is already holding.
///
/// Always present, not only when there is something to show: a code that has been
/// redeemed and not yet spent is the most time-sensitive thing in the app, and it
/// must never be somewhere the user has to remember how to find.
class _YourCodesCard extends StatelessWidget {
  const _YourCodesCard({required this.activeCount, required this.onTap});

  /// Codes still spendable — expiry included, so nothing is advertised as ready
  /// when it has quietly lapsed.
  final int activeCount;

  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final has = activeCount > 0;

    return Semantics(
      button: true,
      label: has
          ? 'Your codes, $activeCount ready to use'
          : 'Your codes, none active',
      child: ZoaCard(
        onTap: onTap,
        accent: has,
        padding: const EdgeInsets.all(ZoaSpace.lg),
        child: Row(
          children: [
            Icon(
              Icons.confirmation_number_outlined,
              size: 20,
              color: has ? ZoaColors.goldDeep : ZoaColors.inkSoft,
            ),
            const SizedBox(width: ZoaSpace.md),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Your codes', style: ZoaType.label),
                  const SizedBox(height: 2),
                  Text(
                    has
                        ? '$activeCount ready to use at a till'
                        : 'Nothing active — redeem a reward below',
                    style: ZoaType.bodySm,
                  ),
                ],
              ),
            ),
            const Icon(Icons.chevron_right, size: 20, color: ZoaColors.inkSoft),
          ],
        ),
      ),
    );
  }
}

/// Affordability toggle and partner filter.
class _Filters extends StatelessWidget {
  const _Filters({
    required this.affordableOnly,
    required this.partnerId,
    required this.partners,
    required this.enabled,
    required this.onAffordableChanged,
    required this.onPartnerChanged,
  });

  final bool affordableOnly;
  final int? partnerId;
  final List<VoucherPartner> partners;
  final bool enabled;
  final ValueChanged<bool> onAffordableChanged;
  final ValueChanged<int?> onPartnerChanged;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        FilterChip(
          label: const Text('Only what I can afford'),
          selected: affordableOnly,
          onSelected: enabled ? onAffordableChanged : null,
          showCheckmark: true,
          checkmarkColor: ZoaColors.forestDeep,
          backgroundColor: ZoaColors.paperCard,
          selectedColor: ZoaColors.leafWash,
          labelStyle: ZoaType.bodySm.copyWith(
            color: affordableOnly ? ZoaColors.forestDeep : ZoaColors.inkSoft,
            fontWeight: affordableOnly ? FontWeight.w600 : FontWeight.w400,
          ),
          side: BorderSide(
            color: affordableOnly ? ZoaColors.leaf : ZoaColors.line,
            width: affordableOnly ? 1.6 : 1,
          ),
        ),
        if (partners.length > 1) ...[
          const SizedBox(height: ZoaSpace.sm),
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            // The partner row scrolls rather than wraps: partner count grows with
            // the programme, and a wrapping row would push the catalogue itself
            // off the first screen.
            child: Row(
              children: [
                _PartnerChip(
                  label: 'All shops',
                  selected: partnerId == null,
                  enabled: enabled,
                  onTap: () => onPartnerChanged(null),
                ),
                for (final partner in partners) ...[
                  const SizedBox(width: ZoaSpace.sm),
                  _PartnerChip(
                    label: partner.name,
                    selected: partnerId == partner.id,
                    enabled: enabled,
                    onTap: () => onPartnerChanged(partner.id),
                  ),
                ],
              ],
            ),
          ),
        ],
      ],
    );
  }
}

class _PartnerChip extends StatelessWidget {
  const _PartnerChip({
    required this.label,
    required this.selected,
    required this.enabled,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final bool enabled;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return ChoiceChip(
      label: Text(label),
      selected: selected,
      onSelected: enabled ? (_) => onTap() : null,
      showCheckmark: false,
      backgroundColor: ZoaColors.paperCard,
      selectedColor: ZoaColors.leafWash,
      labelStyle: ZoaType.bodySm.copyWith(
        color: selected ? ZoaColors.forestDeep : ZoaColors.inkSoft,
        fontWeight: selected ? FontWeight.w600 : FontWeight.w400,
        fontSize: 12.5,
      ),
      side: BorderSide(
        color: selected ? ZoaColors.leaf : ZoaColors.line,
        width: selected ? 1.6 : 1,
      ),
    );
  }
}

/// One voucher.
///
/// An unaffordable voucher is dimmed but still tappable, and shows the shortfall
/// with a progress bar. Blocking the tap would hide the detail — and the gap is
/// the reason to go recycle again.
class _VoucherCard extends StatelessWidget {
  const _VoucherCard({
    required this.voucher,
    required this.balance,
    required this.onTap,
  });

  final Voucher voucher;
  final int balance;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final affordable = voucher.affordable;
    final shortfall = voucher.shortfallFrom(balance);

    return Semantics(
      button: true,
      label: '${voucher.title} at ${voucher.partner.name}, '
          '${voucher.pointsCost} points'
          '${affordable ? ', you can afford this' : ', $shortfall points short'}',
      child: Material(
        color: ZoaColors.paperCard,
        borderRadius: ZoaRadius.allMd,
        child: InkWell(
          onTap: onTap,
          borderRadius: ZoaRadius.allMd,
          splashColor: ZoaColors.leafWash,
          child: Container(
            padding: const EdgeInsets.all(ZoaSpace.lg),
            decoration: BoxDecoration(
              borderRadius: ZoaRadius.allMd,
              border: Border.all(
                color: affordable ? ZoaColors.leaf : ZoaColors.line,
                width: affordable ? 1.6 : 1,
              ),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    _PartnerBadge(partner: voucher.partner, dimmed: !affordable),
                    const SizedBox(width: ZoaSpace.md),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            voucher.partner.name,
                            style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft),
                          ),
                          const SizedBox(height: 2),
                          Text(
                            voucher.title,
                            style: ZoaType.label.copyWith(
                              color: affordable
                                  ? ZoaColors.forestDeep
                                  : ZoaColors.inkSoft,
                            ),
                          ),
                        ],
                      ),
                    ),
                    const SizedBox(width: ZoaSpace.sm),
                    Column(
                      crossAxisAlignment: CrossAxisAlignment.end,
                      children: [
                        Text(
                          formatPoints(voucher.pointsCost),
                          style: ZoaType.pointsCost.copyWith(
                            fontSize: 15,
                            color: affordable
                                ? ZoaColors.forestDeep
                                : ZoaColors.inkSoft,
                          ),
                        ),
                        Text('pts', style: ZoaType.tag.copyWith(fontSize: 10)),
                      ],
                    ),
                  ],
                ),
                const SizedBox(height: ZoaSpace.md),
                Row(
                  children: [
                    _Pill(
                      label: voucher.discountLabel,
                      emphasised: affordable,
                    ),
                    if (voucher.scarcityLabel != null) ...[
                      const SizedBox(width: ZoaSpace.sm),
                      _Pill(label: voucher.scarcityLabel!, warning: true),
                    ],
                    const Spacer(),
                    if (affordable)
                      Row(
                        children: [
                          const Icon(Icons.check_circle,
                              size: 14, color: ZoaColors.leaf),
                          const SizedBox(width: 4),
                          Text(
                            'Ready',
                            style: ZoaType.bodySm.copyWith(
                              color: ZoaColors.leaf,
                              fontWeight: FontWeight.w600,
                              fontSize: 12,
                            ),
                          ),
                        ],
                      ),
                  ],
                ),
                if (!affordable) ...[
                  const SizedBox(height: ZoaSpace.md),
                  _ShortfallBar(
                    progress: voucher.progressFrom(balance),
                    shortfall: shortfall,
                  ),
                ],
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// Lettermark for a partner, since no logo assets exist yet.
class _PartnerBadge extends StatelessWidget {
  const _PartnerBadge({required this.partner, required this.dimmed});

  final VoucherPartner partner;
  final bool dimmed;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 40,
      height: 40,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: dimmed ? ZoaColors.paper : ZoaColors.leafWash,
        borderRadius: ZoaRadius.allSm,
        border: Border.all(color: dimmed ? ZoaColors.line : ZoaColors.leaf),
      ),
      child: Text(
        partner.initials,
        style: ZoaType.label.copyWith(
          fontSize: 14,
          color: dimmed ? ZoaColors.inkSoft : ZoaColors.forestDeep,
        ),
      ),
    );
  }
}

class _Pill extends StatelessWidget {
  const _Pill({
    required this.label,
    this.emphasised = false,
    this.warning = false,
  });

  final String label;
  final bool emphasised;
  final bool warning;

  @override
  Widget build(BuildContext context) {
    final Color background;
    final Color foreground;

    if (warning) {
      background = ZoaColors.gold.withValues(alpha: 0.16);
      foreground = ZoaColors.inkSoft;
    } else if (emphasised) {
      background = ZoaColors.leafWash;
      foreground = ZoaColors.forestDeep;
    } else {
      background = ZoaColors.paper;
      foreground = ZoaColors.inkSoft;
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: ZoaSpace.sm, vertical: 3),
      decoration: BoxDecoration(
        color: background,
        borderRadius: ZoaRadius.allSm,
      ),
      child: Text(
        label,
        style: ZoaType.bodySm.copyWith(
          fontSize: 11.5,
          color: foreground,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }
}

/// How far off an unaffordable voucher is.
class _ShortfallBar extends StatelessWidget {
  const _ShortfallBar({required this.progress, required this.shortfall});

  final double progress;
  final int shortfall;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        ClipRRect(
          borderRadius: BorderRadius.circular(3),
          child: LinearProgressIndicator(
            value: progress,
            minHeight: 5,
            backgroundColor: ZoaColors.paper,
            valueColor: const AlwaysStoppedAnimation(ZoaColors.gold),
          ),
        ),
        const SizedBox(height: 5),
        Text(
          '${formatPoints(shortfall)} points to go',
          style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft, fontSize: 11.5),
        ),
      ],
    );
  }
}
