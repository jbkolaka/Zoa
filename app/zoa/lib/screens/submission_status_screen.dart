/// Submission Status — track one submission through the loop.
///
/// The seal does real work here: `highlightStage` marks which quarter of the loop
/// the submission has reached, so the screen answers "where is this?" before any
/// text is read.
library;

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../api/api_models.dart';
import '../state/app_status.dart';
import '../state/auth_controller.dart';
import '../state/submissions_controller.dart';
import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';
import '../util/format.dart';
import '../widgets/fade_slide_in.dart';
import '../widgets/loop_seal.dart';
import '../widgets/material_icons.dart';
import '../widgets/zoa_empty_state.dart';
import '../widgets/zoa_ui.dart';

class SubmissionStatusScreen extends StatefulWidget {
  const SubmissionStatusScreen({super.key, required this.submissionId});

  final int submissionId;

  @override
  State<SubmissionStatusScreen> createState() => _SubmissionStatusScreenState();
}

class _SubmissionStatusScreenState extends State<SubmissionStatusScreen> {
  /// App Flow §1 has the client poll for the balance update rather than receive a
  /// push — push notifications are explicitly a stretch item. Eight seconds is
  /// slow enough to be cheap and fast enough that a live demo does not stall.
  static const _pollInterval = Duration(seconds: 8);

  Timer? _poll;
  Submission? _submission;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _load());
  }

  @override
  void dispose() {
    _poll?.cancel();
    super.dispose();
  }

  Future<void> _load() async {
    final submissions = context.read<SubmissionsController>();
    final loaded = await submissions.refreshOne(widget.submissionId);

    if (!mounted) return;
    setState(() {
      // Only the first load is allowed to leave this null and fall through to
      // the error state. A failed manual refresh keeps the submission already on
      // screen rather than replacing it with a dead end.
      if (loaded != null || _submission == null) _submission = loaded;
      _loading = false;
    });

    _schedulePoll();
  }

  /// Polls only while the outcome can still change. Once verified or rejected
  /// there is nothing left to wait for, so the timer stops rather than hitting
  /// the server forever.
  void _schedulePoll() {
    _poll?.cancel();

    final submission = _submission;
    if (submission == null || !submission.isOpen) return;

    _poll = Timer(_pollInterval, () async {
      if (!mounted) return;

      final updated =
          await context.read<SubmissionsController>().refreshOne(widget.submissionId);
      if (!mounted) return;

      final becameVerified = updated != null &&
          updated.isVerified &&
          !(_submission?.isVerified ?? false);

      // A failed poll — wifi blip, backend restart — must not discard the state
      // we already have. Nulling it here would blank the screen *and* stop the
      // timer (a null submission is not `isOpen`), stranding the user on an
      // error at the exact moment the collector's decision is what we await.
      // Hold the last-known submission and let the next tick try again.
      if (updated != null) setState(() => _submission = updated);

      // Points have landed — pull the new balance so every screen agrees.
      if (becameVerified) {
        await context.read<AuthController>().refresh();
        if (!mounted) return;
      }

      _schedulePoll();
    });
  }

  @override
  Widget build(BuildContext context) {
    final submission = _submission;

    return Scaffold(
      backgroundColor: ZoaColors.paper,
      appBar: AppBar(
        backgroundColor: ZoaColors.paper,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        foregroundColor: ZoaColors.forestDeep,
        title: Text('Submission #${widget.submissionId}', style: ZoaType.label),
      ),
      body: SafeArea(
        child: _body(context, submission),
      ),
    );
  }

  Widget _body(BuildContext context, Submission? submission) {
    if (_loading) {
      return const Center(
        child: LoopSeal.loading(size: 200, subLabel: 'loading…'),
      );
    }

    if (submission == null) {
      return ZoaErrorState(
        title: 'Could not load submission',
        message: context.read<SubmissionsController>().error?.message ??
            'That submission is no longer available.',
        onRetry: _load,
      );
    }

    return RefreshIndicator(
      onRefresh: _load,
      color: ZoaColors.forestDeep,
      backgroundColor: ZoaColors.paperCard,
      child: ZoaPage(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: FadeSlideIn.staggered([
            Center(
              child: LoopSeal(
                size: 210,
                highlightStage: _stageFor(submission),
                spinning: submission.isOpen,
              ),
            ),
            const SizedBox(height: ZoaSpace.xl),
            _StatusHeadline(submission: submission),
            const SizedBox(height: ZoaSpace.xl),
            _DetailCard(submission: submission),
            if (submission.isVerified) ...[
              const SizedBox(height: ZoaSpace.lg),
              _EarnedCard(submission: submission),
            ],
            const SizedBox(height: ZoaSpace.lg),
            _Timeline(submission: submission),
            const SizedBox(height: ZoaSpace.xl),
          ]),
        ),
      ),
    );
  }

  /// Maps a submission's status onto the quarter of the loop it occupies.
  LoopStage _stageFor(Submission submission) {
    if (submission.isVerified) return LoopStage.earn;
    if (submission.isCollected) return LoopStage.verify;
    return LoopStage.recycle;
  }
}

class _StatusHeadline extends StatelessWidget {
  const _StatusHeadline({required this.submission});

  final Submission submission;

  @override
  Widget build(BuildContext context) {
    final (title, blurb) = switch (submission.status) {
      SubmissionStatus.pending => (
          'Waiting for a collector',
          'A collector will confirm the material and weigh it. Points are '
              'credited from their measurement.',
        ),
      SubmissionStatus.collected => (
          'Picked up — being weighed',
          'A collector has your material and is confirming the type and weight.',
        ),
      SubmissionStatus.verified => (
          'Verified and credited',
          'The weight is confirmed and the points are in your balance.',
        ),
      SubmissionStatus.rejected => (
          'Not accepted',
          'The collector could not accept this submission. Nothing was '
              'credited — you can log a new one.',
        ),
      _ => ('Submitted', 'This submission is being processed.'),
    };

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        StatusPill(status: submission.status),
        const SizedBox(height: ZoaSpace.md),
        Text(title, style: ZoaType.h2),
        const SizedBox(height: ZoaSpace.sm),
        Text(blurb, style: ZoaType.bodySoft),
      ],
    );
  }
}

class _DetailCard extends StatelessWidget {
  const _DetailCard({required this.submission});

  final Submission submission;

  @override
  Widget build(BuildContext context) {
    final meta = context.watch<AppStatus>().meta;
    final label = meta?.labelFor(submission.materialType) ?? submission.materialType;

    return ZoaCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              MaterialIcon.forKey(submission.materialType, size: 34),
              const SizedBox(width: ZoaSpace.md),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(label, style: ZoaType.cardTitle),
                    const SizedBox(height: 2),
                    Text(
                      'Logged ${formatDate(submission.createdAt)}',
                      style: ZoaType.bodySm,
                    ),
                  ],
                ),
              ),
            ],
          ),
          const Padding(
            padding: EdgeInsets.symmetric(vertical: ZoaSpace.md),
            child: Divider(color: ZoaColors.line, height: 1),
          ),
          _Row(
            label: 'Your estimate',
            value: submission.estimatedQtyKg == null
                ? '—'
                : '${_kg(submission.estimatedQtyKg!)} kg',
          ),
          _Row(
            label: 'Verified weight',
            // An unverified weight reads as a dash, never 0 — "not yet measured"
            // and "measured at nothing" are different facts.
            value: submission.verifiedQtyKg == null
                ? 'not yet measured'
                : '${_kg(submission.verifiedQtyKg!)} kg',
          ),
          if (submission.location != null)
            _Row(label: 'Location', value: submission.location!),
        ],
      ),
    );
  }

  /// Trims a trailing `.0` so 4.0 shows as "4" but 4.5 stays "4.5".
  static String _kg(double value) {
    final text = value.toStringAsFixed(2);
    return text.replaceFirst(RegExp(r'\.?0+$'), '');
  }
}

/// The reward moment: what this submission earned.
class _EarnedCard extends StatelessWidget {
  const _EarnedCard({required this.submission});

  final Submission submission;

  @override
  Widget build(BuildContext context) {
    final points = submission.pointsAwarded ?? 0;

    return Container(
      padding: const EdgeInsets.all(ZoaSpace.xl),
      decoration: BoxDecoration(
        color: ZoaColors.forestDeep,
        borderRadius: ZoaRadius.allLg,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const ZoaKicker('Earned', onForest: true),
          const SizedBox(height: ZoaSpace.sm),
          Row(
            crossAxisAlignment: CrossAxisAlignment.baseline,
            textBaseline: TextBaseline.alphabetic,
            children: [
              Text(
                '+${formatPoints(points)}',
                style: ZoaType.pointsHero.copyWith(
                  color: ZoaColors.gold,
                  fontSize: 38,
                ),
              ),
              const SizedBox(width: ZoaSpace.sm),
              Text(
                points == 1 ? 'point' : 'points',
                style: ZoaType.mono.copyWith(color: ZoaColors.leafSoft),
              ),
            ],
          ),
          if (submission.verifiedAt != null) ...[
            const SizedBox(height: ZoaSpace.sm),
            Text(
              'Credited ${formatDate(submission.verifiedAt!)}',
              style: ZoaType.bodySm.copyWith(color: ZoaColors.leafSoft),
            ),
          ],
        ],
      ),
    );
  }
}

/// Where the submission sits in the lifecycle, as a vertical track.
class _Timeline extends StatelessWidget {
  const _Timeline({required this.submission});

  final Submission submission;

  @override
  Widget build(BuildContext context) {
    if (submission.isRejected) {
      return ZoaCard(
        child: Row(
          children: [
            const Icon(Icons.block_outlined, size: 18, color: ZoaColors.statusError),
            const SizedBox(width: ZoaSpace.sm),
            Expanded(
              child: Text(
                'This submission was not accepted, so no points were credited.',
                style: ZoaType.bodySm,
              ),
            ),
          ],
        ),
      );
    }

    final steps = [
      ('Logged', true),
      ('Picked up', submission.isCollected || submission.isVerified),
      ('Verified & credited', submission.isVerified),
    ];

    return ZoaCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const ZoaKicker('Progress'),
          const SizedBox(height: ZoaSpace.md),
          for (var i = 0; i < steps.length; i++) ...[
            Row(
              children: [
                Icon(
                  // Paired with a label, so completion is never conveyed by
                  // colour alone (UI/UX doc §5).
                  steps[i].$2 ? Icons.check_circle : Icons.radio_button_unchecked,
                  size: 18,
                  color: steps[i].$2 ? ZoaColors.leaf : ZoaColors.line,
                ),
                const SizedBox(width: ZoaSpace.sm),
                Text(
                  steps[i].$1,
                  style: steps[i].$2
                      ? ZoaType.label
                      : ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
                ),
              ],
            ),
            if (i < steps.length - 1)
              Padding(
                padding: const EdgeInsets.only(left: 8),
                child: Container(width: 2, height: 14, color: ZoaColors.line),
              ),
          ],
        ],
      ),
    );
  }
}

/// A status label with an icon. Public so the collector queue and activity list
/// render status identically.
class StatusPill extends StatelessWidget {
  const StatusPill({super.key, required this.status});

  final String status;

  @override
  Widget build(BuildContext context) {
    final (label, color, icon) = switch (status) {
      SubmissionStatus.pending => ('Pending', ZoaColors.statusPending, Icons.schedule),
      SubmissionStatus.collected => ('Collected', ZoaColors.leaf, Icons.local_shipping_outlined),
      SubmissionStatus.verified => ('Verified', ZoaColors.statusSuccess, Icons.check_circle_outline),
      SubmissionStatus.rejected => ('Not accepted', ZoaColors.statusError, Icons.block_outlined),
      _ => (status, ZoaColors.inkSoft, Icons.help_outline),
    };

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: color.withOpacity(0.12),
        borderRadius: ZoaRadius.allPill,
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 13, color: color),
          const SizedBox(width: 5),
          Text(label, style: ZoaType.tag.copyWith(color: color)),
        ],
      ),
    );
  }
}

class _Row extends StatelessWidget {
  const _Row({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 7),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(width: 118, child: Text(label, style: ZoaType.bodySm)),
          Expanded(
            child: Text(
              value,
              style: ZoaType.mono.copyWith(color: ZoaColors.ink),
            ),
          ),
        ],
      ),
    );
  }
}
