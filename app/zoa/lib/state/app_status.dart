/// Backend connectivity state, shared across the app.
///
/// Phase 0's job is proving the app can reach the backend, so this holds the
/// health probe result and the server-driven reference data. Later phases add
/// auth and session state alongside it.
library;

import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import '../api/api_exception.dart';
import '../api/api_models.dart';

class AppStatus extends ChangeNotifier {
  AppStatus(this._api);

  final ApiClient _api;

  HealthStatus? _health;
  MetaCatalog? _meta;
  ApiException? _error;
  bool _checking = false;

  HealthStatus? get health => _health;
  MetaCatalog? get meta => _meta;
  ApiException? get error => _error;
  bool get checking => _checking;

  /// True only when the server answered *and* reported its database live.
  bool get isConnected => _health?.isHealthy ?? false;

  /// Where we are trying to reach — shown in the failure state, since a wrong
  /// host is by far the most common cause and otherwise looks like an outage.
  String get baseUrl => _api.baseUrl;

  /// Probes `/health`, then loads `/meta`.
  ///
  /// Health is the gate: without it there is nothing to show. Meta is
  /// best-effort — losing the taxonomy should not block the app from starting,
  /// since the submission form can fall back to a manual entry path.
  Future<void> check() async {
    _checking = true;
    _error = null;
    notifyListeners();

    try {
      _health = await _api.health();
    } on ApiException catch (error) {
      _health = null;
      _error = error;
      _checking = false;
      notifyListeners();
      return;
    }

    try {
      _meta = await _api.meta();
    } on ApiException catch (error) {
      // Non-fatal: keep the connection, note the gap.
      _meta = null;
      debugPrint('meta unavailable: $error');
    }

    _checking = false;
    notifyListeners();
  }
}
