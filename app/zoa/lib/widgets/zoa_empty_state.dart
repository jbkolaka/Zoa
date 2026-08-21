/// The shared empty state, built around the loop seal.
///
/// The design brief asks for the seal to do real work in the app — a branded
/// empty state is one of the three places it names. Used by any list that can
/// legitimately have nothing in it (no submissions yet, no redemptions yet).
library;

import 'package:flutter/material.dart';

import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';
import 'fade_slide_in.dart';
import 'loop_seal.dart';
import 'zoa_ui.dart';

class ZoaEmptyState extends StatelessWidget {
  const ZoaEmptyState({
    super.key,
    required this.title,
    required this.blurb,
    this.stage,
    this.action,
    this.sealSize = 200,
  });

  final String title;
  final String blurb;

  /// Which stage of the loop this emptiness sits at — highlighted on the seal,
  /// so an empty screen still tells the user where they are in the process.
  final LoopStage? stage;

  /// Optional primary action, e.g. "Log your first submission".
  final Widget? action;

  final double sealSize;

  @override
  Widget build(BuildContext context) {
    final actionWidget = action;

    return Center(
      child: SingleChildScrollView(
        padding: const EdgeInsets.symmetric(
          horizontal: ZoaSpace.xl,
          vertical: ZoaSpace.lg,
        ),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: FadeSlideIn.staggered([
            LoopSeal(size: sealSize, highlightStage: stage),
            const SizedBox(height: ZoaSpace.xl),
            Text(title, style: ZoaType.h3, textAlign: TextAlign.center),
            const SizedBox(height: ZoaSpace.sm),
            Text(
              blurb,
              style: ZoaType.bodySm,
              textAlign: TextAlign.center,
            ),
            if (actionWidget != null) ...[
              const SizedBox(height: ZoaSpace.xl),
              actionWidget,
            ],
          ]),
        ),
      ),
    );
  }
}

/// A full-screen error state with a retry affordance.
///
/// Every async action needs a visible failure path (UI/UX doc §4); this is the
/// screen-level one, used when a whole view could not load.
class ZoaErrorState extends StatelessWidget {
  const ZoaErrorState({
    super.key,
    required this.message,
    this.onRetry,
    this.title = 'Something went wrong',
  });

  final String title;
  final String message;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    final retry = onRetry;

    return Center(
      child: ZoaPage(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: FadeSlideIn.staggered([
            ZoaCard(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      const Icon(
                        Icons.error_outline,
                        size: 18,
                        color: ZoaColors.statusError,
                      ),
                      const SizedBox(width: ZoaSpace.sm),
                      Expanded(child: Text(title, style: ZoaType.label)),
                    ],
                  ),
                  const SizedBox(height: ZoaSpace.sm),
                  Text(message, style: ZoaType.bodySm),
                ],
              ),
            ),
            if (retry != null) ...[
              const SizedBox(height: ZoaSpace.lg),
              ZoaPrimaryButton(label: 'Try again', onPressed: retry),
            ],
          ]),
        ),
      ),
    );
  }
}
