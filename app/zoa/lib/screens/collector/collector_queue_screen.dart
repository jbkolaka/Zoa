/// Collector queue — list open submissions and confirm them.
///
/// The Implementation Plan allows this to be a minimal screen in the same app
/// gated by role, rather than a separate tool, and that is what this is: the
/// verification half of the loop, reachable only by a collector or admin.
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../api/api_models.dart';
import '../../state/app_status.dart';
import '../../state/submissions_controller.dart';
import '../../theme/zoa_colors.dart';
import '../../theme/zoa_theme.dart';
import '../../util/format.dart';
import '../../widgets/fade_slide_in.dart';
import '../../widgets/loop_seal.dart';
import '../../widgets/material_icons.dart';
import '../../widgets/zoa_empty_state.dart';
import '../../widgets/zoa_ui.dart';
import '../submission_status_screen.dart';
import 'verify_sheet.dart';

class CollectorQueueScreen extends StatefulWidget {
  const CollectorQueueScreen({super.key});

  @override
  State<CollectorQueueScreen> createState() => _CollectorQueueScreenState();
}

class _CollectorQueueScreenState extends State<CollectorQueueScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) context.read<SubmissionsController>().loadQueue();
    });
  }

  Future<void> _openVerifySheet(Submission submission) async {
    final result = await showVerifySheet(context, submission);
    if (!mounted || result == null) return;

    final points = result.pointsAwarded;
    final message = points > 0
        ? 'Verified — ${formatPoints(points)} points credited to '
            '${submission.userName}'
        : 'Submission marked ${result.submission.status}';

    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(message, style: ZoaType.bodySm.copyWith(color: ZoaColors.paper)),
        backgroundColor: ZoaColors.forestDeep,
        behavior: SnackBarBehavior.floating,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final submissions = context.watch<SubmissionsController>();
    final queue = submissions.queue;

    if (submissions.loading && queue.isEmpty) {
      return const Center(child: LoopSeal.loading(size: 190, subLabel: 'loading queue…'));
    }

    if (submissions.error != null && queue.isEmpty) {
      return ZoaErrorState(
        title: 'Could not load the queue',
        message: submissions.error!.message,
        onRetry: () => context.read<SubmissionsController>().loadQueue(),
      );
    }

    if (queue.isEmpty) {
      return ZoaEmptyState(
        title: 'Nothing waiting',
        blurb: 'Every submission has been handled. New ones appear here as '
            'recyclers log them.',
        stage: LoopStage.verify,
        action: ZoaGhostButton(
          label: 'Refresh',
          expand: false,
          onPressed: () => context.read<SubmissionsController>().loadQueue(),
        ),
      );
    }

    return RefreshIndicator(
      onRefresh: () => context.read<SubmissionsController>().loadQueue(),
      color: ZoaColors.forestDeep,
      backgroundColor: ZoaColors.paperCard,
      child: ZoaPage(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: FadeSlideIn.staggered([
            const SizedBox(height: ZoaSpace.sm),
            const ZoaEyebrow('Collector'),
            const SizedBox(height: ZoaSpace.md),
            Text('Awaiting verification', style: ZoaType.h2),
            const SizedBox(height: ZoaSpace.sm),
            Text(
              // Oldest first: whoever has waited longest gets seen first.
              '${queue.length} ${queue.length == 1 ? 'submission' : 'submissions'} '
              'to confirm, oldest first. Points are credited from the weight you '
              'enter.',
              style: ZoaType.bodySoft,
            ),
            const SizedBox(height: ZoaSpace.xl),
            for (final submission in queue) ...[
              _QueueCard(
                submission: submission,
                onVerify: () => _openVerifySheet(submission),
              ),
              const SizedBox(height: ZoaSpace.md),
            ],
            const SizedBox(height: ZoaSpace.lg),
          ]),
        ),
      ),
    );
  }
}

class _QueueCard extends StatelessWidget {
  const _QueueCard({required this.submission, required this.onVerify});

  final Submission submission;
  final VoidCallback onVerify;

  @override
  Widget build(BuildContext context) {
    final meta = context.watch<AppStatus>().meta;
    final label = meta?.labelFor(submission.materialType) ?? submission.materialType;
    final rate = meta?.rateFor(submission.materialType);

    final estimate = submission.estimatedQtyKg;
    final projected = (estimate != null && rate != null)
        ? (estimate * rate).round()
        : null;

    return ZoaCard(
      padding: const EdgeInsets.all(ZoaSpace.lg),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              MaterialIcon.forKey(
                submission.materialType,
                group: meta?.materialFor(submission.materialType)?.group,
                size: 34,
              ),
              const SizedBox(width: ZoaSpace.md),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(label, style: ZoaType.cardTitle),
                    const SizedBox(height: 2),
                    Text(
                      // The submitter's name, because a collector meets a person
                      // rather than a record id.
                      submission.userName.isEmpty
                          ? 'Submission #${submission.id}'
                          : submission.userName,
                      style: ZoaType.bodySm,
                    ),
                  ],
                ),
              ),
              StatusPill(status: submission.status),
            ],
          ),
          const SizedBox(height: ZoaSpace.md),
          Row(
            children: [
              Expanded(
                child: _Fact(
                  label: 'Estimated',
                  value: estimate == null ? '—' : '$estimate kg',
                ),
              ),
              Expanded(
                child: _Fact(
                  label: 'If confirmed',
                  value: projected == null ? '—' : '≈ $projected pts',
                  gold: true,
                ),
              ),
            ],
          ),
          if (submission.location != null) ...[
            const SizedBox(height: ZoaSpace.sm),
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Icon(Icons.place_outlined, size: 15, color: ZoaColors.inkSoft),
                const SizedBox(width: 5),
                Expanded(
                  child: Text(submission.location!, style: ZoaType.bodySm),
                ),
              ],
            ),
          ],
          const SizedBox(height: ZoaSpace.sm),
          Text(
            'Logged ${formatDate(submission.createdAt)}',
            style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft),
          ),
          const SizedBox(height: ZoaSpace.md),
          ZoaPrimaryButton(
            label: 'Confirm collection',
            onPressed: onVerify,
          ),
        ],
      ),
    );
  }
}

class _Fact extends StatelessWidget {
  const _Fact({required this.label, required this.value, this.gold = false});

  final String label;
  final String value;
  final bool gold;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft)),
        const SizedBox(height: 2),
        Text(
          value,
          style: gold
              ? ZoaType.pointsCost.copyWith(fontSize: 15)
              : ZoaType.mono.copyWith(color: ZoaColors.ink, fontSize: 15),
        ),
      ],
    );
  }
}
