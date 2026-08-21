/// Profile tab — account details, role, and sign out.
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../state/auth_controller.dart';
import '../state/redemptions_controller.dart';
import '../state/submissions_controller.dart';
import '../state/vouchers_controller.dart';
import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';
import '../util/format.dart';
import '../widgets/fade_slide_in.dart';
import '../widgets/zoa_ui.dart';
import 'admin/admin_overview_screen.dart';
import 'status_screen.dart';

class ProfileScreen extends StatelessWidget {
  const ProfileScreen({super.key});

  Future<void> _confirmSignOut(BuildContext context) async {
    final auth = context.read<AuthController>();
    // Captured before the dialog, so the submissions of the user signing out are
    // cleared rather than lingering for whoever signs in next.
    final submissions = context.read<SubmissionsController>();
    // Same for the voucher catalogue: it caches a points balance and the
    // affordability flags computed from it, which belong to this user only.
    final vouchers = context.read<VouchersController>();
    // And the codes most of all — a redemption code is bearer-like, so the next
    // person on a shared handset must not find the previous user's still on screen.
    final redemptions = context.read<RedemptionsController>();

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        backgroundColor: ZoaColors.paperCard,
        surfaceTintColor: Colors.transparent,
        shape: const RoundedRectangleBorder(borderRadius: ZoaRadius.allMd),
        title: Text('Sign out?', style: ZoaType.h3),
        content: Text(
          'Your points stay on your account. You will need your phone number '
          'and password to sign back in.',
          style: ZoaType.bodySm,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(false),
            child: Text(
              'Cancel',
              style: ZoaType.button.copyWith(color: ZoaColors.inkSoft),
            ),
          ),
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(true),
            child: Text(
              'Sign out',
              style: ZoaType.button.copyWith(color: ZoaColors.statusError),
            ),
          ),
        ],
      ),
    );

    if (confirmed ?? false) {
      submissions.reset();
      vouchers.reset();
      redemptions.reset();
      // The root listener swaps in the login screen once state flips.
      await auth.signOut();
    }
  }

  @override
  Widget build(BuildContext context) {
    final user = context.watch<AuthController>().user;
    if (user == null) return const SizedBox.shrink();

    return ZoaPage(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: FadeSlideIn.staggered([
          const SizedBox(height: ZoaSpace.sm),
          Row(
            children: [
              _Avatar(initials: user.initials),
              const SizedBox(width: ZoaSpace.md),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(user.name, style: ZoaType.h3),
                    const SizedBox(height: 4),
                    Text(user.phoneNumber, style: ZoaType.mono),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: ZoaSpace.lg),
          Align(
            alignment: Alignment.centerLeft,
            // Role is shown for everyone, not just staff: it tells a collector
            // why they can see the verification queue, and a recycler that they
            // cannot.
            child: ZoaTag(user.roleLabel),
          ),
          const SizedBox(height: ZoaSpace.xl),
          ZoaCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const ZoaKicker('Account'),
                const SizedBox(height: ZoaSpace.md),
                _DetailRow(
                  label: 'Points balance',
                  value: '${formatPoints(user.pointsBalance)} pts',
                  emphasise: true,
                ),
                _DetailRow(label: 'Member since', value: formatDate(user.createdAt)),
                _DetailRow(label: 'Account ID', value: '#${user.id}'),
              ],
            ),
          ),
          const SizedBox(height: ZoaSpace.lg),
          ZoaCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const ZoaKicker('Points history'),
                const SizedBox(height: ZoaSpace.sm),
                Text(
                  'A full record of every point earned and spent arrives with '
                  'the submission flow.',
                  style: ZoaType.bodySm,
                ),
              ],
            ),
          ),
          const SizedBox(height: ZoaSpace.lg),
          // Admin only, and reached from here rather than as a seventh bottom-nav
          // destination: the Implementation Plan calls this a "minor" overview, and
          // an admin already carries six tabs.
          if (user.isAdmin) ...[
            ZoaCard(
              onTap: () => Navigator.of(context).push(
                MaterialPageRoute<void>(
                  builder: (_) => const AdminOverviewScreen(),
                ),
              ),
              child: Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text('Platform statistics', style: ZoaType.label),
                        const SizedBox(height: 2),
                        Text(
                          'Users, weight collected, points outstanding and AI '
                          'accuracy.',
                          style: ZoaType.bodySm,
                        ),
                      ],
                    ),
                  ),
                  const Icon(
                    Icons.chevron_right,
                    size: 20,
                    color: ZoaColors.inkSoft,
                  ),
                ],
              ),
            ),
            const SizedBox(height: ZoaSpace.lg),
          ],
          ZoaCard(
            onTap: () => Navigator.of(context).push(
              MaterialPageRoute<void>(
                builder: (_) => const StatusScreenPage(),
              ),
            ),
            child: Row(
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text('System status', style: ZoaType.label),
                      const SizedBox(height: 2),
                      Text(
                        'Backend, database and material rates.',
                        style: ZoaType.bodySm,
                      ),
                    ],
                  ),
                ),
                const Icon(
                  Icons.chevron_right,
                  size: 20,
                  color: ZoaColors.inkSoft,
                ),
              ],
            ),
          ),
          const SizedBox(height: ZoaSpace.xxl),
          ZoaGhostButton(
            label: 'Sign out',
            onPressed: () => _confirmSignOut(context),
          ),
          const SizedBox(height: ZoaSpace.xl),
        ]),
      ),
    );
  }
}

/// Initials disc, in place of a photo — there is no avatar upload in scope.
class _Avatar extends StatelessWidget {
  const _Avatar({required this.initials});

  final String initials;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 54,
      height: 54,
      alignment: Alignment.center,
      decoration: const BoxDecoration(
        color: ZoaColors.leafWash,
        shape: BoxShape.circle,
      ),
      child: Text(
        initials,
        style: ZoaType.cardTitle.copyWith(color: ZoaColors.forestDeep),
      ),
    );
  }
}

class _DetailRow extends StatelessWidget {
  const _DetailRow({
    required this.label,
    required this.value,
    this.emphasise = false,
  });

  final String label;
  final String value;

  /// Renders the value in gold mono — used for anything points-related.
  final bool emphasise;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Row(
        children: [
          Expanded(child: Text(label, style: ZoaType.bodySm)),
          Text(
            value,
            style: emphasise
                ? ZoaType.pointsCost.copyWith(fontSize: 15)
                : ZoaType.mono.copyWith(color: ZoaColors.ink),
          ),
        ],
      ),
    );
  }
}
