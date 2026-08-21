/// Decides what the app shows at the root: splash, sign-in, or the shell.
///
/// Boot sequence, in order:
///   1. Probe `/health`. Without a backend there is nothing to sign in to, so a
///      failure here stops at the splash with a retry.
///   2. Restore any stored session by validating the token against `/me`.
///   3. Show the shell if signed in, otherwise the login screen.
///
/// Keeping this in one place means no screen navigates imperatively on auth
/// changes — they just change state, and this rebuilds. That is what makes a
/// mid-session 401 land the user on the login screen from anywhere in the app.
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../state/app_status.dart';
import '../state/auth_controller.dart';
import 'auth/login_screen.dart';
import 'home_shell.dart';
import 'splash_screen.dart';

class RootGate extends StatefulWidget {
  const RootGate({super.key});

  @override
  State<RootGate> createState() => _RootGateState();
}

class _RootGateState extends State<RootGate> {
  /// Shortest time the splash stays up. A probe that answers in 2ms would
  /// otherwise flash the brand for a single frame, which reads as a glitch.
  static const _minimumDwell = Duration(milliseconds: 900);

  bool _booted = false;

  @override
  void initState() {
    super.initState();
    // Deferred to after the first frame so `context.read` is safe and the seal is
    // already painted when the requests go out.
    WidgetsBinding.instance.addPostFrameCallback((_) => _boot());
  }

  Future<void> _boot() async {
    final status = context.read<AppStatus>();
    final auth = context.read<AuthController>();
    final started = DateTime.now();

    setState(() => _booted = false);

    await status.check();
    if (!mounted) return;

    if (status.isConnected) {
      await auth.restore();
      if (!mounted) return;
    }

    final elapsed = DateTime.now().difference(started);
    if (elapsed < _minimumDwell) {
      await Future<void>.delayed(_minimumDwell - elapsed);
      if (!mounted) return;
    }

    setState(() => _booted = true);
  }

  @override
  Widget build(BuildContext context) {
    final status = context.watch<AppStatus>();
    final auth = context.watch<AuthController>();

    // Still connecting, or the backend is unreachable.
    if (!_booted || status.checking || !status.isConnected) {
      return SplashScreen(
        checking: status.checking,
        error: status.isConnected ? null : status.error,
        baseUrl: status.baseUrl,
        onRetry: _boot,
      );
    }

    return switch (auth.state) {
      AuthState.restoring => SplashScreen(
          baseUrl: status.baseUrl,
          onRetry: _boot,
        ),
      AuthState.signedOut => const LoginScreen(),
      AuthState.signedIn => const HomeShell(),
    };
  }
}
