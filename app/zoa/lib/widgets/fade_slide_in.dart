/// Entrance motion for the app, mirroring the website's `.reveal` class:
/// fade from 0 to 1 while rising 18px, over 0.7s ease.
///
/// The brief allows subtle, purposeful motion only — this exists so content
/// arrives calmly instead of snapping in, and nothing more.
library;

import 'package:flutter/material.dart';

import '../theme/zoa_theme.dart';

/// Fades and slides [child] in once, when it first mounts.
///
/// [delay] staggers siblings so a list arrives in sequence rather than all at
/// once. Keep stagger totals under ~300ms; past that it reads as sluggish.
class FadeSlideIn extends StatefulWidget {
  const FadeSlideIn({
    super.key,
    required this.child,
    this.delay = Duration.zero,
    this.offset = ZoaMotion.revealOffset,
  });

  final Widget child;

  /// How long to wait before starting. Used to stagger a list.
  final Duration delay;

  /// Vertical travel in logical pixels. Defaults to the website's 18px.
  final double offset;

  /// Builds a staggered column of children, each delayed by [step] more than
  /// the last — the common case for a screen's content arriving on load.
  static List<Widget> staggered(
    List<Widget> children, {
    Duration step = const Duration(milliseconds: 70),
    double offset = ZoaMotion.revealOffset,
  }) {
    return List<Widget>.generate(
      children.length,
      (i) => FadeSlideIn(
        delay: step * i,
        offset: offset,
        child: children[i],
      ),
    );
  }

  @override
  State<FadeSlideIn> createState() => _FadeSlideInState();
}

class _FadeSlideInState extends State<FadeSlideIn>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: ZoaMotion.reveal,
  );

  late final Animation<double> _eased = CurvedAnimation(
    parent: _controller,
    curve: ZoaMotion.curve,
  );

  @override
  void initState() {
    super.initState();
    _start();
  }

  Future<void> _start() async {
    if (widget.delay > Duration.zero) {
      await Future<void>.delayed(widget.delay);
      // The widget can be disposed while the delay is pending — e.g. the user
      // navigates away mid-stagger. Animating a disposed controller throws.
      if (!mounted) return;
    }
    _controller.forward();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // With reduced motion requested, render the final state immediately.
    if (MediaQuery.of(context).disableAnimations) return widget.child;

    return AnimatedBuilder(
      animation: _eased,
      builder: (context, child) => Opacity(
        opacity: _eased.value,
        child: Transform.translate(
          offset: Offset(0, widget.offset * (1 - _eased.value)),
          child: child,
        ),
      ),
      child: widget.child,
    );
  }
}
