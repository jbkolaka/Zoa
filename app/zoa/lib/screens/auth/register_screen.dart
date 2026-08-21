/// Create an account.
library;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';

import '../../state/auth_controller.dart';
import '../../theme/zoa_colors.dart';
import '../../theme/zoa_theme.dart';
import '../../widgets/fade_slide_in.dart';
import '../../widgets/zoa_text_field.dart';
import '../../widgets/zoa_ui.dart';

/// Mirrors the backend's [auth.MinPasswordLength]. Kept in sync deliberately:
/// the client should not ask for a password the server will reject.
const int _minPasswordLength = 6;

class RegisterScreen extends StatefulWidget {
  const RegisterScreen({super.key});

  @override
  State<RegisterScreen> createState() => _RegisterScreenState();
}

class _RegisterScreenState extends State<RegisterScreen> {
  final _name = TextEditingController();
  final _phone = TextEditingController();
  final _password = TextEditingController();
  final _phoneFocus = FocusNode();
  final _passwordFocus = FocusNode();

  String? _nameError;
  String? _phoneError;
  String? _passwordError;

  @override
  void dispose() {
    _name.dispose();
    _phone.dispose();
    _password.dispose();
    _phoneFocus.dispose();
    _passwordFocus.dispose();
    super.dispose();
  }

  bool _validate() {
    final name = _name.text.trim();
    final phone = _phone.text.trim();
    final password = _password.text;

    setState(() {
      _nameError = name.isEmpty ? 'Enter your name' : null;
      _phoneError = phone.isEmpty ? 'Enter your phone number' : null;
      _passwordError = password.length < _minPasswordLength
          ? 'Use at least $_minPasswordLength characters'
          : null;
    });

    return _nameError == null && _phoneError == null && _passwordError == null;
  }

  Future<void> _submit() async {
    if (!_validate()) return;

    final auth = context.read<AuthController>();
    final registered = await auth.register(
      phoneNumber: _phone.text.trim(),
      name: _name.text.trim(),
      password: _password.text,
    );

    if (!mounted) return;

    if (registered) {
      // Registration signs the user in, so the root listener swaps in the shell.
      // Popping clears this route from the stack behind it.
      Navigator.of(context).pop();
      return;
    }

    setState(() {
      _nameError = auth.fieldErrors['name'];
      _phoneError = auth.fieldErrors['phone_number'];
      _passwordError = auth.fieldErrors['password'];
    });
  }

  @override
  Widget build(BuildContext context) {
    final auth = context.watch<AuthController>();
    final error = auth.error;
    final showBanner = error != null && error.fields.isEmpty;

    return Scaffold(
      backgroundColor: ZoaColors.paper,
      appBar: AppBar(
        backgroundColor: ZoaColors.paper,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        foregroundColor: ZoaColors.forestDeep,
        title: Text('Create account', style: ZoaType.label),
      ),
      body: SafeArea(
        child: ZoaPage(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: FadeSlideIn.staggered([
              const SizedBox(height: ZoaSpace.md),
              const ZoaEyebrow('Join as a recycler'),
              const SizedBox(height: ZoaSpace.md),
              Text('Start earning for\nwhat you recycle.', style: ZoaType.h2),
              const SizedBox(height: ZoaSpace.md),
              Text(
                'No cash, no payouts — verified recycling becomes points, and '
                'points become discounts at stores you already use.',
                style: ZoaType.bodySoft,
              ),
              const SizedBox(height: ZoaSpace.xxl),
              if (showBanner) ...[
                ZoaErrorBanner(message: error.message),
                const SizedBox(height: ZoaSpace.lg),
              ],
              ZoaTextField(
                label: 'Your name',
                controller: _name,
                hint: 'Amina Wanjiru',
                errorText: _nameError,
                textCapitalization: TextCapitalization.words,
                textInputAction: TextInputAction.next,
                autofillHints: const [AutofillHints.name],
                maxLength: 80,
                enabled: !auth.busy,
                onSubmitted: (_) => _phoneFocus.requestFocus(),
              ),
              const SizedBox(height: ZoaSpace.lg),
              ZoaTextField(
                label: 'Phone number',
                controller: _phone,
                focusNode: _phoneFocus,
                hint: '07XX XXX XXX',
                helper: 'This is how you sign in. Any format works.',
                errorText: _phoneError,
                keyboardType: TextInputType.phone,
                textInputAction: TextInputAction.next,
                autofillHints: const [AutofillHints.telephoneNumber],
                inputFormatters: [
                  FilteringTextInputFormatter.allow(RegExp(r'[\d+\s]')),
                ],
                mono: true,
                enabled: !auth.busy,
                onSubmitted: (_) => _passwordFocus.requestFocus(),
              ),
              const SizedBox(height: ZoaSpace.lg),
              ZoaTextField(
                label: 'Password',
                controller: _password,
                focusNode: _passwordFocus,
                helper: 'At least $_minPasswordLength characters.',
                errorText: _passwordError,
                obscure: true,
                textInputAction: TextInputAction.done,
                autofillHints: const [AutofillHints.newPassword],
                enabled: !auth.busy,
                onSubmitted: (_) => _submit(),
              ),
              const SizedBox(height: ZoaSpace.xxl),
              ZoaPrimaryButton(
                label: 'Create account',
                loading: auth.busy,
                onPressed: auth.busy ? null : _submit,
              ),
              const SizedBox(height: ZoaSpace.lg),
              Text(
                'By creating an account you agree that submissions are verified '
                'by a collector before points are credited.',
                style: ZoaType.bodySm,
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: ZoaSpace.xl),
            ]),
          ),
        ),
      ),
    );
  }
}
