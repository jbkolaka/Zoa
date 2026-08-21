/// System status — backend, database and material rates.
///
/// Was the Home tab in Phase 0, proving the app could reach the backend. Now that
/// Home shows the points balance, this lives behind Profile → System status,
/// because "is the backend actually up?" is the first thing to check when a demo
/// misbehaves.
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_models.dart';
import '../state/app_status.dart';
import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';
import '../widgets/fade_slide_in.dart';
import '../widgets/zoa_empty_state.dart';
import '../widgets/zoa_ui.dart';

class StatusScreen extends StatelessWidget {
  const StatusScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final status = context.watch<AppStatus>();
    final health = status.health;

    if (health == null) {
      return ZoaErrorState(
        title: 'Not connected',
        message: status.error?.message ?? 'The backend is unreachable.',
        onRetry: () => context.read<AppStatus>().check(),
      );
    }

    return RefreshIndicator(
      onRefresh: () => context.read<AppStatus>().check(),
      color: ZoaColors.forestDeep,
      backgroundColor: ZoaColors.paperCard,
      child: ZoaPage(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: FadeSlideIn.staggered([
            const ZoaEyebrow('Platform status'),
            const SizedBox(height: ZoaSpace.md),
            Text('The loop is wired.', style: ZoaType.h2),
            const SizedBox(height: ZoaSpace.md),
            Text(
              'The app is talking to the Go backend and the SQLite schema is in '
              'place. Every rate below is read live from the server.',
              style: ZoaType.bodySoft,
            ),
            const SizedBox(height: ZoaSpace.xl),
            _ServiceCard(health: health, baseUrl: status.baseUrl),
            const SizedBox(height: ZoaSpace.lg),
            _TaxonomyCard(meta: status.meta),
            const SizedBox(height: ZoaSpace.xl),
          ]),
        ),
      ),
    );
  }
}

/// [StatusScreen] as a pushable route, with its own app bar.
///
/// From Phase 1 the Home tab shows the points balance, so this diagnostic view
/// moves behind Profile → System status. It stays reachable because "is the
/// backend actually up?" is the first question to ask when a demo misbehaves.
class StatusScreenPage extends StatelessWidget {
  const StatusScreenPage({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: ZoaColors.paper,
      appBar: AppBar(
        backgroundColor: ZoaColors.paper,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        foregroundColor: ZoaColors.forestDeep,
        title: Text('System status', style: ZoaType.label),
      ),
      body: const SafeArea(child: StatusScreen()),
    );
  }
}

/// Live service facts, in mono because they are data.
class _ServiceCard extends StatelessWidget {
  const _ServiceCard({required this.health, required this.baseUrl});

  final HealthStatus health;
  final String baseUrl;

  @override
  Widget build(BuildContext context) {
    return ZoaCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const ZoaKicker('Backend'),
              const Spacer(),
              _StatusDot(healthy: health.isHealthy),
            ],
          ),
          const SizedBox(height: ZoaSpace.md),
          _Row(label: 'Service', value: health.service),
          _Row(label: 'Version', value: health.version),
          _Row(label: 'Environment', value: health.env),
          _Row(label: 'Host', value: baseUrl),
          const Padding(
            padding: EdgeInsets.symmetric(vertical: ZoaSpace.md),
            child: Divider(color: ZoaColors.line, height: 1),
          ),
          const ZoaKicker('Database'),
          const SizedBox(height: ZoaSpace.md),
          _Row(
            label: 'Connected',
            value: health.databaseConnected ? 'yes' : 'no',
          ),
          _Row(
            label: 'Migrations',
            value: '${health.migrationsApplied} applied',
          ),
          _Row(label: 'Schema', value: health.schemaVersion),
        ],
      ),
    );
  }
}

/// The material taxonomy and its configured points rates, straight from
/// `GET /meta` — proof the reference data round-trips, and the same source the
/// Phase 2 submission form will read.
class _TaxonomyCard extends StatelessWidget {
  const _TaxonomyCard({required this.meta});

  final MetaCatalog? meta;

  /// Display labels for the taxonomy's group keys.
  static const _groupLabels = {
    'plastics': 'Plastics',
    'paper': 'Paper & cardboard',
    'glass': 'Glass',
    'metal': 'Metal',
    'organic': 'Organic waste',
  };

  @override
  Widget build(BuildContext context) {
    final catalog = meta;

    if (catalog == null) {
      return ZoaCard(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const ZoaKicker('Materials'),
            const SizedBox(height: ZoaSpace.sm),
            Text(
              'The taxonomy could not be loaded. Submissions will fall back to '
              'manual material selection.',
              style: ZoaType.bodySm,
            ),
          ],
        ),
      );
    }

    final grouped = catalog.byGroup;

    return ZoaCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const ZoaKicker('Materials tracked'),
              const Spacer(),
              ZoaTag('${catalog.materials.length} categories'),
            ],
          ),
          const SizedBox(height: ZoaSpace.sm),
          Text(
            'Rates are server-side and admin-configurable, so the app never '
            'holds a stale copy.',
            style: ZoaType.bodySm,
          ),
          const SizedBox(height: ZoaSpace.lg),
          for (final entry in grouped.entries) ...[
            Text(
              _groupLabels[entry.key] ?? entry.key,
              style: ZoaType.label,
            ),
            const SizedBox(height: ZoaSpace.sm),
            for (final material in entry.value)
              Padding(
                padding: const EdgeInsets.only(bottom: 6),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(material.label, style: ZoaType.bodySm),
                    ),
                    // Gold, because it is about points.
                    Text(
                      '${material.pointsPerKg} pts/kg',
                      style: ZoaType.pointsCost,
                    ),
                  ],
                ),
              ),
            if (entry.key != grouped.keys.last)
              const SizedBox(height: ZoaSpace.md),
          ],
        ],
      ),
    );
  }
}

/// A label/value line. Label in sans, value in mono — data reads as data.
class _Row extends StatelessWidget {
  const _Row({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 110,
            child: Text(label, style: ZoaType.bodySm),
          ),
          Expanded(
            child: Text(
              value.isEmpty ? '—' : value,
              style: ZoaType.mono.copyWith(color: ZoaColors.ink),
            ),
          ),
        ],
      ),
    );
  }
}

/// Health indicator. Always paired with a text label, never colour alone
/// (UI/UX doc §5).
class _StatusDot extends StatelessWidget {
  const _StatusDot({required this.healthy});

  final bool healthy;

  @override
  Widget build(BuildContext context) {
    final color = healthy ? ZoaColors.statusSuccess : ZoaColors.statusError;

    return Row(
      children: [
        Container(
          width: 8,
          height: 8,
          decoration: BoxDecoration(color: color, shape: BoxShape.circle),
        ),
        const SizedBox(width: 6),
        Text(
          healthy ? 'live' : 'down',
          style: ZoaType.tag.copyWith(color: color),
        ),
      ],
    );
  }
}
