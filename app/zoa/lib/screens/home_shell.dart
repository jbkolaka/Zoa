/// The signed-in shell: app bar, tab body, bottom navigation.
///
/// Bottom navigation rather than a drawer, so primary destinations stay in
/// thumb reach (UI/UX doc §1.5). The bar is hand-built rather than Flutter's
/// `NavigationBar` so its chrome matches the design system exactly and does not
/// depend on per-component theme classes that shift between SDK versions.
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_models.dart';
import '../state/auth_controller.dart';
import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';
import '../util/format.dart';
import '../widgets/brand_mark.dart';
import '../widgets/zoa_ui.dart';
import 'collector/collector_queue_screen.dart';
import 'home_screen.dart';
import 'new_submission_screen.dart';
import 'partner/verify_code_screen.dart';
import 'profile_screen.dart';
import 'voucher_catalogue_screen.dart';

/// The destinations a signed-in account can hold.
///
/// These were positional constants (`recycle = 1`) back when every account saw
/// the same first four tabs. A collector no longer does — it has no Recycle tab
/// — so selection is tracked by name instead: a fixed index would have quietly
/// selected Rewards for them, and shifted every tab after it.
/// The destinations a signed-in account can hold.
///
/// These were positional constants (`recycle = 1`) back when every account saw
/// the same first four tabs. A collector no longer does — it has no Recycle tab
/// — so selection is tracked by name instead: a fixed index would have quietly
/// selected Rewards for them, and shifted every tab after it.
enum ZoaTab { home, recycle, rewards, profile, queue, verify }

/// The destinations an account holds, in bar order.
///
/// Kept pure and outside the widget so the role rules can be tested on their
/// own. Pumping [HomeShell] would build all five or six screens behind the bar,
/// each of which wants its own registered controller and fires a request on
/// first build — so a widget test of this rule would be testing everything
/// except the rule.
List<ZoaTab> zoaTabsFor(ZoaUser? user) {
  // A collector verifies what other people hand over and does not log its own
  // recycling, so it gets no Recycle tab. Keyed to the exact role rather than
  // [ZoaUser.isCollector], because that getter deliberately includes admin —
  // inheriting here would strip the tab from the one account that has to drive
  // the whole demo, which is the reverse of what the inclusive getters are for.
  final isCollectorOnly = user?.isCollectorOnly ?? false;

  return [
    ZoaTab.home,
    if (!isCollectorOnly) ZoaTab.recycle,
    ZoaTab.rewards,
    ZoaTab.profile,
    // Collectors and partner staff get their working view as an extra
    // destination. The Implementation Plan explicitly allows both to be
    // role-gated screens in this app rather than separate tools.
    if (user?.isCollector ?? false) ZoaTab.queue,
    if (user?.isPartnerStaff ?? false) ZoaTab.verify,
  ];
}

class HomeShell extends StatefulWidget {
  const HomeShell({super.key});

  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends State<HomeShell> {
  ZoaTab _selected = ZoaTab.home;

  void _select(ZoaTab tab) {
    if (tab == _selected) return;
    setState(() => _selected = tab);
  }

  /// Destinations map to the screen inventory in UI/UX doc §2.
  Widget _screenFor(ZoaTab tab, {required bool canLogRecycling}) =>
      switch (tab) {
        // Nothing to log for a collector, so the Home CTA goes too — a null
        // callback drops the button rather than dimming it.
        ZoaTab.home => HomeScreen(
            onLogRecycling:
                canLogRecycling ? () => _select(ZoaTab.recycle) : null,
          ),
        ZoaTab.recycle => const NewSubmissionScreen(),
        ZoaTab.rewards => const VoucherCatalogueScreen(),
        ZoaTab.profile => const ProfileScreen(),
        ZoaTab.queue => const CollectorQueueScreen(),
        ZoaTab.verify => const VerifyCodeScreen(),
      };

  static _NavDestination _destinationFor(ZoaTab tab) => switch (tab) {
        ZoaTab.home =>
          const _NavDestination(label: 'Home', icon: Icons.home_outlined),
        ZoaTab.recycle => const _NavDestination(
            label: 'Recycle', icon: Icons.add_circle_outline),
        ZoaTab.rewards => const _NavDestination(
            label: 'Rewards', icon: Icons.confirmation_number_outlined),
        ZoaTab.profile =>
          const _NavDestination(label: 'Profile', icon: Icons.person_outline),
        ZoaTab.queue => const _NavDestination(
            label: 'Queue', icon: Icons.fact_check_outlined),
        ZoaTab.verify =>
          const _NavDestination(label: 'Verify', icon: Icons.qr_code_scanner),
      };

  @override
  Widget build(BuildContext context) {
    final user = context.watch<AuthController>().user;
    final tabs = zoaTabsFor(user);
    final canLogRecycling = tabs.contains(ZoaTab.recycle);

    // Losing a role while its tab is selected removes that tab from the list.
    // Fall back to Home rather than leaving the bar with nothing selected.
    final found = tabs.indexOf(_selected);
    final index = found < 0 ? 0 : found;

    return Scaffold(
      backgroundColor: ZoaColors.paper,
      appBar: const _ZoaAppBar(),
      body: IndexedStack(
        index: index,
        children: [
          for (final tab in tabs)
            _screenFor(tab, canLogRecycling: canLogRecycling),
        ],
      ),
      bottomNavigationBar: _ZoaBottomNav(
        index: index,
        destinations: [for (final tab in tabs) _destinationFor(tab)],
        onSelect: (i) => _select(tabs[i]),
      ),
    );
  }
}

/// App bar: brand mark, wordmark, and the live points balance.
class _ZoaAppBar extends StatelessWidget implements PreferredSizeWidget {
  const _ZoaAppBar();

  @override
  Size get preferredSize => const Size.fromHeight(60);

  @override
  Widget build(BuildContext context) {
    final user = context.watch<AuthController>().user;

    return Container(
      decoration: const BoxDecoration(
        // Matches the website's translucent sticky header, minus the blur.
        color: ZoaColors.paper,
        border: Border(bottom: BorderSide(color: ZoaColors.line)),
      ),
      child: SafeArea(
        bottom: false,
        child: Padding(
          padding: const EdgeInsets.symmetric(
            horizontal: ZoaSpace.gutter,
            vertical: ZoaSpace.md,
          ),
          child: Row(
            children: [
              const ZoaBrandMark(size: 28),
              const SizedBox(width: ZoaSpace.sm),
              Text('Zoa', style: ZoaType.h3.copyWith(fontSize: 20)),
              const Spacer(),
              if (user != null)
                // The balance stays visible on every tab: UI/UX doc §1.2 wants
                // points present, not something to go looking for.
                ZoaTag(
                  '${formatPoints(user.pointsBalance)} pts',
                  color: ZoaColors.goldDeep,
                  background: ZoaColors.goldWash,
                ),
            ],
          ),
        ),
      ),
    );
  }
}

/// One bottom-navigation destination.
class _NavDestination {
  const _NavDestination({required this.label, required this.icon});

  final String label;
  final IconData icon;
}

class _ZoaBottomNav extends StatelessWidget {
  const _ZoaBottomNav({
    required this.index,
    required this.destinations,
    required this.onSelect,
  });

  final int index;
  final List<_NavDestination> destinations;
  final ValueChanged<int> onSelect;

  /// Thin-stroke outlined icons throughout, matching the line-icon style of the
  /// website's Materials section. Each carries a label — never icon alone
  /// (UI/UX doc §4). The destinations themselves are built per role in
  /// [HomeShell], paired with their screens.
  @override
  Widget build(BuildContext context) {
    // The selected pill's side padding has to give way as destinations are added.
    // At four tabs there is room for the full 16 — which is what both a plain
    // recycler and a collector see (the collector trades Recycle for Queue). An
    // admin sees six (Home, Recycle, Rewards, Profile, Queue, Verify), which
    // leaves about 60dp per slot on a 360dp phone — and a 54dp pill in a 60dp
    // slot has nowhere to go. This is the demo account's own layout, so it
    // cannot be allowed to clip.
    final pillPadding = destinations.length > 5
        ? 8.0
        : destinations.length > 4
            ? 12.0
            : 16.0;

    return Container(
      decoration: const BoxDecoration(
        color: ZoaColors.paperCard,
        border: Border(top: BorderSide(color: ZoaColors.line)),
      ),
      child: SafeArea(
        top: false,
        child: Padding(
          padding: const EdgeInsets.symmetric(vertical: ZoaSpace.sm),
          child: Row(
            children: [
              for (var i = 0; i < destinations.length; i++)
                Expanded(
                  child: _NavItem(
                    destination: destinations[i],
                    selected: i == index,
                    pillPadding: pillPadding,
                    onTap: () => onSelect(i),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

class _NavItem extends StatelessWidget {
  const _NavItem({
    required this.destination,
    required this.selected,
    required this.pillPadding,
    required this.onTap,
  });

  final _NavDestination destination;
  final bool selected;

  /// Horizontal padding inside the selected pill, narrowed by the bar as the
  /// destination count grows.
  final double pillPadding;

  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final color = selected ? ZoaColors.forestDeep : ZoaColors.inkSoft;

    return Semantics(
      button: true,
      selected: selected,
      label: destination.label,
      child: InkWell(
        onTap: onTap,
        borderRadius: ZoaRadius.allMd,
        splashColor: ZoaColors.leafWash,
        highlightColor: ZoaColors.leafWash,
        child: Padding(
          padding: const EdgeInsets.symmetric(vertical: 6),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              // The selected pill is a second, non-colour selection cue, so the
              // state does not depend on colour alone (UI/UX doc §5).
              AnimatedContainer(
                duration: ZoaMotion.quick,
                curve: ZoaMotion.curve,
                padding: EdgeInsets.symmetric(horizontal: pillPadding, vertical: 5),
                decoration: BoxDecoration(
                  color: selected ? ZoaColors.leafWash : Colors.transparent,
                  borderRadius: ZoaRadius.allPill,
                ),
                child: Icon(destination.icon, size: 22, color: color),
              ),
              const SizedBox(height: 3),
              // Labels are never dropped in favour of icons alone (UI/UX doc §4),
              // so a narrow slot shrinks and ellipsises the text instead.
              Text(
                destination.label,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                textAlign: TextAlign.center,
                style: ZoaType.tag.copyWith(
                  color: color,
                  fontWeight: selected ? FontWeight.w600 : FontWeight.w400,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
