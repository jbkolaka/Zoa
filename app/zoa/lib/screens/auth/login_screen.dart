/// Sign in with a phone number and password.
library;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';

import '../../state/auth_controller.dart';
import '../../theme/zoa_colors.dart';
import '../../theme/zoa_theme.dart';
import '../../widgets/brand_mark.dart';
import '../../widgets/fade_slide_in.dart';
import '../../widgets/zoa_text_field.dart';
import '../../widgets/zoa_ui.dart';
import 'register_screen.dart';

class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key});

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _phone = TextEditingController();
  final _password = TextEditingController();
  final _passwordFocus = FocusNode();

  /// Client-side validation messages, separate from the server's.
  String? _phoneError;
  String? _passwordError;

  @override
  void dispose() {
    _phone.dispose();
    _password.dispose();
    _passwordFocus.dispose();
    super.dispose();
  }

  /// Catches the obvious mistakes before spending a round trip. The server
  /// remains the authority — this only saves the user a wait.
  bool _validate() {
    final phone = _phone.text.trim();
    final password = _password.text;

    setState(() {
      _phoneError = phone.isEmpty ? 'Enter your phone number' : null;
      _passwordError = password.isEmpty ? 'Enter your password' : null;
    });

    return _phoneError == null && _passwordError == null;
  }

  Future<void> _submit() async {
    if (!_validate()) return;

    final auth = context.read<AuthController>();
    final signedIn = await auth.signIn(
      phoneNumber: _phone.text.trim(),
      password: _password.text,
    );

    if (!mounted || signedIn) return;
    // On failure the auth controller holds the error; surface any field-specific
    // messages against the right inputs.
    setState(() {
      _phoneError = auth.fieldErrors['phone_number'];
      _passwordError = auth.fieldErrors['password'];
    });
  }

  void _goToRegister() {
    context.read<AuthController>().clearError();
    Navigator.of(context).push(
      MaterialPageRoute<void>(builder: (_) => const RegisterScreen()),
    );
  }

  @override
  Widget build(BuildContext context) {
    final auth = context.watch<AuthController>();
    final error = auth.error;

    // Field-level messages are shown inline; only show the banner for errors that
    // belong to no single field (wrong credentials, server unreachable).
    final showBanner = error != null && error.fields.isEmpty;

    return Scaffold(
      backgroundColor: ZoaColors.paper,
      body: SafeArea(
        child: ZoaPage(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: FadeSlideIn.staggered([
              const SizedBox(height: ZoaSpace.xl),
              Row(
                children: [
                  const ZoaBrandMark(size: 32),
                  const SizedBox(width: ZoaSpace.sm),
                  Text('Zoa', style: ZoaType.h3.copyWith(fontSize: 22)),
                ],
              ),
              const SizedBox(height: ZoaSpace.xxl),
              const ZoaEyebrow('Welcome back'),
              const SizedBox(height: ZoaSpace.md),
              Text('Sign in to your\nrecycling account.', style: ZoaType.h2),
              const SizedBox(height: ZoaSpace.md),
              Text(
                'Your points and redemption history are tied to your phone '
                'number.',
                style: ZoaType.bodySoft,
              ),
              const SizedBox(height: ZoaSpace.xxl),
              if (showBanner) ...[
                ZoaErrorBanner(message: error.message),
                const SizedBox(height: ZoaSpace.lg),
              ],
              ZoaTextField(
                label: 'Phone number',
                controller: _phone,
                hint: '07XX XXX XXX',
                errorText: _phoneError,
                keyboardType: TextInputType.phone,
                textInputAction: TextInputAction.next,
                autofillHints: const [AutofillHints.telephoneNumber],
                inputFormatters: [
                  // Digits, spaces and a leading +: everything a Kenyan number
                  // might be written with, and nothing else.
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
                errorText: _passwordError,
                obscure: true,
                textInputAction: TextInputAction.done,
                autofillHints: const [AutofillHints.password],
                enabled: !auth.busy,
                onSubmitted: (_) => _submit(),
              ),
              const SizedBox(height: ZoaSpace.xxl),
              ZoaPrimaryButton(
                label: 'Sign in',
                loading: auth.busy,
                onPressed: auth.busy ? null : _submit,
              ),
              const SizedBox(height: ZoaSpace.md),
              ZoaGhostButton(
                label: 'Create an account',
                onPressed: auth.busy ? null : _goToRegister,
              ),
              const SizedBox(height: ZoaSpace.xl),
            ]),
          ),
        ),
      ),
    );
  }
}
