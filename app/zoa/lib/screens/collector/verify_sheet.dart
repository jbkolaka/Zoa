/// The collector's verification sheet: enter the measured weight, optionally
/// correct the material, then confirm.
///
/// A bottom sheet rather than a route, so the queue stays visible behind it and
/// the collector can see they are acting on the right submission.
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../api/api_models.dart';
import '../../state/app_status.dart';
import '../../state/submissions_controller.dart';
import '../../theme/zoa_colors.dart';
import '../../theme/zoa_theme.dart';
import '../../widgets/material_icons.dart';
import '../../widgets/zoa_text_field.dart';
import '../../widgets/zoa_ui.dart';

/// Shows the verification sheet. Returns the result on success, null if
/// dismissed or failed.
Future<VerifyResult?> showVerifySheet(BuildContext context, Submission submission) {
  return showModalBottomSheet<VerifyResult>(
    context: context,
    isScrollControlled: true, // so the sheet rises above the keyboard
    backgroundColor: ZoaColors.paper,
    shape: const RoundedRectangleBorder(
      borderRadius: BorderRadius.vertical(top: Radius.circular(ZoaRadius.lg)),
    ),
    builder: (_) => _VerifySheet(submission: submission),
  );
}

class _VerifySheet extends StatefulWidget {
  const _VerifySheet({required this.submission});

  final Submission submission;

  @override
  State<_VerifySheet> createState() => _VerifySheetState();
}

class _VerifySheetState extends State<_VerifySheet> {
  late final TextEditingController _weight = TextEditingController(
    // Pre-filled with the user's estimate: usually close, and it saves typing.
    // The collector still has to look at it, which is the point of the step.
    text: widget.submission.estimatedQtyKg?.toString() ?? '',
  );

  late String _materialKey = widget.submission.materialType;
  String? _weightError;
  bool _correcting = false;

  @override
  void initState() {
    super.initState();
    _weight.addListener(_onWeightChanged);
  }

  void _onWeightChanged() => setState(() {});

  @override
  void dispose() {
    _weight.removeListener(_onWeightChanged);
    _weight.dispose();
    super.dispose();
  }

  double? get _parsedWeight =>
      double.tryParse(_weight.text.trim().replaceAll(',', '.'));

  bool _validate() {
    final weight = _parsedWeight;
    setState(() {
      if (weight == null) {
        _weightError = 'Enter the weight you measured';
      } else if (weight <= 0) {
        _weightError = 'Weight must be more than 0';
      } else if (weight > 5000) {
        _weightError = 'That looks too large — check the decimal point';
      } else {
        _weightError = null;
      }
    });
    return _weightError == null;
  }

  Future<void> _confirm() async {
    if (!_validate()) return;

    final controller = context.read<SubmissionsController>();
    final result = await controller.verify(
      widget.submission.id,
      verifiedQtyKg: _parsedWeight,
      // Only sent when actually changed, so an untouched sheet does not look
      // like a correction in the stored record.
      materialType: _materialKey == widget.submission.materialType ? null : _materialKey,
    );

    if (!mounted) return;

    if (result == null) {
      setState(() => _weightError = controller.error?.fields['verified_qty_kg']);
      return;
    }
    Navigator.of(context).pop(result);
  }

  Future<void> _markCollected() async {
    final controller = context.read<SubmissionsController>();
    final result = await controller.verify(
      widget.submission.id,
      status: SubmissionStatus.collected,
    );
    if (!mounted) return;
    if (result != null) Navigator.of(context).pop(result);
  }

  Future<void> _reject() async {
    final controller = context.read<SubmissionsController>();
    final result = await controller.verify(
      widget.submission.id,
      status: SubmissionStatus.rejected,
    );
    if (!mounted) return;
    if (result != null) Navigator.of(context).pop(result);
  }

  @override
  Widget build(BuildContext context) {
    final controller = context.watch<SubmissionsController>();
    final meta = context.watch<AppStatus>().meta;
    final error = controller.error;
    final showBanner = error != null && error.fields.isEmpty;

    final rate = meta?.rateFor(_materialKey);
    final weight = _parsedWeight;
    final points = (weight != null && weight > 0 && rate != null)
        ? (weight * rate).round()
        : null;

    return Padding(
      // Lifts the sheet clear of the keyboard.
      padding: EdgeInsets.only(bottom: MediaQuery.of(context).viewInsets.bottom),
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(ZoaSpace.xl),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            Center(
              child: Container(
                width: 42,
                height: 4,
                decoration: BoxDecoration(
                  color: ZoaColors.line,
                  borderRadius: ZoaRadius.allPill,
                ),
              ),
            ),
            const SizedBox(height: ZoaSpace.lg),
            const ZoaKicker('Confirm collection'),
            const SizedBox(height: ZoaSpace.sm),
            Text(
              widget.submission.userName.isEmpty
                  ? 'Submission #${widget.submission.id}'
                  : widget.submission.userName,
              style: ZoaType.h3,
            ),
            const SizedBox(height: ZoaSpace.xl),
            if (showBanner) ...[
              ZoaErrorBanner(message: error.message),
              const SizedBox(height: ZoaSpace.lg),
            ],
            _MaterialRow(
              materialKey: _materialKey,
              submittedKey: widget.submission.materialType,
              label: meta?.labelFor(_materialKey) ?? _materialKey,
              onChange: () => setState(() => _correcting = !_correcting),
              correcting: _correcting,
            ),
            if (_correcting) ...[
              const SizedBox(height: ZoaSpace.md),
              _MaterialCorrectionList(
                materials: meta?.materials ?? const [],
                selected: _materialKey,
                onSelect: (key) => setState(() {
                  _materialKey = key;
                  _correcting = false;
                }),
              ),
            ],
            const SizedBox(height: ZoaSpace.lg),
            ZoaTextField(
              label: 'Measured weight',
              controller: _weight,
              hint: '4.2',
              helper: 'In kilograms. This is what points are calculated from.',
              errorText: _weightError,
              keyboardType: const TextInputType.numberWithOptions(decimal: true),
              textInputAction: TextInputAction.done,
              mono: true,
              enabled: !controller.submitting,
              onSubmitted: (_) => _confirm(),
            ),
            const SizedBox(height: ZoaSpace.lg),
            ZoaCard(
              accent: true,
              padding: const EdgeInsets.all(ZoaSpace.md),
              child: Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text('Will credit', style: ZoaType.tag),
                        const SizedBox(height: 2),
                        Text(
                          points == null ? 'enter a weight' : '$points points',
                          style: ZoaType.cardTitle.copyWith(
                            color: points == null
                                ? ZoaColors.inkSoft
                                : ZoaColors.forestDeep,
                          ),
                        ),
                      ],
                    ),
                  ),
                  if (rate != null) Text('$rate pts/kg', style: ZoaType.pointsCost),
                ],
              ),
            ),
            const SizedBox(height: ZoaSpace.xl),
            ZoaPrimaryButton(
              label: 'Verify & credit points',
              loading: controller.submitting,
              onPressed: controller.submitting ? null : _confirm,
            ),
            const SizedBox(height: ZoaSpace.sm),
            // Only offered while still pending — a submission already marked
            // collected cannot go back to collected.
            if (widget.submission.isPending) ...[
              ZoaGhostButton(
                label: 'Picked up, weighing later',
                onPressed: controller.submitting ? null : _markCollected,
              ),
              const SizedBox(height: ZoaSpace.sm),
            ],
            TextButton(
              onPressed: controller.submitting ? null : _reject,
              child: Text(
                'Cannot accept this submission',
                style: ZoaType.button.copyWith(color: ZoaColors.statusError),
              ),
            ),
            const SizedBox(height: ZoaSpace.sm),
          ],
        ),
      ),
    );
  }
}

/// Shows the material, flagging it when the collector has overridden what the
/// user submitted.
class _MaterialRow extends StatelessWidget {
  const _MaterialRow({
    required this.materialKey,
    required this.submittedKey,
    required this.label,
    required this.onChange,
    required this.correcting,
  });

  final String materialKey;
  final String submittedKey;
  final String label;
  final VoidCallback onChange;
  final bool correcting;

  @override
  Widget build(BuildContext context) {
    final corrected = materialKey != submittedKey;

    return ZoaCard(
      padding: const EdgeInsets.all(ZoaSpace.md),
      child: Row(
        children: [
          MaterialIcon.forKey(materialKey, size: 30),
          const SizedBox(width: ZoaSpace.md),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(label, style: ZoaType.label),
                const SizedBox(height: 2),
                Text(
                  corrected ? 'Corrected by you' : 'As submitted',
                  style: ZoaType.tag.copyWith(
                    color: corrected ? ZoaColors.goldDeep : ZoaColors.inkSoft,
                  ),
                ),
              ],
            ),
          ),
          TextButton(
            onPressed: onChange,
            child: Text(
              correcting ? 'Close' : 'Change',
              style: ZoaType.button.copyWith(
                color: ZoaColors.leaf,
                fontSize: 13,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Compact list for picking a corrected material type.
class _MaterialCorrectionList extends StatelessWidget {
  const _MaterialCorrectionList({
    required this.materials,
    required this.selected,
    required this.onSelect,
  });

  final List<MaterialInfo> materials;
  final String selected;
  final ValueChanged<String> onSelect;

  @override
  Widget build(BuildContext context) {
    if (materials.isEmpty) {
      return Text(
        'The material list is unavailable, so the submitted type will be kept.',
        style: ZoaType.bodySm,
      );
    }

    return Container(
      constraints: const BoxConstraints(maxHeight: 260),
      decoration: BoxDecoration(
        color: ZoaColors.paperCard,
        border: Border.all(color: ZoaColors.line),
        borderRadius: ZoaRadius.allMd,
      ),
      child: ListView.builder(
        shrinkWrap: true,
        itemCount: materials.length,
        itemBuilder: (context, index) {
          final material = materials[index];
          final isSelected = material.key == selected;

          return InkWell(
            onTap: () => onSelect(material.key),
            child: Padding(
              padding: const EdgeInsets.symmetric(
                horizontal: ZoaSpace.md,
                vertical: ZoaSpace.sm,
              ),
              child: Row(
                children: [
                  MaterialIcon.forKey(
                    material.key,
                    group: material.group,
                    size: 24,
                  ),
                  const SizedBox(width: ZoaSpace.md),
                  Expanded(
                    child: Text(
                      material.label,
                      style: isSelected ? ZoaType.label : ZoaType.bodySm,
                    ),
                  ),
                  Text('${material.pointsPerKg}/kg', style: ZoaType.pointsCost),
                  if (isSelected) ...[
                    const SizedBox(width: ZoaSpace.sm),
                    const Icon(Icons.check, size: 16, color: ZoaColors.leaf),
                  ],
                ],
              ),
            ),
          );
        },
      ),
    );
  }
}
