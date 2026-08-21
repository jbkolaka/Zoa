/// Shared UI primitives, each a direct translation of a component in
/// `docs/zoa-website.html`. Screens compose these rather than restyling
/// Material widgets, so the design system has exactly one implementation.
library;

import 'package:flutter/material.dart';

import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';

/// The eyebrow pill — `.eyebrow`: a small mono label on a sage wash, led by a
/// gold dot. Used above a heading to name the context.
class ZoaEyebrow extends StatelessWidget {
  const ZoaEyebrow(this.label, {super.key, this.onForest = false});

  final String label;

  /// Set when placed on a dark green surface, which inverts the palette
  /// (`.pilot-content .eyebrow` on the website).
  final bool onForest;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 6),
      decoration: BoxDecoration(
        color: onForest ? ZoaColors.goldWash : ZoaColors.leafWash,
        borderRadius: ZoaRadius.allPill,
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 7,
            height: 7,
            decoration: const BoxDecoration(
              color: ZoaColors.goldDeep,
              shape: BoxShape.circle,
            ),
          ),
          const SizedBox(width: 8),
          Text(
            label,
            style: ZoaType.eyebrow.copyWith(
              color: onForest ? ZoaColors.gold : ZoaColors.forestDeep,
            ),
          ),
        ],
      ),
    );
  }
}

/// A section kicker — `.kicker`: uppercase, wide-tracked, gold mono.
class ZoaKicker extends StatelessWidget {
  const ZoaKicker(this.label, {super.key, this.onForest = false});

  final String label;
  final bool onForest;

  @override
  Widget build(BuildContext context) {
    return Text(
      label.toUpperCase(),
      style: ZoaType.kicker.copyWith(
        color: onForest ? ZoaColors.gold : ZoaColors.goldDeep,
      ),
    );
  }
}

/// A section heading block — `.section-head`: kicker, title, optional blurb.
class ZoaSectionHead extends StatelessWidget {
  const ZoaSectionHead({
    super.key,
    this.kicker,
    required this.title,
    this.blurb,
    this.onForest = false,
  });

  final String? kicker;
  final String title;
  final String? blurb;
  final bool onForest;

  @override
  Widget build(BuildContext context) {
    final kickerText = kicker;
    final blurbText = blurb;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (kickerText != null) ...[
          ZoaKicker(kickerText, onForest: onForest),
          const SizedBox(height: ZoaSpace.sm),
        ],
        Text(
          title,
          style: ZoaType.h2.copyWith(
            color: onForest ? ZoaColors.paper : ZoaColors.forestDeep,
          ),
        ),
        if (blurbText != null) ...[
          const SizedBox(height: ZoaSpace.md),
          Text(
            blurbText,
            style: ZoaType.bodySoft.copyWith(
              color: onForest ? ZoaColors.leafSoft : ZoaColors.inkSoft,
            ),
          ),
        ],
      ],
    );
  }
}

/// A card surface — `.mat-card` on light ground, `.flow-card` on forest.
class ZoaCard extends StatelessWidget {
  const ZoaCard({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.all(ZoaSpace.xl),
    this.onTap,
    this.onForest = false,
    this.accent = false,
  });

  final Widget child;
  final EdgeInsetsGeometry padding;
  final VoidCallback? onTap;

  /// Render for placement on a dark green surface.
  final bool onForest;

  /// Reward-flavoured variant — gold wash and gold border, for anything about
  /// points (`.phone-result` on the website).
  final bool accent;

  @override
  Widget build(BuildContext context) {
    final Color background;
    final Color border;

    if (accent) {
      background = ZoaColors.goldWash;
      border = ZoaColors.goldBorder;
    } else if (onForest) {
      background = ZoaColors.onForestCard;
      border = ZoaColors.onForestLine;
    } else {
      background = ZoaColors.paperCard;
      border = ZoaColors.line;
    }

    final decorated = Container(
      padding: padding,
      decoration: BoxDecoration(
        color: background,
        border: Border.all(color: border),
        borderRadius: ZoaRadius.allMd,
      ),
      child: child,
    );

    if (onTap == null) return decorated;

    // Material+InkWell so the tap ripple is clipped to the card's radius.
    return Material(
      color: Colors.transparent,
      borderRadius: ZoaRadius.allMd,
      child: InkWell(
        onTap: onTap,
        borderRadius: ZoaRadius.allMd,
        splashColor: ZoaColors.leafWash,
        highlightColor: ZoaColors.leafWash,
        child: decorated,
      ),
    );
  }
}

/// A small mono tag — `.mat-tag`.
class ZoaTag extends StatelessWidget {
  const ZoaTag(this.label, {super.key, this.color, this.background});

  final String label;
  final Color? color;
  final Color? background;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: background ?? ZoaColors.leafWash,
        borderRadius: ZoaRadius.allPill,
      ),
      child: Text(
        label,
        style: ZoaType.tag.copyWith(color: color ?? ZoaColors.forestDeep),
      ),
    );
  }
}

/// The filled pill button — `.btn-primary`.
class ZoaPrimaryButton extends StatelessWidget {
  const ZoaPrimaryButton({
    super.key,
    required this.label,
    this.onPressed,
    this.loading = false,
    this.icon,
    this.expand = true,
  });

  final String label;
  final VoidCallback? onPressed;

  /// Swaps the label for a spinner and blocks input. Every async action needs
  /// a visible loading state (UI/UX doc §4).
  final bool loading;

  final IconData? icon;

  /// Whether to fill the available width. Primary actions are full-width so
  /// they stay in comfortable thumb reach (UI/UX doc §1.5).
  final bool expand;

  @override
  Widget build(BuildContext context) {
    final enabled = onPressed != null && !loading;

    final button = Material(
      color: enabled ? ZoaColors.forestDeep : ZoaColors.forestDeep.withOpacity(0.45),
      borderRadius: ZoaRadius.allPill,
      child: InkWell(
        onTap: enabled ? onPressed : null,
        borderRadius: ZoaRadius.allPill,
        splashColor: ZoaColors.leaf,
        highlightColor: ZoaColors.leaf,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 26, vertical: 15),
          child: _content(),
        ),
      ),
    );

    return expand ? SizedBox(width: double.infinity, child: button) : button;
  }

  Widget _content() {
    if (loading) {
      return const Center(
        child: SizedBox(
          height: 18,
          width: 18,
          child: CircularProgressIndicator(
            strokeWidth: 2,
            valueColor: AlwaysStoppedAnimation(ZoaColors.paperCard),
          ),
        ),
      );
    }

    final iconData = icon;
    return Row(
      mainAxisSize: expand ? MainAxisSize.max : MainAxisSize.min,
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        Text(label, style: ZoaType.button.copyWith(color: ZoaColors.paperCard)),
        if (iconData != null) ...[
          const SizedBox(width: 8),
          Icon(iconData, size: 18, color: ZoaColors.paperCard),
        ],
      ],
    );
  }
}

/// The outlined pill button — `.btn-ghost`.
class ZoaGhostButton extends StatelessWidget {
  const ZoaGhostButton({
    super.key,
    required this.label,
    this.onPressed,
    this.expand = true,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool expand;

  @override
  Widget build(BuildContext context) {
    final button = Material(
      color: Colors.transparent,
      borderRadius: ZoaRadius.allPill,
      child: InkWell(
        onTap: onPressed,
        borderRadius: ZoaRadius.allPill,
        splashColor: ZoaColors.leafWash,
        highlightColor: ZoaColors.leafWash,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 26, vertical: 14),
          decoration: BoxDecoration(
            border: Border.all(color: ZoaColors.forestDeep, width: 1.5),
            borderRadius: ZoaRadius.allPill,
          ),
          child: Text(
            label,
            textAlign: TextAlign.center,
            style: ZoaType.button.copyWith(color: ZoaColors.forestDeep),
          ),
        ),
      ),
    );

    return expand ? SizedBox(width: double.infinity, child: button) : button;
  }
}

/// A labelled statistic — `.stat`. Mono label under a Fraunces number.
class ZoaStat extends StatelessWidget {
  const ZoaStat({super.key, required this.value, required this.label});

  final String value;
  final String label;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(value, style: ZoaType.statNum),
        const SizedBox(height: ZoaSpace.xs),
        Text(label, style: ZoaType.bodySm),
      ],
    );
  }
}

/// Standard page padding — the website's `.wrap` gutter.
class ZoaPage extends StatelessWidget {
  const ZoaPage({super.key, required this.child, this.scrollable = true});

  final Widget child;
  final bool scrollable;

  @override
  Widget build(BuildContext context) {
    const padding = EdgeInsets.symmetric(
      horizontal: ZoaSpace.gutter,
      vertical: ZoaSpace.lg,
    );

    if (!scrollable) return Padding(padding: padding, child: child);

    return SingleChildScrollView(
      padding: padding,
      // Keeps the pull-to-scroll gesture available even when content is short,
      // which matters on the mostly-empty screens of early phases.
      physics: const AlwaysScrollableScrollPhysics(),
      child: child,
    );
  }
}

/// A form-level error banner, for failures that belong to no single field
/// (wrong credentials, server unreachable). Field-specific messages render
/// inline on the input instead.
class ZoaErrorBanner extends StatelessWidget {
  const ZoaErrorBanner({super.key, required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(ZoaSpace.md),
      decoration: BoxDecoration(
        color: ZoaColors.statusError.withOpacity(0.08),
        border: Border.all(color: ZoaColors.statusError.withOpacity(0.35)),
        borderRadius: ZoaRadius.allSm,
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Icon(Icons.error_outline, size: 18, color: ZoaColors.statusError),
          const SizedBox(width: ZoaSpace.sm),
          Expanded(
            child: Text(
              message,
              style: ZoaType.bodySm.copyWith(color: ZoaColors.ink),
            ),
          ),
        ],
      ),
    );
  }
}
