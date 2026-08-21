/// Persistent storage for the JWT.
///
/// Split out from [AuthController] so [ApiClient] can read the current token
/// without depending on the controller — otherwise the client and the controller
/// would each need the other at construction time.
library;

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

class TokenStore {
  TokenStore({FlutterSecureStorage? storage})
      : _storage = storage ?? const FlutterSecureStorage();

  static const _tokenKey = 'zoa.auth.token';

  final FlutterSecureStorage _storage;

  /// Held in memory as well as in the keystore: the request interceptor reads
  /// this synchronously on every call, and an async keystore read per request
  /// would be both slow and pointless.
  String? _token;

  String? get token => _token;

  bool get hasToken => _token != null && _token!.isNotEmpty;

  /// Loads a previously saved token into memory. Called once on launch.
  ///
  /// A keystore read can fail outright — for instance after an OS restore onto a
  /// new device, where the encryption key no longer exists. That is treated as
  /// "no session" rather than an error, since the only sensible recovery is to
  /// sign in again.
  Future<String?> load() async {
    try {
      _token = await _storage.read(key: _tokenKey);
    } on Exception {
      _token = null;
    }
    return _token;
  }

  Future<void> save(String token) async {
    _token = token;
    await _storage.write(key: _tokenKey, value: token);
  }

  Future<void> clear() async {
    _token = null;
    try {
      await _storage.delete(key: _tokenKey);
    } on Exception {
      // Already gone, or the keystore is unavailable. The in-memory token is
      // cleared either way, so the session is over as far as the app is
      // concerned.
    }
  }
}
