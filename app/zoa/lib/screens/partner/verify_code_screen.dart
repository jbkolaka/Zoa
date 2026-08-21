/// Partner code verification — the till end of the loop (Phase 4).
///
/// Manual code entry by design: there is no retailer POS integration in this build
/// window, so a cashier types or pastes the code and the server decides. That is
/// stated plainly on the screen rather than dressed up as a scanner, because the
/// honest version is what a partner would actually be trained on.
///
/// The result panel is deliberately large and unambiguous. The person reading it
/// is mid-transaction with a customer waiting, and the only question that matters
/// is whether to apply the discount — so both outcomes answer exactly that, in
/// words, with an icon, not by colour alone (UI/UX §5).
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../api/api_exception.dart';
import '../../api/api_models.dart';
import '../../state/redemptions_controller.dart';
import '../../theme/zoa_colors.dart';
import '../../theme/zoa_theme.dart';
import '../../widgets/fade_slide_in.dart';
import '../../widgets/zoa_text_field.dart';
import '../../widgets/zoa_ui.dart';

class VerifyCodeScreen extends StatefulWidget {
  const VerifyCodeScreen({super.key});

  @override
  State<VerifyCodeScreen> createState() => _VerifyCodeScreenState();
}

class _VerifyCodeScreenState extends State<VerifyCodeScreen> {
  final TextEditingController _code = TextEditingController();

  /// The last accepted code, held so the panel can stay on screen after the field
  /// is cleared for the next customer.
  RedemptionVerification? _accepted;

  /// The refusal to show — already-used, expired, or an unknown code.
  ApiException? _refused;

  @override
  void initState() {
    super.initState();
    _code.addListener(_onCodeChanged);
  }

  void _onCodeChanged() => setState(() {});

  @override
  void dispose() {
    _code.removeListener(_onCodeChanged);
    _code.dispose();
    super.dispose();
  }

  /// Whether there is enough in the field to be worth sending.
  ///
  /// Only emptiness is checked, not the UUID shape: the server is the authority on
  /// what is a real code, and a client-side format rule would reject a valid code
  /// if the format ever changed.
  bool get _canSubmit => normaliseRedemptionCode(_code.text).isNotEmpty;

  Future<void> _verify() async {
    if (!_canSubmit) return;

    final controller = context.read<RedemptionsController>();
    final result = await controller.verify(_code.text);

    if (!mounted) return;
    setState(() {
      _accepted = result;
      // A 409 is the interesting answer here, not an error to bury: it means the
      // code was already spent or has expired, and the cashier must be told.
      _refused = result == null ? controller.error : null;
      if (result != null) _code.clear();
    });
  }

  void _reset() {
    context.read<RedemptionsController>().clearError();
    setState(() {
      _accepted = null;
      _refused = null;
      _code.clear();
    });
  }

  @override
  Widget build(BuildContext context) {
    final submitting = context.watch<RedemptionsController>().submitting;
    final answered = _accepted != null || _refused != null;

    return ZoaPage(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: FadeSlideIn.staggered([
          const SizedBox(height: ZoaSpace.sm),
          const ZoaEyebrow('Partner till'),
          const SizedBox(height: ZoaSpace.md),
          Text('Verify a\ncustomer code', style: ZoaType.h2),
          const SizedBox(height: ZoaSpace.md),
          Text(
            'Type or paste the code from the customer\'s phone. Zoa checks it and '
            'tells you whether to apply the discount.',
            style: ZoaType.bodySoft,
          ),
          const SizedBox(height: ZoaSpace.xl),
          ZoaTextField(
            label: 'Redemption code',
            controller: _code,
            hint: '7f3c1a92-4b0e-4c7d-9e21-8a5f0b6d1c34',
            helper: 'Pasting the scanned QR link works too.',
            // Codes read as data, and mono makes a mistyped character findable.
            mono: true,
            enabled: !submitting,
            textInputAction: TextInputAction.go,
            onSubmitted: (_) => _verify(),
          ),
          const SizedBox(height: ZoaSpace.md),
          ZoaPrimaryButton(
            label: 'Check code',
            icon: Icons.qr_code_scanner,
            loading: submitting,
            onPressed: _canSubmit ? _verify : null,
          ),
          if (answered) ...[
            const SizedBox(height: ZoaSpace.lg),
            if (_accepted != null)
              _AcceptedPanel(verification: _accepted!)
            else
              _RefusedPanel(error: _refused!),
            const SizedBox(height: ZoaSpace.md),
            ZoaGhostButton(label: 'Check another code', onPressed: _reset),
          ],
          const SizedBox(height: ZoaSpace.lg),
          const _HonestNote(),
          const SizedBox(height: ZoaSpace.xl),
        ]),
      ),
    );
  }
}

/// The accept case: what the discount is, and whose it was.
class _AcceptedPanel extends StatelessWidget {
  const _AcceptedPanel({required this.verification});

  final RedemptionVerification verification;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(ZoaSpace.lg),
      decoration: BoxDecoration(
        color: ZoaColors.leafWash,
        border: Border.all(color: ZoaColors.leaf, width: 1.6),
        borderRadius: ZoaRadius.allMd,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(Icons.check_circle, size: 24, color: ZoaColors.statusSuccess),
              const SizedBox(width: ZoaSpace.sm),
              Expanded(
                child: Text(
                  'Accepted',
                  style: ZoaType.h3.copyWith(color: ZoaColors.forestDeep),
                ),
              ),
            ],
          ),
          const SizedBox(height: ZoaSpace.md),
          // The server's own wording, so the app and the API never describe the
          // same discount two different ways.
          Text(verification.message, style: ZoaType.body),
          const SizedBox(height: ZoaSpace.md),
          const Divider(height: 1, color: ZoaColors.leaf),
          const SizedBox(height: ZoaSpace.md),
          _row('Reward', verification.voucher.title),
          const SizedBox(height: ZoaSpace.sm),
          _row('Partner', verification.voucher.partner.name),
          const SizedBox(height: ZoaSpace.sm),
          _row('Customer', verification.userName),
          const SizedBox(height: ZoaSpace.md),
          Text(
            'This code is now used and cannot be accepted again.',
            style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
          ),
        ],
      ),
    );
  }

  Widget _row(String label, String value) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          width: 78,
          child: Text(label, style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft)),
        ),
        Expanded(child: Text(value, style: ZoaType.bodySm.copyWith(color: ZoaColors.ink))),
      ],
    );
  }
}

/// The refuse case. Leads with the instruction, not the diagnosis — a cashier
/// needs to know not to give the discount before they need to know why.
class _RefusedPanel extends StatelessWidget {
  const _RefusedPanel({required this.error});

  final ApiException error;

  @override
  Widget build(BuildContext context) {
    // A 409 means the code is real but spent or expired; a 404 means it is not a
    // Zoa code at all. Both refuse, and the headline says which.
    final headline = error.code == ApiErrorCode.notFound
        ? 'Not a valid code'
        : error.code == ApiErrorCode.conflict
            ? 'Do not accept'
            : 'Could not check the code';

    // Only a genuine transport or server failure is retryable — a used code will
    // never become valid, and offering "try again" there would be misleading.
    final retryable = error.isRetryable;

    return Container(
      padding: const EdgeInsets.all(ZoaSpace.lg),
      decoration: BoxDecoration(
        color: ZoaColors.statusError.withValues(alpha: 0.08),
        border: Border.all(color: ZoaColors.statusError, width: 1.6),
        borderRadius: ZoaRadius.allMd,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                retryable ? Icons.wifi_off : Icons.cancel,
                size: 24,
                color: ZoaColors.statusError,
              ),
              const SizedBox(width: ZoaSpace.sm),
              Expanded(
                child: Text(
                  headline,
                  style: ZoaType.h3.copyWith(color: ZoaColors.statusError),
                ),
              ),
            ],
          ),
          const SizedBox(height: ZoaSpace.md),
          Text(error.message, style: ZoaType.body),
          if (retryable) ...[
            const SizedBox(height: ZoaSpace.sm),
            Text(
              'The code has not been used — check the connection and try again.',
              style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
            ),
          ],
        ],
      ),
    );
  }
}

/// States the design limit rather than hiding it. `07_Implementation_Plan.md`
/// § Honest Framing asks for this to be said out loud to judges; saying it in the
/// product itself is the same argument.
class _HonestNote extends StatelessWidget {
  const _HonestNote();

  @override
  Widget build(BuildContext context) {
    return ZoaCard(
      padding: const EdgeInsets.all(ZoaSpace.lg),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const ZoaKicker('How this works'),
          const SizedBox(height: ZoaSpace.sm),
          Text(
            'Verification is manual code entry — there is no till-system '
            'integration yet. The check itself is server-side and one-shot, so a '
            'code cannot be accepted twice even if two tills try at once.',
            style: ZoaType.bodySm,
          ),
        ],
      ),
    );
  }
}
