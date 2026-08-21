/// Build-time configuration.
///
/// Override the API host without editing source:
///
/// ```sh
/// flutter run --dart-define=ZOA_API_BASE_URL=http://192.168.1.42:8080
/// ```
library;

import 'package:flutter/foundation.dart';

abstract final class ZoaConfig {
  /// Base URL of the Go backend.
  ///
  /// Web defaults to `http://localhost:8080`, which the browser resolves to the
  /// machine the backend runs on.
  ///
  /// Everything else defaults to `http://127.0.0.1:8080`, reached through
  /// `adb reverse tcp:8080 tcp:8080`. That one address covers a USB-connected
  /// physical device — including one mirrored with `scrcpy` — *and* an emulator,
  /// because `adb reverse` works on both. Defaulting to the emulator's `10.0.2.2`
  /// host alias instead would mean the more common case (a real phone) is the one
  /// that needs a flag, and silently fails without it. An emulator running
  /// without the reverse tunnel wants that alias explicitly:
  ///
  /// ```sh
  /// flutter run --dart-define=ZOA_API_BASE_URL=http://10.0.2.2:8080
  /// ```
  ///
  /// Must stay `const`. `String.fromEnvironment` only reads `--dart-define` when
  /// it is evaluated in a const context; as a `final` it returns the default and
  /// discards every `--dart-define=ZOA_API_BASE_URL=...` without warning, which
  /// looks exactly like the override being wrong rather than ignored.
  static const String apiBaseUrl = String.fromEnvironment(
    'ZOA_API_BASE_URL',
    defaultValue: kIsWeb ? 'http://localhost:8080' : 'http://127.0.0.1:8080',
  );

  /// Product name, as shown in the UI.
  static const String appName = 'Zoa';

  /// How long to wait on a normal request before giving up.
  static const Duration requestTimeout = Duration(seconds: 15);

  /// The server's own budget for the classification call (TRD §3 caps it at ~3s
  /// so the submission flow never stalls behind the model).
  ///
  /// This is the *server's* ceiling, not the client's: the backend abandons a
  /// slow model at this point and answers `200 {"degraded": true}`. The client
  /// must therefore wait longer than this — see [classifyRequestTimeout].
  static const Duration classifyBudget = Duration(seconds: 3);

  /// Ceiling for the whole classify round trip: photo upload, then the server's
  /// [classifyBudget], then the response.
  ///
  /// Deliberately much larger than the budget. A 3 MB photo on Kenyan mobile
  /// data can take ten seconds to upload before the model is even reached, and a
  /// client that gave up at 3s would abandon the request just before the server
  /// returned the graceful "pick it manually" answer — turning a handled
  /// degradation into a network error for no reason.
  static const Duration classifyRequestTimeout = Duration(seconds: 40);
}
