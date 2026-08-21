/// The Zoa brand mark — a re-draw of the website's `.brand-mark` SVG.
///
/// A ringed S-curve: gold dot at the intake, forest dot at the return. Used in
/// the app bar, where the full loop seal would be far too heavy.
library;

import 'package:flutter/material.dart';

import '../theme/zoa_colors.dart';

class ZoaBrandMark extends StatelessWidget {
  const ZoaBrandMark({super.key, this.size = 28});

  final double size;

  @override
  Widget build(BuildContext context) {
    return SizedBox.square(
      dimension: size,
      child: CustomPaint(painter: _BrandMarkPainter()),
    );
  }
}

class _BrandMarkPainter extends CustomPainter {
  /// The source SVG's coordinate space.
  static const double _box = 40;

  @override
  void paint(Canvas canvas, Size size) {
    canvas.save();
    canvas.scale(size.shortestSide / _box);

    // Outer ring.
    canvas.drawCircle(
      const Offset(20, 20),
      19,
      Paint()
        ..color = ZoaColors.forest
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1.5,
    );

    // The S-curve threading through the ring.
    final curve = Path()
      ..moveTo(20, 8)
      ..cubicTo(25, 12, 25, 20, 20, 20)
      ..cubicTo(15, 20, 15, 28, 20, 32);

    canvas.drawPath(
      curve,
      Paint()
        ..color = ZoaColors.leaf
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2.2
        ..strokeCap = StrokeCap.round,
    );

    // Intake (gold) and return (forest) terminals.
    canvas.drawCircle(const Offset(20, 8), 2.6, Paint()..color = ZoaColors.gold);
    canvas.drawCircle(const Offset(20, 32), 2.6, Paint()..color = ZoaColors.forest);

    canvas.restore();
  }

  @override
  bool shouldRepaint(_BrandMarkPainter oldDelegate) => false;
}
