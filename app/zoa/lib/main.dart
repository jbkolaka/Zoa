/// Zoa — recycling rewards for Kenyan households and hotels.
///
/// Entry point. Constructs the client and controllers here, before `runApp`,
/// because [ApiClient] and [AuthController] each need the other: the client reads
/// the current token, and the controller reacts to the client's 401s. Building
/// them in order and wiring the callback afterwards keeps that acyclic.
library;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import 'api/api_client.dart';
import 'config.dart';
import 'screens/root_gate.dart';
import 'state/app_status.dart';
import 'state/auth_controller.dart';
import 'state/redemptions_controller.dart';
import 'state/submissions_controller.dart';
import 'state/token_store.dart';
import 'state/vouchers_controller.dart';
import 'theme/zoa_theme.dart';

void main() {
  final tokenStore = TokenStore();

  // The client reads the token synchronously on every request rather than being
  // handed one, so a sign-in or sign-out takes effect without rebuilding it.
  final api = ApiClient(tokenProvider: () => tokenStore.token);

  final auth = AuthController(api: api, tokenStore: tokenStore);

  // Any 401, from any request, drops the session exactly once and centrally.
  api.onUnauthorized = auth.handleUnauthorized;

  runApp(ZoaApp(api: api, auth: auth));
}

class ZoaApp extends StatelessWidget {
  const ZoaApp({super.key, required this.api, required this.auth});

  final ApiClient api;
  final AuthController auth;

  @override
  Widget build(BuildContext context) {
    return MultiProvider(
      providers: [
        Provider<ApiClient>.value(value: api),
        ChangeNotifierProvider<AuthController>.value(value: auth),
        ChangeNotifierProvider<AppStatus>(create: (_) => AppStatus(api)),
        ChangeNotifierProvider<SubmissionsController>(
          create: (_) => SubmissionsController(api),
        ),
        ChangeNotifierProvider<VouchersController>(
          create: (_) => VouchersController(api),
        ),
        ChangeNotifierProvider<RedemptionsController>(
          create: (_) => RedemptionsController(api),
        ),
      ],
      child: MaterialApp(
        title: ZoaConfig.appName,
        debugShowCheckedModeBanner: false,
        theme: buildZoaTheme(),
        home: const RootGate(),
        builder: (context, child) {
          // Respect the system font scale (UI/UX doc §5 — support large text),
          // but cap it: past ~1.4× the points balance and redemption code stop
          // fitting their cards, and a clipped code is worse than a smaller one.
          final media = MediaQuery.of(context);
          return MediaQuery(
            data: media.copyWith(
              textScaler: media.textScaler.clamp(maxScaleFactor: 1.4),
            ),
            child: child ?? const SizedBox.shrink(),
          );
        },
      ),
    );
  }
}
