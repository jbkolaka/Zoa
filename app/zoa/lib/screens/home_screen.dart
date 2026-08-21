/// Home tab — the points balance.
///
/// UI/UX doc §1.2 asks for the balance to be prominent and for points to feel
/// earned rather than abstract, so it sits on a deep forest card with the number
/// in savanna gold: the one place in the app where gold is the loudest element.
library;

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
import '../widgets/zoa_ui.dart';
import 'submission_status_screen.dart';

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key, this.onLogRecycling});

  /// Switches the shell to the Recycle tab. Supplied by [HomeShell] so this
  /// screen does not need to know how navigation is structured.
  final VoidCallback? onLogRecycling;

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) context.read<SubmissionsController>().loadMine();
    });
  }

  /// Pull-to-refresh reloads both halves: the balance and the activity list.
  Future<void> _refresh() async {
    await Future.wait([
      context.read<AuthController>().refresh(),
      context.read<SubmissionsController>().loadMine(),
    ]);
  }

  @override
  Widget build(BuildContext context) {
    final auth = context.watch<AuthController>();
    final user = auth.user;

    if (user == null) {
      // Signed out mid-session; the root listener is already swapping in the
      // login screen, so this is only ever a single frame.
      return const SizedBox.shrink();
    }

    return RefreshIndicator(
      onRefresh: _refresh,
      color: ZoaColors.forestDeep,
      backgroundColor: ZoaColors.paperCard,
      child: ZoaPage(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: FadeSlideIn.staggered([
            const SizedBox(height: ZoaSpace.sm),
            Text(
              'Hello, ${user.name.split(' ').first}',
              style: ZoaType.h2,
            ),
            const SizedBox(height: ZoaSpace.xs),
            Text(
              'Every kilogram you hand over is weighed, verified and counted.',
              style: ZoaType.bodySoft,
            ),
            const SizedBox(height: ZoaSpace.xl),
            _BalanceCard(points: user.pointsBalance),
            const SizedBox(height: ZoaSpace.lg),
            // Omitted rather than disabled for an account with nothing to log —
            // a collector. A dimmed button would advertise an action that will
            // never become available to them.
            if (widget.onLogRecycling != null)
              ZoaPrimaryButton(
                label: 'Log recycling',
                icon: Icons.arrow_forward,
                onPressed: widget.onLogRecycling,
              ),
            if (widget.onLogRecycling != null)
              const SizedBox(height: ZoaSpace.xxl),
            if (widget.onLogRecycling == null)
              const SizedBox(height: ZoaSpace.md),
            const _RecentActivity(),
            const SizedBox(height: ZoaSpace.xl),
          ]),
        ),
      ),
    );
  }
}

/// The balance panel. Deep forest ground, gold number.
class _BalanceCard extends StatelessWidget {
  const _BalanceCard({required this.points});

  final int points;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(ZoaSpace.xl),
      decoration: BoxDecoration(
        color: ZoaColors.forestDeep,
        borderRadius: ZoaRadius.allLg,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Expanded(child: ZoaKicker('Your balance', onForest: true)),
              // A small, still seal — brand presence without competing with the
              // number for attention.
              const LoopSeal(
                size: 46,
                centerLabel: '',
                showStageLabels: false,
              ),
            ],
          ),
          const SizedBox(height: ZoaSpace.md),
          Row(
            crossAxisAlignment: CrossAxisAlignment.baseline,
            textBaseline: TextBaseline.alphabetic,
            children: [
              Text(
                formatPoints(points),
                style: ZoaType.pointsHero.copyWith(color: ZoaColors.gold),
              ),
              const SizedBox(width: ZoaSpace.sm),
              Text(
                points == 1 ? 'point' : 'points',
                style: ZoaType.mono.copyWith(color: ZoaColors.leafSoft),
              ),
            ],
          ),
          const SizedBox(height: ZoaSpace.md),
          const Divider(color: ZoaColors.onForestLine, height: 1),
          const SizedBox(height: ZoaSpace.md),
          Text(
            points == 0
                // A zero balance is the expected state for a new account, so it
                // explains the loop rather than reading as an error.
                ? 'Log your first submission to start earning. Points arrive once '
                    'a collector confirms the weight.'
                : 'Points are credited after a collector verifies each '
                    'submission, and can be spent on partner vouchers.',
            style: ZoaType.bodySm.copyWith(color: ZoaColors.leafSoft),
          ),
        ],
      ),
    );
  }
}

/// Recent activity — the user's own submissions, newest first.
class _RecentActivity extends StatelessWidget {
  const _RecentActivity();

  /// How many to show inline. Enough to see the loop working without turning the
  /// home screen into a full history page.
  static const _maxItems = 4;

  @override
  Widget build(BuildContext context) {
    final submissions = context.watch<SubmissionsController>();
    final items = submissions.mine.take(_maxItems).toList();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            const ZoaKicker('Recent activity'),
            const Spacer(),
            if (submissions.mine.length > _maxItems)
              Text(
                '${submissions.mine.length} total',
                style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft),
              ),
          ],
        ),
        const SizedBox(height: ZoaSpace.md),
        if (submissions.loading && items.isEmpty)
          ZoaCard(
            child: Text('Loading your submissions…', style: ZoaType.bodySm),
          )
        else if (items.isEmpty)
          ZoaCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    const Icon(
                      Icons.inbox_outlined,
                      size: 18,
                      color: ZoaColors.inkSoft,
                    ),
                    const SizedBox(width: ZoaSpace.sm),
                    Text('Nothing yet', style: ZoaType.label),
                  ],
                ),
                const SizedBox(height: ZoaSpace.sm),
                Text(
                  'Your submissions and the points they earn will appear here.',
                  style: ZoaType.bodySm,
                ),
              ],
            ),
          )
        else
          for (final submission in items) ...[
            _ActivityRow(submission: submission),
            const SizedBox(height: ZoaSpace.sm),
          ],
      ],
    );
  }
}

/// One submission, tappable through to its status screen.
class _ActivityRow extends StatelessWidget {
  const _ActivityRow({required this.submission});

  final Submission submission;

  @override
  Widget build(BuildContext context) {
    final meta = context.watch<AppStatus>().meta;
    final label = meta?.labelFor(submission.materialType) ?? submission.materialType;
    final weight = submission.displayQtyKg;

    return ZoaCard(
      padding: const EdgeInsets.all(ZoaSpace.md),
      onTap: () => Navigator.of(context).push(
        MaterialPageRoute<void>(
          builder: (_) => SubmissionStatusScreen(submissionId: submission.id),
        ),
      ),
      child: Row(
        children: [
          MaterialIcon.forKey(
            submission.materialType,
            group: meta?.materialFor(submission.materialType)?.group,
            size: 30,
          ),
          const SizedBox(width: ZoaSpace.md),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(label, style: ZoaType.label),
                const SizedBox(height: 3),
                Row(
                  children: [
                    StatusPill(status: submission.status),
                    if (weight != null) ...[
                      const SizedBox(width: ZoaSpace.sm),
                      Text(
                        '$weight kg',
                        style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft),
                      ),
                    ],
                  ],
                ),
              ],
            ),
          ),
          if (submission.pointsAwarded != null)
            // Gold, because it is about points.
            Text(
              '+${formatPoints(submission.pointsAwarded!)}',
              style: ZoaType.pointsCost.copyWith(fontSize: 15),
            )
          else
            const Icon(Icons.chevron_right, size: 18, color: ZoaColors.inkSoft),
        ],
      ),
    );
  }
}
