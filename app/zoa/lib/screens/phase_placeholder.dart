/// Placeholder for destinations whose phase has not been built yet.
///
/// Deliberately not a blank screen: it names the phase and describes what will
/// land there, so the navigation is walkable end to end from Phase 0 and the
/// build order stays visible while demoing.
library;

import 'package:flutter/material.dart';

import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';
import '../widgets/fade_slide_in.dart';
import '../widgets/loop_seal.dart';
import '../widgets/zoa_ui.dart';

class PhasePlaceholder extends StatelessWidget {
  const PhasePlaceholder({
    super.key,
    required this.phase,
    required this.title,
    required this.blurb,
    this.stage,
  });

  /// e.g. "Phase 2".
  final String phase;

  final String title;
  final String blurb;

  /// Which stage of the loop this screen belongs to — highlighted on the seal.
  final LoopStage? stage;

  @override
  Widget build(BuildContext context) {
    return ZoaPage(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: FadeSlideIn.staggered([
          const SizedBox(height: ZoaSpace.lg),
          Center(child: LoopSeal(size: 190, highlightStage: stage)),
          const SizedBox(height: ZoaSpace.xxl),
          ZoaEyebrow('$phase · not yet built'),
          const SizedBox(height: ZoaSpace.md),
          Text(title, style: ZoaType.h2),
          const SizedBox(height: ZoaSpace.md),
          Text(blurb, style: ZoaType.bodySoft),
          const SizedBox(height: ZoaSpace.xl),
          const _LoopReminder(),
        ]),
      ),
    );
  }
}

/// Restates the loop the platform is built around. Present on every unbuilt
/// screen so the core story stays legible even mid-build.
class _LoopReminder extends StatelessWidget {
  const _LoopReminder();

  @override
  Widget build(BuildContext context) {
    const steps = [
      ('01', 'Log & photograph', 'Material, weight, source — home or hotel.'),
      ('02', 'AI suggests, human confirms', 'A collector verifies type and weight on pickup.'),
      ('03', 'Points, credited', 'Verified weight × material rate, in an auditable ledger.'),
      ('04', 'Redeem for real discounts', 'A unique code, shown at partner checkout.'),
    ];

    return ZoaCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const ZoaKicker('The loop'),
          const SizedBox(height: ZoaSpace.md),
          for (final (number, title, detail) in steps) ...[
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                SizedBox(
                  width: 28,
                  child: Text(
                    number,
                    style: ZoaType.tag.copyWith(color: ZoaColors.goldDeep),
                  ),
                ),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(title, style: ZoaType.label),
                      const SizedBox(height: 2),
                      Text(detail, style: ZoaType.bodySm),
                    ],
                  ),
                ),
              ],
            ),
            if (number != '04') const SizedBox(height: ZoaSpace.md),
          ],
        ],
      ),
    );
  }
}
