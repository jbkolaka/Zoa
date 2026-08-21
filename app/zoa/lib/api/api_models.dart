/// Typed models for the API responses used so far.
///
/// Every model ignores unknown keys, per the contract's rule that the server
/// may add fields without breaking a shipped build.
library;

/// Response from `GET /health`.
class HealthStatus {
  const HealthStatus({
    required this.status,
    required this.service,
    required this.version,
    required this.env,
    required this.uptimeSeconds,
    required this.databaseConnected,
    required this.migrationsApplied,
    required this.schemaVersion,
  });

  final String status;
  final String service;
  final String version;
  final String env;
  final int uptimeSeconds;
  final bool databaseConnected;
  final int migrationsApplied;
  final String schemaVersion;

  bool get isHealthy => status == 'ok' && databaseConnected;

  factory HealthStatus.fromJson(Map<String, dynamic> json) {
    final database = (json['database'] as Map?)?.cast<String, dynamic>() ?? const {};

    return HealthStatus(
      status: json['status'] as String? ?? 'unknown',
      service: json['service'] as String? ?? 'zoa-api',
      version: json['version'] as String? ?? '?',
      env: json['env'] as String? ?? '?',
      uptimeSeconds: (json['uptime_seconds'] as num?)?.toInt() ?? 0,
      databaseConnected: database['connected'] as bool? ?? false,
      migrationsApplied: (database['migrations_applied'] as num?)?.toInt() ?? 0,
      schemaVersion: database['schema_version'] as String? ?? '',
    );
  }
}

/// One entry of the material taxonomy from `GET /meta`.
///
/// The list is served rather than hardcoded because points rates are
/// admin-configurable (FR-9) — a baked-in client copy would silently drift out
/// of step with `material_rates`.
class MaterialInfo {
  const MaterialInfo({
    required this.key,
    required this.group,
    required this.label,
    required this.pointsPerKg,
  });

  /// Taxonomy key — the value stored in `submissions.material_type`.
  final String key;

  /// `plastics` | `paper` | `glass` | `metal` | `organic`.
  final String group;

  /// Display label, e.g. "PET bottles".
  final String label;

  final int pointsPerKg;

  /// Organic materials need a source type (hotel vs residential) per FR-24.
  bool get requiresSourceType => group == 'organic';

  factory MaterialInfo.fromJson(Map<String, dynamic> json) => MaterialInfo(
        key: json['key'] as String,
        group: json['group'] as String? ?? 'other',
        label: json['label'] as String? ?? json['key'] as String,
        pointsPerKg: (json['points_per_kg'] as num?)?.toInt() ?? 0,
      );
}

/// Response from `GET /meta` — server-driven reference data.
class MetaCatalog {
  const MetaCatalog({required this.materials});

  final List<MaterialInfo> materials;

  /// Taxonomy grouped by material group, preserving the server's ordering so
  /// the selector always lists plastics first, organics last.
  Map<String, List<MaterialInfo>> get byGroup {
    final grouped = <String, List<MaterialInfo>>{};
    for (final material in materials) {
      grouped.putIfAbsent(material.group, () => []).add(material);
    }
    return grouped;
  }

  /// The display label for a taxonomy key, falling back to the key itself so an
  /// unknown material still renders something readable rather than blank.
  String labelFor(String key) {
    for (final material in materials) {
      if (material.key == key) return material.label;
    }
    return key;
  }

  /// The configured rate for a key, or null if unknown.
  int? rateFor(String key) {
    for (final material in materials) {
      if (material.key == key) return material.pointsPerKg;
    }
    return null;
  }

  /// The full entry for a key, or null if unknown.
  MaterialInfo? materialFor(String key) {
    for (final material in materials) {
      if (material.key == key) return material;
    }
    return null;
  }

  factory MetaCatalog.fromJson(Map<String, dynamic> json) {
    final raw = json['materials'] as List? ?? const [];
    return MetaCatalog(
      materials: raw
          .whereType<Map>()
          .map((m) => MaterialInfo.fromJson(m.cast<String, dynamic>()))
          .toList(growable: false),
    );
  }
}

/// A user account, as returned by `/auth/register`, `/auth/login` and `/me`.
///
/// `password_hash` is never present in any response, so there is no field for it
/// here — the model cannot accidentally carry or log one.
class ZoaUser {
  const ZoaUser({
    required this.id,
    required this.phoneNumber,
    required this.name,
    required this.role,
    required this.pointsBalance,
    required this.createdAt,
  });

  final int id;

  /// Always in canonical `+254…` form; the server normalises on the way in.
  final String phoneNumber;

  final String name;

  /// `user` | `collector` | `partner_staff` | `admin`.
  final String role;

  final int pointsBalance;
  final DateTime createdAt;

  bool get isCollector => role == 'collector' || role == 'admin';
  bool get isPartnerStaff => role == 'partner_staff' || role == 'admin';
  bool get isAdmin => role == 'admin';

  /// True only for a dedicated collector account — admin does *not* inherit it.
  ///
  /// The getters above are deliberately inclusive of admin so one admin login
  /// can drive every flow in a demo. This asks the opposite question — "is this
  /// account *only* a collector?" — and it gates what a collector must *not*
  /// do. Inheriting here would take the capability away from admin rather than
  /// grant one, which is the reverse of what the other getters are for.
  bool get isCollectorOnly => role == 'collector';

  /// Human-readable role, for the profile screen.
  String get roleLabel => switch (role) {
        'collector' => 'Collector',
        'partner_staff' => 'Partner staff',
        'admin' => 'Administrator',
        _ => 'Recycler',
      };

  /// Initials for the profile avatar, at most two letters.
  String get initials {
    final parts = name.trim().split(RegExp(r'\s+')).where((p) => p.isNotEmpty);
    if (parts.isEmpty) return '?';
    final letters = parts.take(2).map((p) => p[0].toUpperCase());
    return letters.join();
  }

  factory ZoaUser.fromJson(Map<String, dynamic> json) => ZoaUser(
        id: (json['id'] as num).toInt(),
        phoneNumber: json['phone_number'] as String? ?? '',
        name: json['name'] as String? ?? '',
        role: json['role'] as String? ?? 'user',
        pointsBalance: (json['points_balance'] as num?)?.toInt() ?? 0,
        createdAt:
            DateTime.tryParse(json['created_at'] as String? ?? '')?.toLocal() ??
                DateTime.fromMillisecondsSinceEpoch(0),
      );
}

/// Response from `/auth/register` and `/auth/login`.
class AuthSession {
  const AuthSession({
    required this.token,
    required this.user,
    this.expiresAt,
  });

  final String token;
  final ZoaUser user;
  final DateTime? expiresAt;

  factory AuthSession.fromJson(Map<String, dynamic> json) {
    final user = json['user'];
    if (user is! Map) {
      throw const FormatException('auth response is missing the user object');
    }

    return AuthSession(
      token: json['token'] as String? ?? '',
      user: ZoaUser.fromJson(user.cast<String, dynamic>()),
      expiresAt:
          DateTime.tryParse(json['expires_at'] as String? ?? '')?.toLocal(),
    );
  }
}

/// Submission status values. Mirrors the lifecycle in FR-5; transitions are
/// server-side only, so the client reads these but never sends them.
abstract final class SubmissionStatus {
  static const pending = 'pending';
  static const collected = 'collected';
  static const verified = 'verified';
  static const rejected = 'rejected';
}

/// A recycling submission.
class Submission {
  const Submission({
    required this.id,
    required this.userId,
    required this.userName,
    required this.materialType,
    required this.status,
    required this.createdAt,
    this.estimatedQtyKg,
    this.verifiedQtyKg,
    this.location,
    this.collectorId,
    this.verifiedAt,
    this.pointsAwarded,
  });

  final int id;
  final int userId;

  /// Submitter's name — the collector queue needs a person to ask for.
  final String userName;

  /// A taxonomy key such as `pet` or `food_waste`.
  final String materialType;

  final String status;
  final DateTime createdAt;

  /// User's own estimate.
  final double? estimatedQtyKg;

  /// Collector's measurement — null until verified.
  final double? verifiedQtyKg;

  final String? location;
  final int? collectorId;
  final DateTime? verifiedAt;

  /// Ledger total for this submission — null until verified.
  final int? pointsAwarded;

  bool get isPending => status == SubmissionStatus.pending;
  bool get isCollected => status == SubmissionStatus.collected;
  bool get isVerified => status == SubmissionStatus.verified;
  bool get isRejected => status == SubmissionStatus.rejected;

  /// Whether a collector can still act on this submission.
  bool get isOpen => isPending || isCollected;

  /// The weight to display: the verified figure once there is one, otherwise the
  /// estimate.
  double? get displayQtyKg => verifiedQtyKg ?? estimatedQtyKg;

  factory Submission.fromJson(Map<String, dynamic> json) {
    final user = json['user'];
    final userMap = user is Map ? user.cast<String, dynamic>() : const {};

    return Submission(
      id: (json['id'] as num).toInt(),
      userId: (json['user_id'] as num?)?.toInt() ?? 0,
      userName: userMap['name'] as String? ?? '',
      materialType: json['material_type'] as String? ?? '',
      status: json['status'] as String? ?? SubmissionStatus.pending,
      createdAt:
          DateTime.tryParse(json['created_at'] as String? ?? '')?.toLocal() ??
              DateTime.fromMillisecondsSinceEpoch(0),
      estimatedQtyKg: (json['estimated_qty_kg'] as num?)?.toDouble(),
      verifiedQtyKg: (json['verified_qty_kg'] as num?)?.toDouble(),
      location: json['location'] as String?,
      collectorId: (json['collector_id'] as num?)?.toInt(),
      verifiedAt:
          DateTime.tryParse(json['verified_at'] as String? ?? '')?.toLocal(),
      pointsAwarded: (json['points_awarded'] as num?)?.toInt(),
    );
  }
}

/// A page of submissions, with the total matching the filter.
class SubmissionPage {
  const SubmissionPage({required this.submissions, required this.total});

  final List<Submission> submissions;

  /// Total matching the filter, ignoring limit/offset — so a queue can show a
  /// real count rather than "50+".
  final int total;

  factory SubmissionPage.fromJson(Map<String, dynamic> json) {
    final raw = json['submissions'] as List? ?? const [];
    return SubmissionPage(
      submissions: raw
          .whereType<Map>()
          .map((s) => Submission.fromJson(s.cast<String, dynamic>()))
          .toList(growable: false),
      total: (json['total'] as num?)?.toInt() ?? 0,
    );
  }
}

/// Result of a collector's verification.
class VerifyResult {
  const VerifyResult({
    required this.submission,
    required this.pointsAwarded,
    required this.pointsBalance,
    required this.rateApplied,
    required this.rateMaterialType,
  });

  final Submission submission;

  /// Points credited to the submitter. Zero for `collected` and `rejected`.
  final int pointsAwarded;

  /// The submitter's balance after crediting.
  final int pointsBalance;

  final int rateApplied;
  final String rateMaterialType;

  factory VerifyResult.fromJson(Map<String, dynamic> json) {
    final submission = json['submission'];
    if (submission is! Map) {
      throw const FormatException('verify response is missing the submission');
    }
    final rate = json['rate_applied'];
    final rateMap = rate is Map ? rate.cast<String, dynamic>() : const {};

    return VerifyResult(
      submission: Submission.fromJson(submission.cast<String, dynamic>()),
      pointsAwarded: (json['points_awarded'] as num?)?.toInt() ?? 0,
      pointsBalance: (json['points_balance'] as num?)?.toInt() ?? 0,
      rateApplied: (rateMap['points_per_kg'] as num?)?.toInt() ?? 0,
      rateMaterialType: rateMap['material_type'] as String? ?? '',
    );
  }
}

/// Result of `POST /submissions/classify` (Phase 2.5).
///
/// The endpoint never fails: a missing key, a timeout, a refusal or a garbled
/// model answer all arrive here as [degraded] rather than as an exception, and
/// the caller's response to all of them is the same — let the user pick the
/// material by hand (FR-23). So there is no error branch to handle, only
/// [degraded] to check.
class Classification {
  const Classification({
    required this.predictedCategory,
    required this.predictedConfidence,
    required this.label,
    required this.group,
    required this.requiresSourceType,
    required this.alternatives,
    required this.latencyMs,
    required this.degraded,
    this.reason,
  });

  /// Taxonomy key the model predicted. Empty when [degraded].
  final String predictedCategory;

  /// Model confidence in [0,1]. Zero when [degraded].
  final double predictedConfidence;

  /// Human-readable label for [predictedCategory], e.g. "PET bottles".
  final String label;

  /// Taxonomy group, e.g. "plastics".
  final String group;

  /// True when the group is organic and the user must be asked hotel vs
  /// residential before submitting (FR-24).
  final bool requiresSourceType;

  /// Runner-up guesses, most confident first — offered as one-tap corrections so
  /// a near miss costs a tap rather than a scroll through fourteen categories.
  final List<ClassificationAlternative> alternatives;

  /// Server-measured round trip, shown so a slow classify is visible rather than
  /// mysterious.
  final int latencyMs;

  /// True when no usable prediction was produced. Fall back to manual selection.
  final bool degraded;

  /// Why it degraded, when the server chose to say.
  final String? reason;

  /// Whether this carries a prediction worth showing the user.
  bool get hasPrediction => !degraded && predictedCategory.isNotEmpty;

  /// Confidence as whole percent, for display.
  int get confidencePercent => (predictedConfidence * 100).round();

  /// Whether the prediction is weak enough that the UI should lead with doubt.
  ///
  /// The threshold is a presentation choice, not a correctness one: below it the
  /// card asks rather than asserts. Points always come from the collector's
  /// measurement, so a wrong guess here costs a tap, never points.
  bool get isLowConfidence => predictedConfidence < 0.6;

  factory Classification.fromJson(Map<String, dynamic> json) {
    final rawAlternatives = json['alternatives'] as List? ?? const [];

    return Classification(
      predictedCategory: json['predicted_category'] as String? ?? '',
      predictedConfidence:
          (json['predicted_confidence'] as num?)?.toDouble() ?? 0,
      label: json['label'] as String? ?? '',
      group: json['group'] as String? ?? '',
      requiresSourceType: json['requires_source_type'] as bool? ?? false,
      alternatives: rawAlternatives
          .whereType<Map>()
          .map((a) =>
              ClassificationAlternative.fromJson(a.cast<String, dynamic>()))
          .toList(growable: false),
      latencyMs: (json['latency_ms'] as num?)?.toInt() ?? 0,
      degraded: json['degraded'] as bool? ?? false,
      reason: json['reason'] as String?,
    );
  }
}

/// A runner-up category from a classification.
class ClassificationAlternative {
  const ClassificationAlternative({
    required this.predictedCategory,
    required this.predictedConfidence,
  });

  final String predictedCategory;
  final double predictedConfidence;

  factory ClassificationAlternative.fromJson(Map<String, dynamic> json) =>
      ClassificationAlternative(
        predictedCategory: json['predicted_category'] as String? ?? '',
        predictedConfidence:
            (json['predicted_confidence'] as num?)?.toDouble() ?? 0,
      );
}

/// A partner retailer, as embedded in a voucher.
class VoucherPartner {
  const VoucherPartner({
    required this.id,
    required this.name,
    required this.logoUrl,
    required this.active,
  });

  final int id;
  final String name;
  final String? logoUrl;
  final bool active;

  /// Initials for the partner badge, at most two letters — the catalogue has no
  /// logo assets yet, and a lettermark reads better than a generic placeholder.
  String get initials {
    final parts = name.trim().split(RegExp(r'\s+')).where((p) => p.isNotEmpty);
    if (parts.isEmpty) return '?';
    return parts.take(2).map((p) => p[0].toUpperCase()).join();
  }

  factory VoucherPartner.fromJson(Map<String, dynamic> json) => VoucherPartner(
        id: (json['id'] as num?)?.toInt() ?? 0,
        name: json['name'] as String? ?? '',
        logoUrl: json['logo_url'] as String?,
        active: json['active'] as bool? ?? true,
      );
}

/// Discount types (docs/06 §2 vouchers.discount_type).
abstract final class DiscountType {
  static const percentage = 'percentage';
  static const fixed = 'fixed';
}

/// A partner voucher from the Phase 3 catalogue.
class Voucher {
  const Voucher({
    required this.id,
    required this.partnerId,
    required this.title,
    required this.pointsCost,
    required this.discountType,
    required this.discountValue,
    required this.expiryDays,
    required this.stockRemaining,
    required this.active,
    required this.partner,
    required this.affordable,
  });

  final int id;
  final int partnerId;
  final String title;
  final int pointsCost;
  final String discountType;
  final double discountValue;

  /// Days the code stays valid once redeemed.
  final int expiryDays;

  /// Remaining stock, or null for unlimited. Null is not zero: a null here means
  /// always available, so it must never render as "0 left".
  final int? stockRemaining;

  final bool active;
  final VoucherPartner partner;

  /// Whether the signed-in user can afford this right now.
  ///
  /// Computed by the server against the live balance, never re-derived here: the
  /// Phase 4 redemption deducts against the same comparison, so a client that
  /// decided for itself would eventually enable a button the server refuses.
  final bool affordable;

  /// The discount as a short phrase: `10% off`, `KSh 100 off`.
  String get discountLabel {
    if (discountType == DiscountType.percentage) {
      return '${_trimZeros(discountValue)}% off';
    }
    return 'KSh ${_trimZeros(discountValue)} off';
  }

  /// Stock phrasing, or null when unlimited or comfortably stocked — scarcity is
  /// only worth saying when it is nearly true.
  String? get scarcityLabel {
    final remaining = stockRemaining;
    if (remaining == null || remaining > 10) return null;
    if (remaining == 1) return 'Last one';
    return 'Only $remaining left';
  }

  /// How close the user is to affording this, in 0..1.
  ///
  /// Needs the balance because [affordable] is a yes/no and a progress bar wants
  /// the distance. Returns 1 when already affordable.
  double progressFrom(int balance) {
    if (pointsCost <= 0) return 1;
    final ratio = balance / pointsCost;
    return ratio.clamp(0.0, 1.0);
  }

  /// Points still needed, or 0 when already affordable.
  int shortfallFrom(int balance) {
    final gap = pointsCost - balance;
    return gap > 0 ? gap : 0;
  }

  static String _trimZeros(double value) {
    if (value == value.roundToDouble()) return value.round().toString();
    return value.toString();
  }

  factory Voucher.fromJson(Map<String, dynamic> json) {
    final partner = json['partner'];

    return Voucher(
      id: (json['id'] as num?)?.toInt() ?? 0,
      partnerId: (json['partner_id'] as num?)?.toInt() ?? 0,
      title: json['title'] as String? ?? '',
      pointsCost: (json['points_cost'] as num?)?.toInt() ?? 0,
      discountType: json['discount_type'] as String? ?? DiscountType.fixed,
      discountValue: (json['discount_value'] as num?)?.toDouble() ?? 0,
      expiryDays: (json['expiry_days'] as num?)?.toInt() ?? 0,
      stockRemaining: (json['stock_remaining'] as num?)?.toInt(),
      active: json['active'] as bool? ?? true,
      partner: partner is Map
          ? VoucherPartner.fromJson(partner.cast<String, dynamic>())
          : const VoucherPartner(id: 0, name: '', logoUrl: null, active: true),
      affordable: json['affordable'] as bool? ?? false,
    );
  }
}

/// The catalogue plus the balance it was computed against.
///
/// The balance travels with the list so the screen can show "you have N points"
/// and explain each affordability verdict without a second call to `/me`.
class VoucherCatalogue {
  const VoucherCatalogue({required this.vouchers, required this.pointsBalance});

  final List<Voucher> vouchers;
  final int pointsBalance;

  factory VoucherCatalogue.fromJson(Map<String, dynamic> json) {
    final raw = json['vouchers'] as List? ?? const [];
    return VoucherCatalogue(
      vouchers: raw
          .whereType<Map>()
          .map((v) => Voucher.fromJson(v.cast<String, dynamic>()))
          .toList(growable: false),
      pointsBalance: (json['points_balance'] as num?)?.toInt() ?? 0,
    );
  }
}

/// Redemption status values (FR-14). `used` and `expired` are terminal, and the
/// client only ever reads these — the server owns every transition.
abstract final class RedemptionStatus {
  static const issued = 'issued';
  static const used = 'used';
  static const expired = 'expired';
}

/// A redeemed voucher: the code shown at a partner till, and its state.
class Redemption {
  const Redemption({
    required this.id,
    required this.userId,
    required this.voucherId,
    required this.code,
    required this.status,
    required this.issuedAt,
    required this.expiry,
    required this.qrPayload,
    this.usedAt,
    this.verifiedBy,
    this.voucher,
  });

  final int id;
  final int userId;
  final int voucherId;

  /// The `redemption_code` — a UUID, read aloud or typed in at a till.
  final String code;

  /// `issued` | `used` | `expired`.
  final String status;

  final DateTime issuedAt;

  /// When the code stops working, derived server-side as
  /// `issued_at + voucher.expiry_days` and never recomputed here: the server owns
  /// the rule, and a client deriving its own would disagree the moment an admin
  /// edited `expiry_days`.
  final DateTime expiry;

  /// The deep link encoded in the QR, e.g. `zoa://redeem/<code>`.
  final String qrPayload;

  /// When the code was accepted at a till — null until then, never 0.
  final DateTime? usedAt;

  /// The partner-staff account that accepted it — null until then.
  final int? verifiedBy;

  /// The voucher this bought. Present on a history listing, which embeds it so
  /// "My Redemptions" renders in one call; null on a create or verify response,
  /// where the voucher arrives as a sibling field instead.
  final Voucher? voucher;

  bool get isIssued => status == RedemptionStatus.issued;
  bool get isUsed => status == RedemptionStatus.used;
  bool get isExpired => status == RedemptionStatus.expired;

  /// Whether this code can still be spent.
  ///
  /// Checks the clock as well as the status, because the server only transitions a
  /// row to `expired` when someone actually tries to verify it — so a code read
  /// from history can still say `issued` while being past its date.
  bool get isRedeemable => isIssued && DateTime.now().isBefore(expiry);

  /// Whole days left before expiry; negative once past.
  int get daysUntilExpiry => expiry.difference(DateTime.now()).inDays;

  /// Status as a short label, always paired with an icon where it is shown so
  /// status never rests on colour alone (UI/UX §5).
  String get statusLabel {
    if (isUsed) return 'Used';
    if (isExpired || !isRedeemable) return 'Expired';
    return 'Ready to use';
  }

  /// The leading block of the code, for a list row too narrow for all 36
  /// characters. Never what a cashier verifies against — that needs the whole code.
  String get shortCode =>
      code.length <= 8 ? code.toUpperCase() : code.substring(0, 8).toUpperCase();

  /// This redemption with its voucher attached.
  ///
  /// A create response carries the voucher as a sibling field rather than nested,
  /// so this reunites the two before the new code joins the history — otherwise a
  /// just-redeemed row would render without a partner name until the next reload.
  Redemption withVoucher(Voucher attached) => Redemption(
        id: id,
        userId: userId,
        voucherId: voucherId,
        code: code,
        status: status,
        issuedAt: issuedAt,
        expiry: expiry,
        qrPayload: qrPayload,
        usedAt: usedAt,
        verifiedBy: verifiedBy,
        voucher: attached,
      );

  factory Redemption.fromJson(Map<String, dynamic> json) {
    final voucher = json['voucher'];

    return Redemption(
      id: (json['id'] as num?)?.toInt() ?? 0,
      userId: (json['user_id'] as num?)?.toInt() ?? 0,
      voucherId: (json['voucher_id'] as num?)?.toInt() ?? 0,
      code: json['redemption_code'] as String? ?? '',
      status: json['status'] as String? ?? RedemptionStatus.issued,
      issuedAt:
          DateTime.tryParse(json['issued_at'] as String? ?? '')?.toLocal() ??
              DateTime.fromMillisecondsSinceEpoch(0),
      expiry: DateTime.tryParse(json['expiry'] as String? ?? '')?.toLocal() ??
          DateTime.fromMillisecondsSinceEpoch(0),
      qrPayload: json['qr_payload'] as String? ?? '',
      usedAt: DateTime.tryParse(json['used_at'] as String? ?? '')?.toLocal(),
      verifiedBy: (json['verified_by'] as num?)?.toInt(),
      voucher: voucher is Map
          ? Voucher.fromJson(voucher.cast<String, dynamic>())
          : null,
    );
  }
}

/// Result of `POST /redemptions` — the new code, and what it cost.
class RedemptionResult {
  const RedemptionResult({
    required this.redemption,
    required this.voucher,
    required this.pointsSpent,
    required this.pointsBalance,
  });

  final Redemption redemption;

  /// The voucher just bought, a sibling of the redemption in this response.
  final Voucher voucher;

  final int pointsSpent;

  /// The balance after the deduction, straight from the server. The app never
  /// subtracts locally, so what it shows is what was actually persisted.
  final int pointsBalance;

  factory RedemptionResult.fromJson(Map<String, dynamic> json) {
    final redemption = json['redemption'];
    if (redemption is! Map) {
      throw const FormatException('redemption response is missing the redemption');
    }
    final voucher = json['voucher'];
    if (voucher is! Map) {
      throw const FormatException('redemption response is missing the voucher');
    }

    return RedemptionResult(
      redemption: Redemption.fromJson(redemption.cast<String, dynamic>()),
      voucher: Voucher.fromJson(voucher.cast<String, dynamic>()),
      pointsSpent: (json['points_spent'] as num?)?.toInt() ?? 0,
      pointsBalance: (json['points_balance'] as num?)?.toInt() ?? 0,
    );
  }
}

/// Result of `POST /redemptions/:code/verify` — the answer a cashier acts on.
class RedemptionVerification {
  const RedemptionVerification({
    required this.status,
    required this.redemption,
    required this.voucher,
    required this.userName,
    required this.message,
  });

  /// The status after the transition — `used` on success.
  final String status;

  final Redemption redemption;
  final Voucher voucher;

  /// Who the code belongs to. A cashier is confirming a person, not a row id.
  final String userName;

  /// What to do at the till, phrased by the server so both halves of the product
  /// describe a discount the same way.
  final String message;

  factory RedemptionVerification.fromJson(Map<String, dynamic> json) {
    final redemption = json['redemption'];
    if (redemption is! Map) {
      throw const FormatException('verify response is missing the redemption');
    }
    final voucher = json['voucher'];
    if (voucher is! Map) {
      throw const FormatException('verify response is missing the voucher');
    }
    final user = json['user'];
    final userMap = user is Map ? user.cast<String, dynamic>() : const {};

    return RedemptionVerification(
      status: json['status'] as String? ?? RedemptionStatus.used,
      redemption: Redemption.fromJson(redemption.cast<String, dynamic>()),
      voucher: Voucher.fromJson(voucher.cast<String, dynamic>()),
      userName: userMap['name'] as String? ?? '',
      message: json['message'] as String? ?? 'Code accepted.',
    );
  }
}

/// Normalises whatever landed in the verification field into a bare code.
///
/// Accepts the code itself or the QR payload (`zoa://redeem/<code>`), since both
/// end up on a clipboard in practice, and trims the whitespace a paste often
/// carries. Lower-cased because the server compares the column exactly and emits
/// lower-case UUIDs — so a code typed in capitals still matches, and since a UUID
/// is only hex and dashes, nothing is lost.
String normaliseRedemptionCode(String raw) {
  const prefix = 'zoa://redeem/';

  var value = raw.trim();
  if (value.toLowerCase().startsWith(prefix)) {
    value = value.substring(prefix.length);
  }
  return value.trim().toLowerCase();
}

/// Platform statistics from `GET /admin/stats` (Phase 5).
///
/// Flat rather than mirroring the response's five nested blocks: one screen reads
/// this, and five small classes would add indirection without adding a decision.
class AdminStats {
  const AdminStats({
    required this.usersTotal,
    required this.usersByRole,
    required this.submissionsTotal,
    required this.submissionsByStatus,
    required this.totalVerifiedKg,
    required this.pointsIssued,
    required this.pointsSpent,
    required this.pointsOutstanding,
    required this.redemptionsTotal,
    required this.redemptionsByStatus,
    required this.predictionsMade,
    required this.verifiedAgainst,
    required this.correctPredictions,
    this.accuracy,
  });

  final int usersTotal;
  final Map<String, int> usersByRole;

  final int submissionsTotal;
  final Map<String, int> submissionsByStatus;

  /// Weight a collector actually measured, not what users estimated — the only
  /// figure the platform can stand behind.
  final double totalVerifiedKg;

  final int pointsIssued;
  final int pointsSpent;

  /// Points earned and not yet spent — the platform's outstanding liability.
  final int pointsOutstanding;

  final int redemptionsTotal;
  final Map<String, int> redemptionsByStatus;

  final int predictionsMade;

  /// Predictions a collector has since verified — the only ones with a
  /// trustworthy answer to compare against.
  final int verifiedAgainst;

  final int correctPredictions;

  /// Model accuracy in [0,1], or **null** when nothing has been verified yet.
  ///
  /// Null is not zero, and the screen must not render it as a score: an untested
  /// model is not a model that is wrong every time.
  final double? accuracy;

  /// Accuracy as whole percent, or null when there is no data.
  int? get accuracyPercent {
    final value = accuracy;
    return value == null ? null : (value * 100).round();
  }

  static Map<String, int> _counts(Object? raw) {
    if (raw is! Map) return const {};
    return raw.map((key, value) =>
        MapEntry(key.toString(), (value as num?)?.toInt() ?? 0));
  }

  factory AdminStats.fromJson(Map<String, dynamic> json) {
    Map<String, dynamic> block(String key) {
      final value = json[key];
      return value is Map ? value.cast<String, dynamic>() : const {};
    }

    final users = block('users');
    final submissions = block('submissions');
    final points = block('points');
    final redemptions = block('redemptions');
    final classification = block('classification');

    return AdminStats(
      usersTotal: (users['total'] as num?)?.toInt() ?? 0,
      usersByRole: _counts(users['by_role']),
      submissionsTotal: (submissions['total'] as num?)?.toInt() ?? 0,
      submissionsByStatus: _counts(submissions['by_status']),
      totalVerifiedKg: (submissions['total_verified_kg'] as num?)?.toDouble() ?? 0,
      pointsIssued: (points['total_issued'] as num?)?.toInt() ?? 0,
      pointsSpent: (points['total_spent'] as num?)?.toInt() ?? 0,
      pointsOutstanding: (points['outstanding'] as num?)?.toInt() ?? 0,
      redemptionsTotal: (redemptions['total'] as num?)?.toInt() ?? 0,
      redemptionsByStatus: _counts(redemptions['by_status']),
      predictionsMade: (classification['predictions_made'] as num?)?.toInt() ?? 0,
      verifiedAgainst: (classification['verified_against'] as num?)?.toInt() ?? 0,
      correctPredictions: (classification['correct'] as num?)?.toInt() ?? 0,
      // Left null when the server sends null — the empty case, which must never
      // become 0.0 on the way in.
      accuracy: (classification['accuracy'] as num?)?.toDouble(),
    );
  }
}
