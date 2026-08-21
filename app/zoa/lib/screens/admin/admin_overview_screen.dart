/// Admin Overview — the platform in one screen (Phase 5).
///
/// Read-only and deliberately small: `07_Implementation_Plan.md` calls this a
/// "minor admin overview screen", optional and second in line behind demo polish.
/// So it states the numbers and nothing else — no editing, no drill-down.
///
/// The classification card is the point of it. FR-22's payoff is being able to say
/// what the AI's accuracy actually *is*, measured against the collectors who
/// checked its work, rather than asserting that the feature exists.
///
/// State lives in this widget rather than in a controller: one read-only call, one
/// reader, nothing to share and nothing to clear on sign-out — the screen is
/// disposed with the shell when the session ends.
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../api/api_client.dart';
import '../../api/api_exception.dart';
import '../../api/api_models.dart';
import '../../theme/zoa_colors.dart';
import '../../theme/zoa_theme.dart';
import '../../util/format.dart';
import '../../widgets/fade_slide_in.dart';
import '../../widgets/zoa_empty_state.dart';
import '../../widgets/zoa_ui.dart';

class AdminOverviewScreen extends StatefulWidget {
  const AdminOverviewScreen({super.key});

  @override
  State<AdminOverviewScreen> createState() => _AdminOverviewScreenState();
}

class _AdminOverviewScreenState extends State<AdminOverviewScreen> {
  AdminStats? _stats;
  ApiException? _error;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _load());
  }

  Future<void> _load() async {
    try {
      final stats = await context.read<ApiClient>().adminStats();
      if (!mounted) return;
      setState(() {
        _stats = stats;
        _error = null;
        _loading = false;
      });
    } on ApiException catch (error) {
      if (!mounted) return;
      setState(() {
        // Keep the last good numbers on a failed refresh; only a first load is
        // allowed to fall through to the error state.
        _error = error;
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: ZoaColors.paper,
      appBar: AppBar(
        backgroundColor: ZoaColors.paper,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        foregroundColor: ZoaColors.forestDeep,
        title: Text('Platform', style: ZoaType.label),
      ),
      body: SafeArea(
        child: RefreshIndicator(
          onRefresh: _load,
          color: ZoaColors.forestDeep,
          backgroundColor: ZoaColors.paperCard,
          child: _body(),
        ),
      ),
    );
  }

  Widget _body() {
    if (_loading) {
      return const Center(child: CircularProgressIndicator(color: ZoaColors.leaf));
    }

    final stats = _stats;
    if (stats == null) {
      return ZoaErrorState(
        title: 'Could not load statistics',
        message: _error?.message ?? 'Try again in a moment.',
        onRetry: _load,
      );
    }

    return ZoaPage(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: FadeSlideIn.staggered([
          const SizedBox(height: ZoaSpace.sm),
          const ZoaEyebrow('Admin'),
          const SizedBox(height: ZoaSpace.md),
          Text('The loop,\nin numbers', style: ZoaType.h2),
          const SizedBox(height: ZoaSpace.xl),

          // Classification first: it is the hardest thing to claim and the easiest
          // to show, so it leads.
          _ClassificationCard(stats: stats),
          const SizedBox(height: ZoaSpace.lg),

          _MaterialCard(stats: stats),
          const SizedBox(height: ZoaSpace.lg),
          _PointsCard(stats: stats),
          const SizedBox(height: ZoaSpace.lg),
          _CountsCard(stats: stats),
          const SizedBox(height: ZoaSpace.xl),
        ]),
      ),
    );
  }
}

/// The FR-22 card: how often the model agreed with the human who checked it.
class _ClassificationCard extends StatelessWidget {
  const _ClassificationCard({required this.stats});

  final AdminStats stats;

  @override
  Widget build(BuildContext context) {
    final percent = stats.accuracyPercent;

    return ZoaCard(
      accent: true,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const ZoaKicker('AI classification'),
          const SizedBox(height: ZoaSpace.md),
          if (percent == null)
            // No denominator yet. Stated as absence, never as a score — "0%" would
            // read as a model that is always wrong.
            Text(
              'No verified predictions yet. Accuracy appears once a collector has '
              'checked a photographed submission.',
              style: ZoaType.bodySm,
            )
          else ...[
            Row(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                Text('$percent', style: ZoaType.pointsHero.copyWith(fontSize: 40)),
                const SizedBox(width: 2),
                Padding(
                  padding: const EdgeInsets.only(bottom: 6),
                  child: Text('%', style: ZoaType.h3),
                ),
                const Spacer(),
                Text(
                  '${stats.correctPredictions} of ${stats.verifiedAgainst}\nverified',
                  textAlign: TextAlign.right,
                  style: ZoaType.bodySm,
                ),
              ],
            ),
            const SizedBox(height: ZoaSpace.sm),
            Text(
              'Measured against what collectors actually confirmed — a corrected '
              'material counts as a miss.',
              style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
            ),
          ],
          const SizedBox(height: ZoaSpace.md),
          const Divider(height: 1, color: ZoaColors.goldBorder),
          const SizedBox(height: ZoaSpace.md),
          _StatRow(label: 'Predictions made', value: formatPoints(stats.predictionsMade)),
          _StatRow(label: 'Verified against', value: formatPoints(stats.verifiedAgainst)),
        ],
      ),
    );
  }
}

/// What was actually collected.
class _MaterialCard extends StatelessWidget {
  const _MaterialCard({required this.stats});

  final AdminStats stats;

  @override
  Widget build(BuildContext context) {
    return ZoaCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const ZoaKicker('Recycling'),
          const SizedBox(height: ZoaSpace.md),
          Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              Text(
                _trimZeros(stats.totalVerifiedKg),
                style: ZoaType.statNum,
              ),
              const SizedBox(width: 4),
              Padding(
                padding: const EdgeInsets.only(bottom: 5),
                child: Text('kg verified', style: ZoaType.bodySm),
              ),
            ],
          ),
          const SizedBox(height: ZoaSpace.xs),
          Text(
            // The distinction that makes the number defensible.
            'Weight a collector measured, not what users estimated.',
            style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
          ),
          const SizedBox(height: ZoaSpace.md),
          const Divider(height: 1, color: ZoaColors.line),
          const SizedBox(height: ZoaSpace.md),
          _StatRow(label: 'Submissions', value: formatPoints(stats.submissionsTotal)),
          for (final entry in _ordered(stats.submissionsByStatus, const [
            SubmissionStatus.pending,
            SubmissionStatus.collected,
            SubmissionStatus.verified,
            SubmissionStatus.rejected,
          ]))
            _StatRow(label: '· ${entry.key}', value: formatPoints(entry.value), muted: true),
        ],
      ),
    );
  }

  static String _trimZeros(double value) {
    if (value == value.roundToDouble()) return value.round().toString();
    return value.toStringAsFixed(1);
  }
}

/// The ledger, aggregated.
class _PointsCard extends StatelessWidget {
  const _PointsCard({required this.stats});

  final AdminStats stats;

  @override
  Widget build(BuildContext context) {
    return ZoaCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const ZoaKicker('Points'),
          const SizedBox(height: ZoaSpace.md),
          _StatRow(label: 'Issued', value: formatPoints(stats.pointsIssued)),
          _StatRow(label: 'Spent', value: formatPoints(stats.pointsSpent)),
          _StatRow(
            label: 'Outstanding',
            value: formatPoints(stats.pointsOutstanding),
            emphasised: true,
          ),
          const SizedBox(height: ZoaSpace.sm),
          Text(
            // Worth saying out loud: this is the number that makes the model work
            // without any cash leaving the business.
            'Outstanding points are a discount liability, never a cash one — no '
            'payout risk.',
            style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
          ),
        ],
      ),
    );
  }
}

/// Accounts and codes.
class _CountsCard extends StatelessWidget {
  const _CountsCard({required this.stats});

  final AdminStats stats;

  @override
  Widget build(BuildContext context) {
    return ZoaCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const ZoaKicker('Accounts & codes'),
          const SizedBox(height: ZoaSpace.md),
          _StatRow(label: 'Users', value: formatPoints(stats.usersTotal)),
          for (final entry in _ordered(stats.usersByRole, const [
            'user',
            'collector',
            'partner_staff',
            'admin',
          ]))
            _StatRow(
              label: '· ${entry.key.replaceAll('_', ' ')}',
              value: formatPoints(entry.value),
              muted: true,
            ),
          const SizedBox(height: ZoaSpace.md),
          const Divider(height: 1, color: ZoaColors.line),
          const SizedBox(height: ZoaSpace.md),
          _StatRow(label: 'Codes issued', value: formatPoints(stats.redemptionsTotal)),
          for (final entry in _ordered(stats.redemptionsByStatus, const [
            RedemptionStatus.issued,
            RedemptionStatus.used,
            RedemptionStatus.expired,
          ]))
            _StatRow(label: '· ${entry.key}', value: formatPoints(entry.value), muted: true),
        ],
      ),
    );
  }
}

/// Orders a count map by a known key order.
///
/// The server sends every key, including zeros, so this is about presentation
/// order rather than filling gaps: a map iterated as-is would reorder between
/// builds and make the list look unstable.
List<MapEntry<String, int>> _ordered(Map<String, int> counts, List<String> order) {
  return [
    for (final key in order)
      if (counts.containsKey(key)) MapEntry(key, counts[key]!),
  ];
}

class _StatRow extends StatelessWidget {
  const _StatRow({
    required this.label,
    required this.value,
    this.muted = false,
    this.emphasised = false,
  });

  final String label;
  final String value;
  final bool muted;
  final bool emphasised;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(
        children: [
          Expanded(
            child: Text(
              label,
              style: ZoaType.bodySm.copyWith(
                color: muted ? ZoaColors.inkSoft : ZoaColors.ink,
                fontSize: muted ? 13 : 14.5,
              ),
            ),
          ),
          Text(
            value,
            style: ZoaType.mono.copyWith(
              fontSize: muted ? 12.5 : 14,
              color: emphasised ? ZoaColors.goldDeep : ZoaColors.ink,
              fontWeight: emphasised ? FontWeight.w600 : FontWeight.w400,
            ),
          ),
        ],
      ),
    );
  }
}
