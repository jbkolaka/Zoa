/// The loop seal — Zoa's visual identity.
///
/// A faithful re-draw of the hero seal in `docs/zoa-website.html`: two outer
/// rings (one dashed, slowly rotating), a paper disc, and four arrows tracing
/// the closed loop **recycle → verify → earn → redeem**, in forest, sage, gold
/// and deep gold.
///
/// Per the design brief this motif is the platform's identity, not just
/// marketing furniture, so it is built to carry real work in the app:
///   * [LoopSeal.loading] — the splash and any long await.
///   * [LoopSeal] with `highlightStep` — shows *where* in the loop a submission
///     currently sits (used by the Submission Status screen).
///   * [LoopSeal] with a custom centre — empty and success states.
library;

import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';

/// The four stages of the loop, in order, matching the seal's arc labels.
enum LoopStage {
  recycle('RECYCLE'),
  verify('VERIFY'),
  earn('EARN'),
  redeem('REDEEM');

  const LoopStage(this.label);

  /// The uppercase mono label drawn on the seal.
  final String label;
}

/// Renders the loop seal at [size] logical pixels square.
class LoopSeal extends StatefulWidget {
  const LoopSeal({
    super.key,
    this.size = 220,
    this.spinning = false,
    this.centerLabel = 'zoa',
    this.subLabel,
    this.highlightStage,
    this.showStageLabels = true,
  });

  /// The seal as a loading indicator: outer ring rotating, optional caption.
  const LoopSeal.loading({
    super.key,
    this.size = 220,
    this.centerLabel = 'zoa',
    this.subLabel,
    this.showStageLabels = true,
  })  : spinning = true,
        highlightStage = null;

  /// Edge length of the (square) seal.
  final double size;

  /// Whether the outer rings rotate. Ignored when the platform requests
  /// reduced motion.
  final bool spinning;

  /// Wordmark drawn at the centre. Pass an empty string to omit.
  final String centerLabel;

  /// Small mono line beneath the wordmark.
  final String? subLabel;

  /// When set, that arc is drawn at full strength and the other three are
  /// dimmed — "you are here" within the loop.
  final LoopStage? highlightStage;

  /// Whether the four stage labels are drawn. Turn off at small sizes, where
  /// the labels stop being legible.
  final bool showStageLabels;

  @override
  State<LoopSeal> createState() => _LoopSealState();
}

class _LoopSealState extends State<LoopSeal> with SingleTickerProviderStateMixin {
  late final AnimationController _ring = AnimationController(
    vsync: this,
    duration: ZoaMotion.sealSpin,
  );

  @override
  void initState() {
    super.initState();
    if (widget.spinning) _ring.repeat();
  }

  @override
  void didUpdateWidget(LoopSeal old) {
    super.didUpdateWidget(old);
    if (widget.spinning == old.spinning) return;
    if (widget.spinning) {
      _ring.repeat();
    } else {
      _ring.stop();
    }
  }

  @override
  void dispose() {
    _ring.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // Honour the platform's reduced-motion setting, mirroring the website's
    // `@media (prefers-reduced-motion: reduce)` block.
    final reduceMotion = MediaQuery.of(context).disableAnimations;

    return Semantics(
      label: 'Zoa loop: recycle, verify, earn, redeem',
      child: SizedBox.square(
        dimension: widget.size,
        child: RepaintBoundary(
          child: AnimatedBuilder(
            animation: _ring,
            builder: (context, _) => CustomPaint(
              painter: _SealPainter(
                rotation: reduceMotion ? 0 : _ring.value * 2 * math.pi,
                centerLabel: widget.centerLabel,
                subLabel: widget.subLabel,
                highlightStage: widget.highlightStage,
                showStageLabels: widget.showStageLabels,
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Paints the seal in the design system's own 320×320 coordinate space, then
/// scales to fit. Working in the source coordinates keeps every radius and
/// offset checkable against the SVG in `docs/zoa-website.html`.
class _SealPainter extends CustomPainter {
  _SealPainter({
    required this.rotation,
    required this.centerLabel,
    required this.subLabel,
    required this.highlightStage,
    required this.showStageLabels,
  });

  final double rotation;
  final String centerLabel;
  final String? subLabel;
  final LoopStage? highlightStage;
  final bool showStageLabels;

  /// The SVG's coordinate space.
  static const double _box = 320;
  static const Offset _c = Offset(160, 160);

  /// Arc colours, in loop order — forest, sage, gold, deep gold.
  static const List<Color> _stageColors = [
    ZoaColors.forest,
    ZoaColors.leaf,
    ZoaColors.gold,
    ZoaColors.goldDeep,
  ];

  @override
  void paint(Canvas canvas, Size size) {
    final scale = size.shortestSide / _box;
    canvas.save();
    canvas.scale(scale);

    _drawRotatingRings(canvas);
    _drawDisc(canvas);
    _drawLoopArcs(canvas);
    if (showStageLabels) _drawStageLabels(canvas);
    _drawCenter(canvas);

    canvas.restore();
  }

  /// The two outer rings, which rotate as one group (`.seal-ring`).
  void _drawRotatingRings(Canvas canvas) {
    canvas.save();
    canvas.translate(_c.dx, _c.dy);
    canvas.rotate(rotation);
    canvas.translate(-_c.dx, -_c.dy);

    // r=150, dashed 2/6, sage.
    _drawDashedCircle(
      canvas,
      radius: 150,
      dash: 2,
      gap: 6,
      paint: Paint()
        ..color = ZoaColors.leaf
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1,
    );

    // r=132, solid, forest.
    canvas.drawCircle(
      _c,
      132,
      Paint()
        ..color = ZoaColors.forest
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1.2,
    );

    canvas.restore();
  }

  /// Flutter has no dashed-stroke primitive, so the dash pattern is drawn as a
  /// run of short arcs. Angles come from arc length ÷ radius.
  void _drawDashedCircle(
    Canvas canvas, {
    required double radius,
    required double dash,
    required double gap,
    required Paint paint,
  }) {
    final rect = Rect.fromCircle(center: _c, radius: radius);
    final dashAngle = dash / radius;
    final period = (dash + gap) / radius;
    final count = (2 * math.pi / period).floor();

    for (var i = 0; i < count; i++) {
      canvas.drawArc(rect, i * period, dashAngle, false, paint);
    }
  }

  /// The paper disc the loop sits on — r=108, paper-card fill, hairline edge.
  void _drawDisc(Canvas canvas) {
    canvas.drawCircle(_c, 108, Paint()..color = ZoaColors.paperCard);
    canvas.drawCircle(
      _c,
      108,
      Paint()
        ..color = ZoaColors.line
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1,
    );
  }

  /// Four 80° arcs at r=90 with tangential arrowheads, leaving a gap at each
  /// quadrant boundary — the closed loop.
  void _drawLoopArcs(Canvas canvas) {
    const radius = 90.0;
    const sweep = 80 * math.pi / 180;
    final rect = Rect.fromCircle(center: _c, radius: radius);

    for (var i = 0; i < 4; i++) {
      final stage = LoopStage.values[i];
      final dimmed = highlightStage != null && highlightStage != stage;
      final color = dimmed
          ? _stageColors[i].withOpacity(0.22)
          : _stageColors[i];

      // Start at the top (-90°) and step a quadrant at a time, inset 5° so the
      // arcs read as four separate strokes rather than one ring.
      final start = (-90 + 5 + i * 90) * math.pi / 180;

      canvas.drawArc(
        rect,
        start,
        sweep,
        false,
        Paint()
          ..color = color
          ..style = PaintingStyle.stroke
          ..strokeWidth = 3
          ..strokeCap = StrokeCap.round,
      );

      _drawArrowHead(canvas, angle: start + sweep, radius: radius, color: color);
    }
  }

  /// A small triangle at the arc's end, pointing along the direction of travel.
  void _drawArrowHead(
    Canvas canvas, {
    required double angle,
    required double radius,
    required Color color,
  }) {
    // Radial (outward) and tangential (clockwise) unit vectors at `angle`.
    final radial = Offset(math.cos(angle), math.sin(angle));
    final tangent = Offset(-math.sin(angle), math.cos(angle));
    final tip = _c + radial * radius + tangent * 9;
    final base = _c + radial * radius;

    final path = Path()
      ..moveTo(tip.dx, tip.dy)
      ..lineTo(base.dx + radial.dx * 5, base.dy + radial.dy * 5)
      ..lineTo(base.dx - radial.dx * 5, base.dy - radial.dy * 5)
      ..close();

    canvas.drawPath(path, Paint()..color = color);
  }

  /// RECYCLE / VERIFY / EARN / REDEEM at the four cardinal points.
  void _drawStageLabels(Canvas canvas) {
    const positions = <Offset>[
      Offset(0, -54), // recycle — top
      Offset(66, 0), // verify — right
      Offset(0, 54), // earn — bottom
      Offset(-66, 0), // redeem — left
    ];

    for (var i = 0; i < 4; i++) {
      final stage = LoopStage.values[i];
      final dimmed = highlightStage != null && highlightStage != stage;

      _paintText(
        canvas,
        stage.label,
        _c + positions[i],
        TextStyle(
          fontFamily: ZoaFonts.mono,
          fontSize: 10,
          fontWeight: dimmed ? FontWeight.w400 : FontWeight.w600,
          letterSpacing: 0.4,
          color: dimmed
              ? ZoaColors.inkSoft.withOpacity(0.4)
              : ZoaColors.inkSoft,
        ),
      );
    }
  }

  /// The centre wordmark and its small mono sub-line.
  void _drawCenter(Canvas canvas) {
    if (centerLabel.isNotEmpty) {
      _paintText(
        canvas,
        centerLabel,
        _c,
        const TextStyle(
          fontFamily: ZoaFonts.display,
          fontSize: 20,
          fontWeight: FontWeight.w600,
          color: ZoaColors.forest,
        ),
      );
    }

    final sub = subLabel;
    if (sub != null && sub.isNotEmpty) {
      _paintText(
        canvas,
        sub,
        _c + const Offset(0, 22),
        const TextStyle(
          fontFamily: ZoaFonts.mono,
          fontSize: 9,
          fontWeight: FontWeight.w400,
          letterSpacing: 0.2,
          color: ZoaColors.inkSoft,
        ),
      );
    }
  }

  /// Draws [text] centred on [center].
  void _paintText(Canvas canvas, String text, Offset center, TextStyle style) {
    final painter = TextPainter(
      text: TextSpan(text: text, style: style),
      textDirection: TextDirection.ltr,
      textAlign: TextAlign.center,
    )..layout(maxWidth: 200);

    painter.paint(
      canvas,
      Offset(center.dx - painter.width / 2, center.dy - painter.height / 2),
    );
  }

  @override
  bool shouldRepaint(_SealPainter old) =>
      old.rotation != rotation ||
      old.centerLabel != centerLabel ||
      old.subLabel != subLabel ||
      old.highlightStage != highlightStage ||
      old.showStageLabels != showStageLabels;
}
