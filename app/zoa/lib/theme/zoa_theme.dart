/// Typography, theme and motion tokens derived from `docs/zoa-website.html`.
///
/// Type roles, straight from the design system:
///   * **Fraunces** (serif) — headlines and display moments only.
///   * **IBM Plex Sans** — body text and UI copy.
///   * **IBM Plex Mono** — stats, codes, tags, anything data-like. Redemption
///     codes are always mono.
///
/// `rem` values from the stylesheet are converted at 1rem = 16 logical pixels,
/// and `em`-based letter-spacing is converted to logical pixels at each style's
/// own size (Flutter's `letterSpacing` is absolute, CSS's `em` is relative).
library;

import 'package:flutter/material.dart';

import 'zoa_colors.dart';

/// Font family names. These must match the `family:` keys in `pubspec.yaml`.
abstract final class ZoaFonts {
  static const display = 'Fraunces';
  static const sans = 'IBM Plex Sans';
  static const mono = 'IBM Plex Mono';
}

/// The Zoa type scale.
abstract final class ZoaType {
  // ---------- display (Fraunces) ----------

  /// Hero headline. The website's `.hero h1` at phone width (2.5rem/1.03).
  static const hero = TextStyle(
    fontFamily: ZoaFonts.display,
    fontSize: 40,
    height: 1.03,
    fontWeight: FontWeight.w600,
    letterSpacing: -0.4, // -0.01em
    color: ZoaColors.forestDeep,
  );

  /// Italic emphasis inside a hero headline — `.hero h1 em`, sage green.
  static const heroEm = TextStyle(
    fontFamily: ZoaFonts.display,
    fontSize: 40,
    height: 1.03,
    fontWeight: FontWeight.w500,
    fontStyle: FontStyle.italic,
    letterSpacing: -0.4,
    color: ZoaColors.leaf,
  );

  /// Section heading — `.section-head h2`.
  static const h2 = TextStyle(
    fontFamily: ZoaFonts.display,
    fontSize: 28,
    height: 1.1,
    fontWeight: FontWeight.w600,
    letterSpacing: -0.28,
    color: ZoaColors.forestDeep,
  );

  /// Sub-heading — `.voucher h3` (1.2rem).
  static const h3 = TextStyle(
    fontFamily: ZoaFonts.display,
    fontSize: 19,
    height: 1.2,
    fontWeight: FontWeight.w600,
    color: ZoaColors.forestDeep,
  );

  /// Card title — `.mat-card h3` (1.08rem).
  static const cardTitle = TextStyle(
    fontFamily: ZoaFonts.display,
    fontSize: 17,
    height: 1.25,
    fontWeight: FontWeight.w600,
    color: ZoaColors.forestDeep,
  );

  /// A big number that is *not* points — `.stat .num` (2.1rem).
  static const statNum = TextStyle(
    fontFamily: ZoaFonts.display,
    fontSize: 34,
    height: 1.1,
    fontWeight: FontWeight.w600,
    color: ZoaColors.forestDeep,
  );

  /// The points balance headline. Its own role because UI/UX §1.2 requires the
  /// balance to read as the most important number on screen.
  static const pointsHero = TextStyle(
    fontFamily: ZoaFonts.display,
    fontSize: 46,
    height: 1.0,
    fontWeight: FontWeight.w600,
    letterSpacing: -0.5,
    color: ZoaColors.forestDeep,
  );

  // ---------- body (IBM Plex Sans) ----------

  /// Intro paragraph — `.hero p.lead` (1.12rem).
  static const lead = TextStyle(
    fontFamily: ZoaFonts.sans,
    fontSize: 18,
    height: 1.5,
    fontWeight: FontWeight.w400,
    color: ZoaColors.inkSoft,
  );

  /// Default body copy.
  static const body = TextStyle(
    fontFamily: ZoaFonts.sans,
    fontSize: 16,
    height: 1.5,
    fontWeight: FontWeight.w400,
    color: ZoaColors.ink,
  );

  /// Body copy, de-emphasised — the website's `color: var(--ink-soft)` default
  /// for paragraphs.
  static const bodySoft = TextStyle(
    fontFamily: ZoaFonts.sans,
    fontSize: 16,
    height: 1.5,
    fontWeight: FontWeight.w400,
    color: ZoaColors.inkSoft,
  );

  /// Small supporting copy — `.mat-card p` (0.9rem).
  static const bodySm = TextStyle(
    fontFamily: ZoaFonts.sans,
    fontSize: 14.5,
    height: 1.45,
    fontWeight: FontWeight.w400,
    color: ZoaColors.inkSoft,
  );

  /// Form labels and inline emphasis — `.feature-item h4`.
  static const label = TextStyle(
    fontFamily: ZoaFonts.sans,
    fontSize: 15,
    height: 1.3,
    fontWeight: FontWeight.w600,
    color: ZoaColors.forestDeep,
  );

  /// Button text — `.btn` (0.95rem/600).
  static const button = TextStyle(
    fontFamily: ZoaFonts.sans,
    fontSize: 15,
    height: 1.2,
    fontWeight: FontWeight.w600,
  );

  // ---------- data (IBM Plex Mono) ----------

  /// Section kicker — `.section-head .kicker`: uppercase, gold, wide-tracked.
  static const kicker = TextStyle(
    fontFamily: ZoaFonts.mono,
    fontSize: 12.5,
    height: 1.2,
    fontWeight: FontWeight.w600,
    letterSpacing: 0.5, // 0.04em
    color: ZoaColors.goldDeep,
  );

  /// Eyebrow pill text — `.eyebrow`.
  static const eyebrow = TextStyle(
    fontFamily: ZoaFonts.mono,
    fontSize: 12.5,
    height: 1.2,
    fontWeight: FontWeight.w500,
    color: ZoaColors.forestDeep,
  );

  /// Generic mono for data values — `.mono` (0.02em tracking).
  static const mono = TextStyle(
    fontFamily: ZoaFonts.mono,
    fontSize: 14,
    height: 1.4,
    fontWeight: FontWeight.w400,
    letterSpacing: 0.28,
    color: ZoaColors.inkSoft,
  );

  /// Small mono tag — `.mat-tag` (0.72rem).
  static const tag = TextStyle(
    fontFamily: ZoaFonts.mono,
    fontSize: 11.5,
    height: 1.2,
    fontWeight: FontWeight.w500,
    letterSpacing: 0.23,
    color: ZoaColors.forestDeep,
  );

  /// A points cost — `.voucher .cost`. Gold, because gold means points.
  static const pointsCost = TextStyle(
    fontFamily: ZoaFonts.mono,
    fontSize: 13,
    height: 1.2,
    fontWeight: FontWeight.w600,
    letterSpacing: 0.26,
    color: ZoaColors.goldDeep,
  );

  /// A redemption code. Large, wide-tracked mono so it can be read aloud to a
  /// cashier or transcribed without ambiguity (UI/UX §1.3).
  static const code = TextStyle(
    fontFamily: ZoaFonts.mono,
    fontSize: 22,
    height: 1.35,
    fontWeight: FontWeight.w600,
    letterSpacing: 1.6,
    color: ZoaColors.forestDeep,
  );
}

/// Motion tokens. The design brief allows subtle, purposeful motion only:
/// fade/slide on load and gentle state changes. Nothing decorative.
abstract final class ZoaMotion {
  /// `.reveal` — 0.7s ease fade + 18px rise.
  static const Duration reveal = Duration(milliseconds: 700);

  /// The reveal's vertical travel, in logical pixels.
  static const double revealOffset = 18;

  /// Hover/press/state transitions — the stylesheet's 0.25s ease.
  static const Duration quick = Duration(milliseconds: 250);

  /// One full rotation of the seal's outer ring — `spin 40s linear`.
  static const Duration sealSpin = Duration(seconds: 40);

  static const Curve curve = Curves.easeOut;
}

/// The app's [ColorScheme].
///
/// Written out explicitly rather than generated by `ColorScheme.fromSeed`,
/// which would produce its own tonal palette and quietly replace the approved
/// hex values with near-misses.
const ColorScheme _zoaScheme = ColorScheme.light(
  primary: ZoaColors.forestDeep,
  onPrimary: ZoaColors.paperCard,
  primaryContainer: ZoaColors.forest,
  onPrimaryContainer: ZoaColors.paper,
  secondary: ZoaColors.leaf,
  onSecondary: ZoaColors.paperCard,
  secondaryContainer: ZoaColors.leafWash,
  onSecondaryContainer: ZoaColors.forestDeep,
  // Gold is the reward accent, so it maps to tertiary — never to primary.
  tertiary: ZoaColors.goldDeep,
  onTertiary: ZoaColors.paperCard,
  tertiaryContainer: ZoaColors.goldWash,
  onTertiaryContainer: ZoaColors.forestDeep,
  error: ZoaColors.statusError,
  onError: ZoaColors.paperCard,
  surface: ZoaColors.paper,
  onSurface: ZoaColors.ink,
  outline: ZoaColors.line,
  outlineVariant: ZoaColors.line,
);

/// Builds the app theme.
///
/// Deliberately minimal: `colorScheme`, `textTheme` and background only.
/// Component-level chrome lives in the `Zoa*` widgets instead of Flutter's
/// per-component theme classes — those have been renamed across recent Flutter
/// releases (`CardTheme` → `CardThemeData`, and similar), and pinning styling
/// to our own widgets keeps the app buildable across SDK versions while
/// matching the design system exactly.
ThemeData buildZoaTheme() {
  return ThemeData(
    useMaterial3: true,
    colorScheme: _zoaScheme,
    scaffoldBackgroundColor: ZoaColors.paper,
    fontFamily: ZoaFonts.sans,
    textTheme: const TextTheme(
      displayLarge: ZoaType.hero,
      displayMedium: ZoaType.pointsHero,
      displaySmall: ZoaType.statNum,
      headlineMedium: ZoaType.h2,
      headlineSmall: ZoaType.h3,
      titleLarge: ZoaType.cardTitle,
      titleMedium: ZoaType.label,
      titleSmall: ZoaType.tag,
      bodyLarge: ZoaType.body,
      bodyMedium: ZoaType.bodySoft,
      bodySmall: ZoaType.bodySm,
      labelLarge: ZoaType.button,
      labelMedium: ZoaType.mono,
      labelSmall: ZoaType.kicker,
    ),
  );
}
