/// Colour tokens, lifted verbatim from the `:root` block of
/// `docs/zoa-website.html`. That file is the approved design system, so these
/// values are copied, never re-derived or "improved".
library;

import 'package:flutter/material.dart';

/// The Zoa palette.
///
/// Deep forest green anchors, sage-leaf green supports, savanna gold is
/// reserved for points and rewards, warm paper is the ground.
abstract final class ZoaColors {
  /// `--forest` — mid-deep green, used for large filled surfaces.
  static const forest = Color(0xFF173226);

  /// `--forest-deep` — the darkest green. Headlines, primary buttons.
  static const forestDeep = Color(0xFF0F2019);

  /// `--leaf` — sage green. Secondary strokes, italic accents, dashed borders.
  static const leaf = Color(0xFF4D7862);

  /// `--leaf-soft` — muted sage. Body copy *on* dark green surfaces.
  static const leafSoft = Color(0xFFA9C4B3);

  /// `--gold` — savanna gold. The reward/points accent. Never used for
  /// ordinary UI chrome; if it is gold, it is about points.
  static const gold = Color(0xFFE3B23C);

  /// `--gold-deep` — deeper gold. Kickers, mono accents, gold on light ground
  /// where `gold` alone would not meet contrast.
  static const goldDeep = Color(0xFFC98F1F);

  /// `--paper` — warm paper background.
  static const paper = Color(0xFFF1F3EA);

  /// `--paper-card` — near-white card surface, slightly warmer than white.
  static const paperCard = Color(0xFFFBFBF6);

  /// `--ink` — near-black text.
  static const ink = Color(0xFF12261E);

  /// `--ink-soft` — muted body text.
  static const inkSoft = Color(0xFF4B5F54);

  /// `--line` — hairline borders and dividers.
  static const line = Color(0xFFD8DDD0);

  // --- derived tokens ---
  // The website builds these with rgba() at point of use; naming them here
  // keeps every screen consistent instead of re-guessing opacities.

  /// Pill/chip fill on a light ground — `rgba(77,120,98,0.14)`.
  static const leafWash = Color(0x244D7862);

  /// Reward-flavoured surface fill — `rgba(227,178,60,0.16)`.
  static const goldWash = Color(0x29E3B23C);

  /// Reward-flavoured border — `rgba(227,178,60,0.40)`.
  static const goldBorder = Color(0x66E3B23C);

  /// Card fill on a dark green ground — `rgba(255,255,255,0.05)`.
  static const onForestCard = Color(0x0DFFFFFF);

  /// Border on a dark green ground — `rgba(255,255,255,0.12)`.
  static const onForestLine = Color(0x1FFFFFFF);

  // --- semantic status colours ---
  // Status is never communicated by colour alone (UI/UX doc §5) — these always
  // pair with a label and an icon.

  /// Awaiting action: pending submission, issued (unused) code.
  static const statusPending = goldDeep;

  /// Confirmed: verified submission, accepted code.
  static const statusSuccess = leaf;

  /// Terminal-negative: rejected submission, expired code.
  static const statusError = Color(0xFF9E4A3C);

  /// Spent/consumed: a code that has been used at checkout.
  static const statusUsed = inkSoft;
}

/// Corner radii — `--radius-lg/md/sm` from the design system.
abstract final class ZoaRadius {
  static const double lg = 28;
  static const double md = 18;
  static const double sm = 10;

  /// Fully rounded — the website's `border-radius: 999px` pills.
  static const double pill = 999;

  static const BorderRadius allLg = BorderRadius.all(Radius.circular(lg));
  static const BorderRadius allMd = BorderRadius.all(Radius.circular(md));
  static const BorderRadius allSm = BorderRadius.all(Radius.circular(sm));
  static const BorderRadius allPill = BorderRadius.all(Radius.circular(pill));
}

/// Spacing scale, distilled from the website's padding rhythm
/// (24 / 26 / 28px card padding, 96px section gaps).
abstract final class ZoaSpace {
  static const double xs = 4;
  static const double sm = 8;
  static const double md = 14;
  static const double lg = 20;
  static const double xl = 26;
  static const double xxl = 40;

  /// Horizontal page gutter — the website's `.wrap` padding, tightened for
  /// phone widths.
  static const double gutter = 20;
}
