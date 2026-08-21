/// New Submission — log a recycling submission.
///
/// UI/UX doc §1.1 gives this screen a budget of under 30 seconds, so it asks for
/// three things only: what the material is, roughly how much, and where. Material
/// selection is a tap on an icon grid rather than a dropdown, because a
/// fourteen-item dropdown on a phone is the slowest possible control.
///
/// Phase 2.5 adds an optional photo. It sits above the material grid and only
/// ever *pre-fills* the selection: the user confirms or overrides, and the
/// collector re-checks at verification. Nothing about the photo is required, so
/// every failure path leads back to the same manual grid (FR-23).
library;

import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';
import 'package:provider/provider.dart';

import '../api/api_models.dart';
import '../state/app_status.dart';
import '../state/auth_controller.dart';
import '../state/submissions_controller.dart';
import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';
import '../widgets/fade_slide_in.dart';
import '../widgets/material_icons.dart';
import '../widgets/zoa_text_field.dart';
import '../widgets/zoa_ui.dart';
import 'submission_status_screen.dart';

/// Display labels for the taxonomy's group keys.
const _groupLabels = {
  'plastics': 'Plastics',
  'paper': 'Paper & cardboard',
  'glass': 'Glass',
  'metal': 'Metal',
  'organic': 'Organic waste',
};

class NewSubmissionScreen extends StatefulWidget {
  const NewSubmissionScreen({super.key});

  @override
  State<NewSubmissionScreen> createState() => _NewSubmissionScreenState();
}

class _NewSubmissionScreenState extends State<NewSubmissionScreen> {
  final _weight = TextEditingController();
  final _location = TextEditingController();

  String? _materialKey;
  String? _materialError;
  String? _weightError;

  /// Waste origin, asked only for organics (FR-24). Hotel kitchen volumes differ
  /// enough from household ones to change collection routing, and the server
  /// rejects an organic submission that does not declare one.
  String? _sourceType;
  String? _sourceError;

  // --- Phase 2.5: photo classification ---

  /// The chosen photo's bytes, held in memory so the preview and the upload use
  /// the same data and the file is read exactly once.
  Uint8List? _photoBytes;
  String? _photoName;

  /// The last classification result, or null if no photo has been classified.
  /// A `degraded` result is kept rather than discarded so the UI can say the
  /// assist was tried and did not land, instead of silently showing nothing.
  Classification? _classification;

  /// What the model predicted, retained even if the user overrides the material.
  ///
  /// This is the whole point of FR-22: the pair (predicted, confirmed) is the
  /// accuracy metric, so the prediction is submitted even when — especially
  /// when — the user disagreed with it.
  String? _predictedCategory;
  double? _predictedConfidence;

  /// True once the user has accepted or rejected the prediction, which is what
  /// collapses the confirm card. A prediction never auto-confirms (TRD §5 risk
  /// table): until this flips, the material grid stays the operative control.
  bool _predictionAnswered = false;

  /// Whether the chosen material belongs to the organic group, which is what
  /// makes [_sourceType] mandatory. Read from the server's taxonomy rather than
  /// hardcoded here, so adding an organic material needs no client change.
  bool _isOrganic(MetaCatalog? meta) {
    final key = _materialKey;
    if (key == null) return false;
    return meta?.materialFor(key)?.group == 'organic';
  }

  @override
  void initState() {
    super.initState();
    // The live points estimate reads the weight field, and controller changes do
    // not rebuild this widget on their own.
    _weight.addListener(_onWeightChanged);
  }

  void _onWeightChanged() => setState(() {});

  @override
  void dispose() {
    _weight.removeListener(_onWeightChanged);
    _weight.dispose();
    _location.dispose();
    super.dispose();
  }

  // --- Phase 2.5: photo capture and classification ---

  /// Picks a photo, then classifies it.
  ///
  /// Images are requested at a reduced size and quality: the server caps uploads
  /// at 8 MB, a modern phone photo can exceed that on its own, and a smaller
  /// image also spends less time on the wire — which is the dominant cost of the
  /// whole round trip on mobile data. Downscaling does not hurt the prediction;
  /// telling a PET bottle from a cardboard box does not need 12 megapixels.
  Future<void> _pickPhoto(ImageSource source) async {
    final picker = ImagePicker();

    final XFile? file;
    try {
      file = await picker.pickImage(
        source: source,
        maxWidth: 1600,
        maxHeight: 1600,
        imageQuality: 85,
      );
    } catch (_) {
      // A denied permission or an unavailable camera lands here. The photo is
      // optional, so this is a dead end for the assist, not for the form.
      if (!mounted) return;
      setState(() {
        _classification = null;
        _photoBytes = null;
        _photoName = null;
      });
      _showPhotoUnavailable();
      return;
    }

    if (file == null || !mounted) return; // user backed out of the picker

    final bytes = await file.readAsBytes();
    if (!mounted) return;

    setState(() {
      _photoBytes = bytes;
      _photoName = file!.name;
      // A new photo invalidates the previous answer, so the confirm card comes
      // back rather than leaving a stale prediction attached to a new image.
      _classification = null;
      _predictedCategory = null;
      _predictedConfidence = null;
      _predictionAnswered = false;
    });

    await _classifyPhoto();
  }

  /// Sends the held photo for classification and applies the result.
  Future<void> _classifyPhoto() async {
    final bytes = _photoBytes;
    if (bytes == null) return;

    final result = await context.read<SubmissionsController>().classify(
          photoBytes: bytes,
          filename: _photoName ?? 'photo.jpg',
        );

    if (!mounted) return;

    setState(() {
      _classification = result;

      if (result != null && result.hasPrediction) {
        // Recorded now so it survives whatever the user does next, including
        // overriding the material entirely (FR-22).
        _predictedCategory = result.predictedCategory;
        _predictedConfidence = result.predictedConfidence;

        // Pre-fill, never auto-confirm. The grid selection moves so the user can
        // see what was proposed, but _predictionAnswered stays false until they
        // say yes or no, and the collector re-checks it regardless.
        _materialKey = result.predictedCategory;
        _materialError = null;
        if (result.group != 'organic') _sourceType = null;
        _predictionAnswered = false;
      } else {
        // Degraded: no prediction to show. The material grid below is already
        // the fallback, so nothing else needs to change.
        _predictedCategory = null;
        _predictedConfidence = null;
        _predictionAnswered = true;
      }
    });
  }

  void _removePhoto() {
    setState(() {
      _photoBytes = null;
      _photoName = null;
      _classification = null;
      _predictedCategory = null;
      _predictedConfidence = null;
      _predictionAnswered = false;
    });
  }

  /// Accepts the prediction as-is.
  void _confirmPrediction() {
    setState(() {
      _predictionAnswered = true;
      _materialError = null;
    });
  }

  /// Rejects the prediction and hands the choice back to the grid.
  ///
  /// The selection is cleared rather than left in place: leaving the rejected
  /// guess selected would mean a user who taps "no" and then submits sends the
  /// exact value they just rejected.
  void _rejectPrediction() {
    setState(() {
      _predictionAnswered = true;
      _materialKey = null;
      _sourceType = null;
      _sourceError = null;
    });
  }

  /// Applies a runner-up category in one tap.
  void _useAlternative(String key, MetaCatalog? meta) {
    setState(() {
      _materialKey = key;
      _materialError = null;
      _predictionAnswered = true;
      if (meta?.materialFor(key)?.group != 'organic') _sourceType = null;
      _sourceError = null;
    });
  }

  void _showPhotoUnavailable() {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(
        content: Text(
          'Could not open the camera. Pick the material below instead — '
          'the photo is optional.',
        ),
      ),
    );
  }

  /// Offers camera or gallery.
  Future<void> _choosePhotoSource() async {
    final source = await showModalBottomSheet<ImageSource>(
      context: context,
      backgroundColor: ZoaColors.paperCard,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(18)),
      ),
      builder: (sheetContext) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const SizedBox(height: ZoaSpace.md),
            Text('Add a photo', style: ZoaType.label),
            const SizedBox(height: ZoaSpace.xs),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: ZoaSpace.lg),
              child: Text(
                'We will suggest the material. You confirm it, and the collector '
                'checks it again on pickup.',
                style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
                textAlign: TextAlign.center,
              ),
            ),
            const SizedBox(height: ZoaSpace.md),
            ListTile(
              leading: const Icon(Icons.photo_camera_outlined,
                  color: ZoaColors.forestDeep),
              title: const Text('Take a photo'),
              onTap: () => Navigator.of(sheetContext).pop(ImageSource.camera),
            ),
            ListTile(
              leading: const Icon(Icons.photo_library_outlined,
                  color: ZoaColors.forestDeep),
              title: const Text('Choose from gallery'),
              onTap: () => Navigator.of(sheetContext).pop(ImageSource.gallery),
            ),
            const SizedBox(height: ZoaSpace.sm),
          ],
        ),
      ),
    );

    if (source == null || !mounted) return;
    await _pickPhoto(source);
  }

  bool _validate() {    final weight = double.tryParse(_weight.text.trim().replaceAll(',', '.'));
    final organic = _isOrganic(context.read<AppStatus>().meta);

    setState(() {
      _materialError = _materialKey == null ? 'Choose what you are recycling' : null;
      if (weight == null) {
        _weightError = 'Enter an approximate weight';
      } else if (weight <= 0) {
        _weightError = 'Weight must be more than 0';
      } else if (weight > 5000) {
        _weightError = 'That looks too large — check the decimal point';
      } else {
        _weightError = null;
      }

      // Checked client-side so the user is told before a round trip, but the
      // server enforces it regardless — this is a convenience, not the gate.
      _sourceError = organic && _sourceType == null
          ? 'Tell us where this came from'
          : null;
    });

    return _materialError == null && _weightError == null && _sourceError == null;
  }

  Future<void> _submit() async {
    if (!_validate()) return;

    final submissions = context.read<SubmissionsController>();
    final weight = double.parse(_weight.text.trim().replaceAll(',', '.'));

    final created = await submissions.create(
      materialType: _materialKey!,
      estimatedQtyKg: weight,
      location: _location.text,
      // Sent only when it means something; the server stores it for organics
      // and ignores it elsewhere.
      sourceType: _isOrganic(context.read<AppStatus>().meta) ? _sourceType : null,
      // Sent even when the user overrode it — the disagreement between predicted
      // and confirmed is precisely the measurement FR-22 asks for.
      predictedCategory: _predictedCategory,
      predictedConfidence: _predictedConfidence,
    );

    if (!mounted) return;

    if (created == null) {
      // Surface any field-specific server messages against the right inputs.
      setState(() {
        _materialError = submissions.error?.fields['material_type'];
        _weightError = submissions.error?.fields['estimated_qty_kg'];
        _sourceError = submissions.error?.fields['source_type'];
      });
      return;
    }

    // Clear the form so returning to this tab does not re-offer a stale entry.
    _weight.clear();
    _location.clear();
    setState(() => _materialKey = null);

    await Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => SubmissionStatusScreen(submissionId: created.id),
      ),
    );

    if (!mounted) return;
    // The collector may have verified while the status screen was open.
    await context.read<AuthController>().refresh();
  }

  @override
  Widget build(BuildContext context) {
    final submissions = context.watch<SubmissionsController>();
    final meta = context.watch<AppStatus>().meta;
    final error = submissions.error;
    final showBanner = error != null && error.fields.isEmpty;

    return ZoaPage(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: FadeSlideIn.staggered([
          const SizedBox(height: ZoaSpace.sm),
          const ZoaEyebrow('Log recycling'),
          const SizedBox(height: ZoaSpace.md),
          Text('What are you\nhanding over?', style: ZoaType.h2),
          const SizedBox(height: ZoaSpace.md),
          Text(
            'A collector confirms the weight and type on pickup. Points are '
            'credited from their measurement, not this estimate.',
            style: ZoaType.bodySoft,
          ),
          const SizedBox(height: ZoaSpace.xl),
          if (showBanner) ...[
            ZoaErrorBanner(message: error.message),
            const SizedBox(height: ZoaSpace.lg),
          ],
          _PhotoAssist(
            photoBytes: _photoBytes,
            classification: _classification,
            classifying: submissions.classifying,
            predictionAnswered: _predictionAnswered,
            enabled: !submissions.submitting,
            meta: meta,
            onAdd: _choosePhotoSource,
            onRemove: _removePhoto,
            onRetry: _classifyPhoto,
            onConfirm: _confirmPrediction,
            onReject: _rejectPrediction,
            onUseAlternative: (key) => _useAlternative(key, meta),
          ),
          const SizedBox(height: ZoaSpace.xl),
          _MaterialPicker(
            materials: meta?.materials ?? const [],
            selected: _materialKey,
            error: _materialError,
            onSelect: (key) => setState(() {
              _materialKey = key;
              _materialError = null;
              // Reset the origin when the material changes: a source type left
              // over from a previous pick would silently mislabel this load.
              if (meta?.materialFor(key)?.group != 'organic') {
                _sourceType = null;
              }
              _sourceError = null;
            }),
          ),
          if (_isOrganic(meta)) ...[
            const SizedBox(height: ZoaSpace.xl),
            _SourceTypePicker(
              selected: _sourceType,
              error: _sourceError,
              enabled: !submissions.submitting,
              onSelect: (value) => setState(() {
                _sourceType = value;
                _sourceError = null;
              }),
            ),
          ],
          const SizedBox(height: ZoaSpace.xl),
          ZoaTextField(
            label: 'Approximate weight',
            controller: _weight,
            hint: '4.5',
            helper: 'In kilograms. A rough estimate is fine.',
            errorText: _weightError,
            keyboardType: const TextInputType.numberWithOptions(decimal: true),
            textInputAction: TextInputAction.next,
            mono: true,
            enabled: !submissions.submitting,
          ),
          const SizedBox(height: ZoaSpace.lg),
          ZoaTextField(
            label: 'Pickup or drop-off point',
            controller: _location,
            hint: 'Kilimani drop-off point',
            helper: 'Optional, but it helps the collector find you.',
            textInputAction: TextInputAction.done,
            textCapitalization: TextCapitalization.sentences,
            maxLength: 200,
            enabled: !submissions.submitting,
            onSubmitted: (_) => _submit(),
          ),
          const SizedBox(height: ZoaSpace.xl),
          if (_materialKey != null) ...[
            _RateEstimate(materialKey: _materialKey!, weightText: _weight.text),
            const SizedBox(height: ZoaSpace.lg),
          ],
          ZoaPrimaryButton(
            label: 'Submit for collection',
            icon: Icons.arrow_forward,
            loading: submissions.submitting,
            onPressed: submissions.submitting ? null : _submit,
          ),
          const SizedBox(height: ZoaSpace.xl),
        ]),
      ),
    );
  }
}

/// The material grid, grouped by taxonomy group, using the website's line icons.
class _MaterialPicker extends StatelessWidget {
  const _MaterialPicker({
    required this.materials,
    required this.selected,
    required this.error,
    required this.onSelect,
  });

  final List<MaterialInfo> materials;
  final String? selected;
  final String? error;
  final ValueChanged<String> onSelect;

  @override
  Widget build(BuildContext context) {
    if (materials.isEmpty) {
      // /meta failed. Rather than block the submission, say so plainly — the
      // taxonomy is server-driven and a retry will usually fix it.
      return ZoaCard(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Material list unavailable', style: ZoaType.label),
            const SizedBox(height: ZoaSpace.sm),
            Text(
              'The material list could not be loaded. Pull down on Home to '
              'reconnect, then try again.',
              style: ZoaType.bodySm,
            ),
          ],
        ),
      );
    }

    final grouped = <String, List<MaterialInfo>>{};
    for (final material in materials) {
      grouped.putIfAbsent(material.group, () => []).add(material);
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('Material', style: ZoaType.label),
        if (error != null) ...[
          const SizedBox(height: 5),
          Row(
            children: [
              const Icon(Icons.error_outline, size: 14, color: ZoaColors.statusError),
              const SizedBox(width: 5),
              Text(
                error!,
                style: ZoaType.bodySm.copyWith(color: ZoaColors.statusError),
              ),
            ],
          ),
        ],
        const SizedBox(height: ZoaSpace.md),
        for (final entry in grouped.entries) ...[
          Text(
            _groupLabels[entry.key] ?? entry.key,
            style: ZoaType.tag.copyWith(color: ZoaColors.inkSoft),
          ),
          const SizedBox(height: ZoaSpace.sm),
          Wrap(
            spacing: ZoaSpace.sm,
            runSpacing: ZoaSpace.sm,
            children: [
              for (final material in entry.value)
                _MaterialChip(
                  material: material,
                  selected: material.key == selected,
                  onTap: () => onSelect(material.key),
                ),
            ],
          ),
          const SizedBox(height: ZoaSpace.md),
        ],
      ],
    );
  }
}

/// The optional photo step and its prediction (Phase 2.5).
///
/// Deliberately reads as an offer rather than a requirement: before a photo is
/// added this is a single flat button, and every outcome — a good prediction, a
/// weak one, a degraded call — leaves the material grid below fully usable. The
/// prediction never auto-confirms (TRD §5 risk table); the user answers, and the
/// collector re-checks at verification.
class _PhotoAssist extends StatelessWidget {
  const _PhotoAssist({
    required this.photoBytes,
    required this.classification,
    required this.classifying,
    required this.predictionAnswered,
    required this.enabled,
    required this.meta,
    required this.onAdd,
    required this.onRemove,
    required this.onRetry,
    required this.onConfirm,
    required this.onReject,
    required this.onUseAlternative,
  });

  final Uint8List? photoBytes;
  final Classification? classification;
  final bool classifying;
  final bool predictionAnswered;
  final bool enabled;
  final MetaCatalog? meta;
  final VoidCallback onAdd;
  final VoidCallback onRemove;
  final VoidCallback onRetry;
  final VoidCallback onConfirm;
  final VoidCallback onReject;
  final ValueChanged<String> onUseAlternative;

  @override
  Widget build(BuildContext context) {
    if (photoBytes == null) return _addPrompt();

    return ZoaCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              ClipRRect(
                borderRadius: ZoaRadius.allSm,
                child: Image.memory(
                  photoBytes!,
                  width: 64,
                  height: 64,
                  fit: BoxFit.cover,
                  // A corrupt or unsupported image must not take the form down.
                  errorBuilder: (_, __, ___) => Container(
                    width: 64,
                    height: 64,
                    color: ZoaColors.paper,
                    child: const Icon(Icons.image_not_supported_outlined,
                        size: 22, color: ZoaColors.inkSoft),
                  ),
                ),
              ),
              const SizedBox(width: ZoaSpace.md),
              Expanded(child: _status(context)),
              IconButton(
                onPressed: enabled ? onRemove : null,
                icon: const Icon(Icons.close, size: 18),
                color: ZoaColors.inkSoft,
                tooltip: 'Remove photo',
                visualDensity: VisualDensity.compact,
              ),
            ],
          ),
          if (_showConfirm) ...[
            const SizedBox(height: ZoaSpace.md),
            const Divider(height: 1, color: ZoaColors.line),
            const SizedBox(height: ZoaSpace.md),
            _confirmRow(),
          ],
          if (_showAlternatives) ...[
            const SizedBox(height: ZoaSpace.md),
            _alternatives(),
          ],
        ],
      ),
    );
  }

  bool get _showConfirm =>
      !classifying &&
      !predictionAnswered &&
      (classification?.hasPrediction ?? false);

  /// Runner-ups are offered only while the prediction is unanswered — once the
  /// user has chosen, the material grid is the single place selection happens.
  bool get _showAlternatives =>
      _showConfirm && (classification?.alternatives.isNotEmpty ?? false);

  Widget _addPrompt() {
    return Align(
      alignment: Alignment.centerLeft,
      child: TextButton.icon(
        onPressed: enabled ? onAdd : null,
        icon: const Icon(Icons.photo_camera_outlined, size: 18),
        label: const Text('Add a photo to identify it'),
        style: TextButton.styleFrom(
          foregroundColor: ZoaColors.forestDeep,
          padding: const EdgeInsets.symmetric(
            horizontal: ZoaSpace.sm,
            vertical: ZoaSpace.sm,
          ),
        ),
      ),
    );
  }

  Widget _status(BuildContext context) {
    if (classifying) {
      return Row(
        children: [
          const SizedBox(
            width: 14,
            height: 14,
            child: CircularProgressIndicator(
              strokeWidth: 2,
              color: ZoaColors.leaf,
            ),
          ),
          const SizedBox(width: ZoaSpace.sm),
          Text('Identifying…', style: ZoaType.bodySm),
        ],
      );
    }

    final result = classification;

    if (result == null || !result.hasPrediction) {
      // Degraded, or the request never completed. Stated plainly and without
      // alarm: nothing is broken from the user's point of view, and the grid
      // below still works.
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('Could not identify it', style: ZoaType.label),
          const SizedBox(height: 3),
          Text(
            'Pick the material below — it only takes a tap.',
            style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
          ),
          const SizedBox(height: ZoaSpace.xs),
          Align(
            alignment: Alignment.centerLeft,
            child: TextButton(
              onPressed: enabled ? onRetry : null,
              style: TextButton.styleFrom(
                foregroundColor: ZoaColors.forestDeep,
                padding: EdgeInsets.zero,
                minimumSize: const Size(0, 28),
                tapTargetSize: MaterialTapTargetSize.shrinkWrap,
              ),
              child: const Text('Try again'),
            ),
          ),
        ],
      );
    }

    final label = result.label.isNotEmpty
        ? result.label
        : (meta?.labelFor(result.predictedCategory) ?? result.predictedCategory);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          // Hedged wording when the model is unsure, so the copy matches the
          // number next to it rather than overselling a weak guess.
          result.isLowConfidence ? 'This might be' : 'This looks like',
          style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
        ),
        const SizedBox(height: 2),
        Text(label, style: ZoaType.label),
        const SizedBox(height: 3),
        Row(
          children: [
            Text(
              '${result.confidencePercent}% sure',
              style: ZoaType.pointsCost.copyWith(fontSize: 11),
            ),
            if (result.requiresSourceType) ...[
              const SizedBox(width: ZoaSpace.sm),
              Text(
                '· organic',
                style: ZoaType.bodySm.copyWith(
                  color: ZoaColors.gold,
                  fontSize: 11,
                ),
              ),
            ],
          ],
        ),
      ],
    );
  }

  Widget _confirmRow() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('Is that right?', style: ZoaType.bodySm),
        const SizedBox(height: ZoaSpace.sm),
        Row(
          children: [
            Expanded(
              child: OutlinedButton.icon(
                onPressed: enabled ? onConfirm : null,
                icon: const Icon(Icons.check, size: 16),
                label: const Text('Yes'),
                style: OutlinedButton.styleFrom(
                  foregroundColor: ZoaColors.forestDeep,
                  side: const BorderSide(color: ZoaColors.leaf, width: 1.6),
                  padding: const EdgeInsets.symmetric(vertical: ZoaSpace.sm),
                ),
              ),
            ),
            const SizedBox(width: ZoaSpace.sm),
            Expanded(
              child: OutlinedButton.icon(
                onPressed: enabled ? onReject : null,
                icon: const Icon(Icons.close, size: 16),
                label: const Text('No, I\'ll pick'),
                style: OutlinedButton.styleFrom(
                  foregroundColor: ZoaColors.inkSoft,
                  side: const BorderSide(color: ZoaColors.line),
                  padding: const EdgeInsets.symmetric(vertical: ZoaSpace.sm),
                ),
              ),
            ),
          ],
        ),
      ],
    );
  }

  Widget _alternatives() {
    final alternatives = classification!.alternatives;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'Or one of these',
          style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft, fontSize: 11.5),
        ),
        const SizedBox(height: ZoaSpace.xs),
        Wrap(
          spacing: ZoaSpace.sm,
          runSpacing: ZoaSpace.xs,
          children: [
            for (final alternative in alternatives)
              ActionChip(
                label: Text(
                  meta?.labelFor(alternative.predictedCategory) ??
                      alternative.predictedCategory,
                  style: ZoaType.bodySm.copyWith(fontSize: 12),
                ),
                onPressed: enabled
                    ? () => onUseAlternative(alternative.predictedCategory)
                    : null,
                backgroundColor: ZoaColors.paper,
                side: const BorderSide(color: ZoaColors.line),
                visualDensity: VisualDensity.compact,
              ),
          ],
        ),
      ],
    );
  }
}

/// Waste origin, shown only for organics (FR-24).
///
/// Two options rather than a dropdown: it is a binary choice on the critical
/// path of the flow, and a tap beats opening a menu to pick one of two things.
class _SourceTypePicker extends StatelessWidget {
  const _SourceTypePicker({
    required this.selected,
    required this.error,
    required this.enabled,
    required this.onSelect,
  });

  final String? selected;
  final String? error;
  final bool enabled;
  final ValueChanged<String> onSelect;

  static const _options = [
    (value: 'residential', label: 'A home', icon: Icons.house_outlined),
    (value: 'hotel', label: 'A hotel or restaurant', icon: Icons.storefront_outlined),
  ];

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('Where did it come from?', style: ZoaType.label),
        if (error != null) ...[
          const SizedBox(height: 5),
          Row(
            children: [
              const Icon(Icons.error_outline, size: 14, color: ZoaColors.statusError),
              const SizedBox(width: 5),
              Text(
                error!,
                style: ZoaType.bodySm.copyWith(color: ZoaColors.statusError),
              ),
            ],
          ),
        ],
        const SizedBox(height: 5),
        Text(
          'Hotel kitchens produce far more organic waste than a household, so '
          'collection is routed differently.',
          style: ZoaType.bodySm.copyWith(color: ZoaColors.inkSoft),
        ),
        const SizedBox(height: ZoaSpace.md),
        Row(
          children: [
            for (final option in _options) ...[
              Expanded(
                child: _SourceChip(
                  label: option.label,
                  icon: option.icon,
                  selected: option.value == selected,
                  enabled: enabled,
                  onTap: () => onSelect(option.value),
                ),
              ),
              if (option != _options.last) const SizedBox(width: ZoaSpace.sm),
            ],
          ],
        ),
      ],
    );
  }
}

class _SourceChip extends StatelessWidget {
  const _SourceChip({
    required this.label,
    required this.icon,
    required this.selected,
    required this.enabled,
    required this.onTap,
  });

  final String label;
  final IconData icon;
  final bool selected;
  final bool enabled;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      button: true,
      selected: selected,
      label: label,
      child: Material(
        color: selected ? ZoaColors.leafWash : ZoaColors.paperCard,
        borderRadius: ZoaRadius.allMd,
        child: InkWell(
          onTap: enabled ? onTap : null,
          borderRadius: ZoaRadius.allMd,
          splashColor: ZoaColors.leafWash,
          child: AnimatedContainer(
            duration: ZoaMotion.quick,
            curve: ZoaMotion.curve,
            padding: const EdgeInsets.symmetric(
              horizontal: ZoaSpace.md,
              vertical: ZoaSpace.md,
            ),
            decoration: BoxDecoration(
              borderRadius: ZoaRadius.allMd,
              border: Border.all(
                // Border weight as well as colour, so selection never depends on
                // colour alone (UI/UX doc §5).
                color: selected ? ZoaColors.leaf : ZoaColors.line,
                width: selected ? 1.8 : 1,
              ),
            ),
            child: Column(
              children: [
                Icon(
                  icon,
                  size: 26,
                  color: selected ? ZoaColors.forestDeep : ZoaColors.inkSoft,
                ),
                const SizedBox(height: ZoaSpace.sm),
                Text(
                  label,
                  style: ZoaType.bodySm.copyWith(
                    color: selected ? ZoaColors.forestDeep : ZoaColors.inkSoft,
                    fontWeight: selected ? FontWeight.w600 : FontWeight.w400,
                    fontSize: 12.5,
                    height: 1.25,
                  ),
                  textAlign: TextAlign.center,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _MaterialChip extends StatelessWidget {
  const _MaterialChip({
    required this.material,
    required this.selected,
    required this.onTap,
  });
  final MaterialInfo material;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      button: true,
      selected: selected,
      label: '${material.label}, ${material.pointsPerKg} points per kilogram',
      child: Material(
        color: selected ? ZoaColors.leafWash : ZoaColors.paperCard,
        borderRadius: ZoaRadius.allMd,
        child: InkWell(
          onTap: onTap,
          borderRadius: ZoaRadius.allMd,
          splashColor: ZoaColors.leafWash,
          child: AnimatedContainer(
            duration: ZoaMotion.quick,
            curve: ZoaMotion.curve,
            width: 104,
            padding: const EdgeInsets.symmetric(
              horizontal: ZoaSpace.sm,
              vertical: ZoaSpace.md,
            ),
            decoration: BoxDecoration(
              borderRadius: ZoaRadius.allMd,
              border: Border.all(
                // Selection is carried by border weight as well as colour, so it
                // does not depend on colour alone (UI/UX doc §5).
                color: selected ? ZoaColors.leaf : ZoaColors.line,
                width: selected ? 1.8 : 1,
              ),
            ),
            child: Column(
              children: [
                MaterialIcon.forKey(
                  material.key,
                  group: material.group,
                  size: 34,
                ),
                const SizedBox(height: ZoaSpace.sm),
                Text(
                  material.label,
                  style: ZoaType.bodySm.copyWith(
                    color: selected ? ZoaColors.forestDeep : ZoaColors.inkSoft,
                    fontWeight: selected ? FontWeight.w600 : FontWeight.w400,
                    fontSize: 12.5,
                    height: 1.25,
                  ),
                  textAlign: TextAlign.center,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                ),
                const SizedBox(height: 3),
                Text(
                  '${material.pointsPerKg}/kg',
                  style: ZoaType.pointsCost.copyWith(fontSize: 11),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// Live points estimate for the current selection and weight.
///
/// Framed as an estimate throughout: the real figure comes from the collector's
/// measurement, and promising a number now that later changes would undermine
/// exactly the trust the verification step exists to build.
class _RateEstimate extends StatelessWidget {
  const _RateEstimate({required this.materialKey, required this.weightText});

  final String materialKey;
  final String weightText;

  @override
  Widget build(BuildContext context) {
    final material = context.watch<AppStatus>().meta?.materialFor(materialKey);
    if (material == null) return const SizedBox.shrink();

    final weight = double.tryParse(weightText.trim().replaceAll(',', '.'));
    final estimate = weight == null || weight <= 0
        ? null
        : (weight * material.pointsPerKg).round();

    return ZoaCard(
      accent: true,
      padding: const EdgeInsets.all(ZoaSpace.md),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Estimated at this weight', style: ZoaType.tag),
                const SizedBox(height: 2),
                Text(
                  estimate == null
                      ? 'Enter a weight to see an estimate'
                      : '≈ $estimate points',
                  style: ZoaType.cardTitle.copyWith(
                    color: estimate == null
                        ? ZoaColors.inkSoft
                        : ZoaColors.forestDeep,
                  ),
                ),
              ],
            ),
          ),
          Text('${material.pointsPerKg} pts/kg', style: ZoaType.pointsCost),
        ],
      ),
    );
  }
}
