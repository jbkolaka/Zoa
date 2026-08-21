/// Material category icons, redrawn from the Materials section of
/// `docs/zoa-website.html`.
///
/// The design brief asks for this exact treatment on material selectors: thin
/// stroke, no fill, rounded joins. Each path below is transcribed from the
/// corresponding SVG in that file and drawn in its original 40×40 coordinate
/// space, so it can be checked against the source line by line.
///
/// Six shapes cover the fourteen taxonomy keys, because the website draws two
/// plastic icons — a bottle for the rigid high-value resins and a tub for film
/// and foam.
library;

import 'package:flutter/material.dart';

import '../theme/zoa_colors.dart';

/// The six icon shapes.
enum MaterialIconShape {
  /// Bottle — PET and HDPE.
  bottle,

  /// Tub with a lid line — LDPE, PP, PS, other plastic.
  tub,

  /// Carton — cardboard and mixed paper.
  carton,

  /// Jar — clear and coloured glass.
  jar,

  /// Can, seen end-on — aluminium and steel.
  can,

  /// Leaf — food and garden waste.
  leaf,
}

/// Maps a taxonomy key to its icon.
///
/// Falls back on the group so an unrecognised key from a future taxonomy change
/// still draws something sensible rather than nothing.
MaterialIconShape materialIconFor(String key, {String? group}) {
  switch (key) {
    case 'pet':
    case 'hdpe':
      return MaterialIconShape.bottle;
    case 'ldpe':
    case 'pp':
    case 'ps':
    case 'other_plastic':
      return MaterialIconShape.tub;
    case 'cardboard':
    case 'mixed_paper':
      return MaterialIconShape.carton;
    case 'glass_clear':
    case 'glass_colored':
      return MaterialIconShape.jar;
    case 'aluminum':
    case 'steel_tin':
      return MaterialIconShape.can;
    case 'food_waste':
    case 'garden_waste':
      return MaterialIconShape.leaf;
  }

  return switch (group) {
    'plastics' => MaterialIconShape.tub,
    'paper' => MaterialIconShape.carton,
    'glass' => MaterialIconShape.jar,
    'metal' => MaterialIconShape.can,
    'organic' => MaterialIconShape.leaf,
    _ => MaterialIconShape.tub,
  };
}

/// Draws a material icon.
class MaterialIcon extends StatelessWidget {
  const MaterialIcon({
    super.key,
    required this.shape,
    this.size = 40,
    this.color,
  });

  /// Convenience: pick the shape from a taxonomy key.
  ///
  /// The parameter is named `taxonomyKey` rather than `key` so it does not
  /// collide with the inherited `Widget.key` forwarded via `super.key`.
  MaterialIcon.forKey(
    String taxonomyKey, {
    super.key,
    String? group,
    this.size = 40,
    this.color,
  }) : shape = materialIconFor(taxonomyKey, group: group);

  final MaterialIconShape shape;
  final double size;

  /// Stroke colour. Defaults to sage, except organics, which the website draws
  /// in gold — organic waste is called out as a first-class stream, not a
  /// footnote (TRD §2.6).
  final Color? color;

  @override
  Widget build(BuildContext context) {
    final stroke = color ??
        (shape == MaterialIconShape.leaf ? ZoaColors.gold : ZoaColors.leaf);

    return SizedBox.square(
      dimension: size,
      child: CustomPaint(
        painter: _MaterialIconPainter(shape: shape, color: stroke),
      ),
    );
  }
}

class _MaterialIconPainter extends CustomPainter {
  _MaterialIconPainter({required this.shape, required this.color});

  final MaterialIconShape shape;
  final Color color;

  /// The source SVGs' coordinate space.
  static const double _box = 40;

  @override
  void paint(Canvas canvas, Size size) {
    canvas.save();
    canvas.scale(size.shortestSide / _box);

    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round;

    for (final path in _paths()) {
      canvas.drawPath(path, paint);
    }

    canvas.restore();
  }

  /// One or more paths per shape, transcribed from the website's SVG `d`
  /// attributes.
  List<Path> _paths() {
    switch (shape) {
      case MaterialIconShape.bottle:
        // M15 8h10v6l4 4v14a2 2 0 0 1-2 2H13a2 2 0 0 1-2-2V18l4-4z
        return [
          Path()
            ..moveTo(15, 8)
            ..lineTo(25, 8)
            ..lineTo(25, 14)
            ..lineTo(29, 18)
            ..lineTo(29, 32)
            ..arcToPoint(const Offset(27, 34), radius: const Radius.circular(2))
            ..lineTo(13, 34)
            ..arcToPoint(const Offset(11, 32), radius: const Radius.circular(2))
            ..lineTo(11, 18)
            ..lineTo(15, 14)
            ..close(),
        ];

      case MaterialIconShape.tub:
        // M10 12h20a2 2 0 0 1 2 2v14H8V14a2 2 0 0 1 2-2z  plus  M8 20h24
        return [
          Path()
            ..moveTo(10, 12)
            ..lineTo(30, 12)
            ..arcToPoint(const Offset(32, 14), radius: const Radius.circular(2))
            ..lineTo(32, 28)
            ..lineTo(8, 28)
            ..lineTo(8, 14)
            ..arcToPoint(const Offset(10, 12), radius: const Radius.circular(2))
            ..close(),
          Path()
            ..moveTo(8, 20)
            ..lineTo(32, 20),
        ];

      case MaterialIconShape.carton:
        // M12 8h16l-2 24H14z  plus  M15 8v6  M25 8v6
        return [
          Path()
            ..moveTo(12, 8)
            ..lineTo(28, 8)
            ..lineTo(26, 32)
            ..lineTo(14, 32)
            ..close(),
          Path()
            ..moveTo(15, 8)
            ..lineTo(15, 14),
          Path()
            ..moveTo(25, 8)
            ..lineTo(25, 14),
        ];

      case MaterialIconShape.jar:
        // M17 6h6l1 8-2 4v14h-4V18l-2-4z
        return [
          Path()
            ..moveTo(17, 6)
            ..lineTo(23, 6)
            ..lineTo(24, 14)
            ..lineTo(22, 18)
            ..lineTo(22, 32)
            ..lineTo(18, 32)
            ..lineTo(18, 18)
            ..lineTo(16, 14)
            ..close(),
        ];

      case MaterialIconShape.can:
        // Two concentric circles, r=13 and r=5.
        return [
          Path()
            ..addOval(Rect.fromCircle(center: const Offset(20, 20), radius: 13)),
          Path()
            ..addOval(Rect.fromCircle(center: const Offset(20, 20), radius: 5)),
        ];

      case MaterialIconShape.leaf:
        // M20 8c6 4 10 10 10 16a10 10 0 0 1-20 0c0-6 4-12 10-16z
        return [
          Path()
            ..moveTo(20, 8)
            ..cubicTo(26, 12, 30, 18, 30, 24)
            ..arcToPoint(const Offset(10, 24), radius: const Radius.circular(10))
            ..cubicTo(10, 18, 14, 12, 20, 8)
            ..close(),
        ];
    }
  }

  @override
  bool shouldRepaint(_MaterialIconPainter old) =>
      old.shape != shape || old.color != color;
}
