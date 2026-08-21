/// Text input, styled from the design system.
///
/// Built on a bare [TextField] inside our own container rather than
/// `InputDecoration` borders, for the same reason the rest of the chrome is
/// hand-rolled: full control over the paper/hairline/sage-focus treatment, and no
/// dependency on Flutter's input decoration theme classes.
library;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';

class ZoaTextField extends StatefulWidget {
  const ZoaTextField({
    super.key,
    required this.label,
    required this.controller,
    this.hint,
    this.helper,
    this.errorText,
    this.obscure = false,
    this.keyboardType,
    this.textInputAction,
    this.textCapitalization = TextCapitalization.none,
    this.autofillHints,
    this.inputFormatters,
    this.enabled = true,
    this.mono = false,
    this.maxLength,
    this.onSubmitted,
    this.focusNode,
  });

  final String label;
  final TextEditingController controller;
  final String? hint;

  /// Persistent guidance below the field, e.g. the accepted phone format.
  final String? helper;

  /// Validation message. Replaces [helper] while set.
  final String? errorText;

  final bool obscure;
  final TextInputType? keyboardType;
  final TextInputAction? textInputAction;

  /// Auto-capitalisation behaviour. Names use [TextCapitalization.words].
  final TextCapitalization textCapitalization;

  final Iterable<String>? autofillHints;
  final List<TextInputFormatter>? inputFormatters;
  final bool enabled;

  /// Render the value in mono — for phone numbers and codes, which read as data.
  final bool mono;

  final int? maxLength;
  final ValueChanged<String>? onSubmitted;
  final FocusNode? focusNode;

  @override
  State<ZoaTextField> createState() => _ZoaTextFieldState();
}

class _ZoaTextFieldState extends State<ZoaTextField> {
  FocusNode? _ownedNode;
  bool _focused = false;
  bool _revealed = false;

  FocusNode get _node => widget.focusNode ?? (_ownedNode ??= FocusNode());

  @override
  void initState() {
    super.initState();
    _node.addListener(_onFocusChange);
  }

  void _onFocusChange() {
    if (_focused == _node.hasFocus) return;
    setState(() => _focused = _node.hasFocus);
  }

  @override
  void dispose() {
    _node.removeListener(_onFocusChange);
    // Only dispose a node this widget created; a caller-supplied node belongs to
    // the caller.
    _ownedNode?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final hasError = widget.errorText != null && widget.errorText!.isNotEmpty;

    final Color borderColor;
    if (hasError) {
      borderColor = ZoaColors.statusError;
    } else if (_focused) {
      borderColor = ZoaColors.leaf;
    } else {
      borderColor = ZoaColors.line;
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(widget.label, style: ZoaType.label),
        const SizedBox(height: 6),
        AnimatedContainer(
          duration: ZoaMotion.quick,
          curve: ZoaMotion.curve,
          decoration: BoxDecoration(
            color: widget.enabled ? ZoaColors.paperCard : ZoaColors.paper,
            borderRadius: ZoaRadius.allSm,
            border: Border.all(
              color: borderColor,
              width: _focused || hasError ? 1.5 : 1,
            ),
          ),
          child: Row(
            children: [
              Expanded(
                child: TextField(
                  controller: widget.controller,
                  focusNode: _node,
                  enabled: widget.enabled,
                  obscureText: widget.obscure && !_revealed,
                  keyboardType: widget.keyboardType,
                  textInputAction: widget.textInputAction,
                  textCapitalization: widget.textCapitalization,
                  autofillHints: widget.autofillHints,
                  inputFormatters: widget.inputFormatters,
                  maxLength: widget.maxLength,
                  onSubmitted: widget.onSubmitted,
                  style: widget.mono
                      ? ZoaType.mono.copyWith(color: ZoaColors.ink, fontSize: 15)
                      : ZoaType.body,
                  cursorColor: ZoaColors.leaf,
                  decoration: InputDecoration(
                    hintText: widget.hint,
                    hintStyle: ZoaType.bodySm.copyWith(
                      color: ZoaColors.inkSoft.withOpacity(0.7),
                    ),
                    // The container draws the border; the field must not add one.
                    border: InputBorder.none,
                    enabledBorder: InputBorder.none,
                    focusedBorder: InputBorder.none,
                    disabledBorder: InputBorder.none,
                    // maxLength would otherwise render a character counter that
                    // clashes with the helper/error line below.
                    counterText: '',
                    isDense: true,
                    contentPadding: const EdgeInsets.symmetric(
                      horizontal: 14,
                      vertical: 14,
                    ),
                  ),
                ),
              ),
              if (widget.obscure)
                // A reveal toggle matters more than usual here: this is a phone
                // keypad, and a mistyped invisible password is the most likely
                // reason a sign-in fails.
                Semantics(
                  label: _revealed ? 'Hide password' : 'Show password',
                  button: true,
                  child: IconButton(
                    icon: Icon(
                      _revealed
                          ? Icons.visibility_off_outlined
                          : Icons.visibility_outlined,
                      size: 20,
                      color: ZoaColors.inkSoft,
                    ),
                    onPressed: () => setState(() => _revealed = !_revealed),
                  ),
                ),
            ],
          ),
        ),
        if (hasError) ...[
          const SizedBox(height: 5),
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Icon(
                Icons.error_outline,
                size: 14,
                color: ZoaColors.statusError,
              ),
              const SizedBox(width: 5),
              Expanded(
                child: Text(
                  widget.errorText!,
                  style: ZoaType.bodySm.copyWith(color: ZoaColors.statusError),
                ),
              ),
            ],
          ),
        ] else if (widget.helper != null) ...[
          const SizedBox(height: 5),
          Text(widget.helper!, style: ZoaType.bodySm),
        ],
      ],
    );
  }
}
