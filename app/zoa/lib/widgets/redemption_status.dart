/// Presentation for a redemption's state.
///
/// One place decides what "used" looks like, because the confirmation screen and
/// the history list must not drift: a code that reads as spent on one screen and
/// ready on the other is a code someone gets turned away with.
library;

import 'package:flutter/material.dart';

import '../api/api_models.dart';
import '../theme/zoa_colors.dart';

/// The colour and icon for a redemption's state.
///
/// Always shown alongside [Redemption.statusLabel] — the icon and the word are
/// what carry the meaning, so status never rests on colour alone (UI/UX §5).
class RedemptionLook {
  const RedemptionLook(this.color, this.icon);

  final Color color;
  final IconData icon;

  /// Resolves the look for a redemption.
  ///
  /// Expiry is judged against the clock rather than the stored status alone: the
  /// server only moves a row to `expired` when someone actually tries to verify
  /// it, so a code read from history can still say `issued` while being stale.
  factory RedemptionLook.of(Redemption redemption) {
    if (redemption.isUsed) {
      return const RedemptionLook(ZoaColors.statusUsed, Icons.task_alt);
    }
    if (!redemption.isRedeemable) {
      return const RedemptionLook(ZoaColors.statusError, Icons.event_busy);
    }
    return const RedemptionLook(ZoaColors.statusPending, Icons.schedule);
  }
}
