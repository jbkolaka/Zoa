/// Splash / connectivity screen.
///
/// Purely presentational — the boot sequence lives in [RootGate], which decides
/// what to show. The loop seal carries the wait: the design brief names it the
/// platform's identity and asks for it to appear as a real loading state in the
/// app, not only on the marketing page.
library;

import 'package:flutter/material.dart';

import '../api/api_exception.dart';
import '../config.dart';
import '../theme/zoa_colors.dart';
import '../theme/zoa_theme.dart';
import '../widgets/fade_slide_in.dart';
import '../widgets/loop_seal.dart';
import '../widgets/zoa_ui.dart';

class SplashScreen extends StatelessWidget {
  const SplashScreen({
    super.key,
    this.checking = true,
    this.error,
    this.baseUrl,
    this.onRetry,
  });

  /// Whether a connection attempt is in flight — spins the seal's outer ring.
  final bool checking;

  /// Set when the backend could not be reached.
  final ApiException? error;

  /// The host being attempted, shown in the failure state.
  final String? baseUrl;

  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    final failure = error;

    return Scaffold(
      backgroundColor: ZoaColors.paper,
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.symmetric(horizontal: ZoaSpace.xl),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                LoopSeal(
                  size: 260,
                  spinning: checking,
                  subLabel: 'recycle · verify · earn · redeem',
                ),
                const SizedBox(height: ZoaSpace.xxl),
                if (failure != null)
                  _ConnectionFailure(
                    error: failure,
                    baseUrl: baseUrl ?? ZoaConfig.apiBaseUrl,
                    checking: checking,
                    onRetry: onRetry,
                  )
                else
                  const _Connecting(),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _Connecting extends StatelessWidget {
  const _Connecting();

  @override
  Widget build(BuildContext context) {
    return FadeSlideIn(
      child: Column(
        children: [
          Text('Connecting…', style: ZoaType.lead),
          const SizedBox(height: ZoaSpace.sm),
          Text(
            'Points for verified recycling.',
            style: ZoaType.bodySm,
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
  }
}

/// Failure state. Names the host it tried and offers a retry — no silent
/// failures (UI/UX doc §4), and connectivity here is expected to be uneven.
class _ConnectionFailure extends StatelessWidget {
  const _ConnectionFailure({
    required this.error,
    required this.baseUrl,
    required this.checking,
    required this.onRetry,
  });

  final ApiException error;
  final String baseUrl;
  final bool checking;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    return FadeSlideIn(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          ZoaCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const ZoaKicker('Cannot reach the server'),
                const SizedBox(height: ZoaSpace.sm),
                Text(error.message, style: ZoaType.body),
                const SizedBox(height: ZoaSpace.md),
                const Divider(color: ZoaColors.line, height: 1),
                const SizedBox(height: ZoaSpace.md),
                Text('Trying', style: ZoaType.tag),
                const SizedBox(height: 2),
                Text(baseUrl, style: ZoaType.mono),
                const SizedBox(height: ZoaSpace.md),
                Text(
                  'Web uses http://localhost:8080. Android emulators reach '
                  'the host at 10.0.2.2. For a real device or scrcpy, run '
                  'adb reverse tcp:8080 tcp:8080 and point the app at '
                  'http://127.0.0.1:8080, or pass your machine\'s LAN '
                  'address with --dart-define=ZOA_API_BASE_URL=http://<ip>:8080',
                  style: ZoaType.bodySm,
                ),
              ],
            ),
          ),
          const SizedBox(height: ZoaSpace.lg),
          ZoaPrimaryButton(
            label: 'Try again',
            loading: checking,
            onPressed: onRetry,
          ),
        ],
      ),
    );
  }
}
