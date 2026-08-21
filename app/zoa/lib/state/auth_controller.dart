/// Authentication and session state.
///
/// Owns the answer to "who is signed in, and what is their balance" — the two
/// things nearly every screen needs.
library;

import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import '../api/api_exception.dart';
import '../api/api_models.dart';
import 'token_store.dart';

/// Where the app is in the sign-in lifecycle.
enum AuthState {
  /// Launch: checking for a stored token. The splash screen shows during this.
  restoring,

  /// No valid session — show sign in / register.
  signedOut,

  /// Authenticated, with a loaded user.
  signedIn,
}

class AuthController extends ChangeNotifier {
  AuthController({required ApiClient api, required TokenStore tokenStore})
      : _api = api,
        _tokenStore = tokenStore;

  final ApiClient _api;
  final TokenStore _tokenStore;

  AuthState _state = AuthState.restoring;
  ZoaUser? _user;
  ApiException? _error;
  bool _busy = false;

  AuthState get state => _state;
  ZoaUser? get user => _user;

  /// The last failure from a sign-in, registration or refresh attempt.
  ApiException? get error => _error;

  /// True while a request is in flight — drives button loading states.
  bool get busy => _busy;

  bool get isSignedIn => _state == AuthState.signedIn && _user != null;

  /// Points balance, or 0 when signed out. Read straight off the cached user;
  /// call [refresh] to pull a fresh value from the server.
  int get pointsBalance => _user?.pointsBalance ?? 0;

  /// Per-field validation messages from the last failure, keyed by field name.
  Map<String, String> get fieldErrors => _error?.fields ?? const {};

  /// Restores a stored session on launch.
  ///
  /// A token on disk is not proof of a valid session — it may be expired, signed
  /// with a secret the server has since rotated, or belong to a deleted account.
  /// So the token is validated against `/me` before the user is considered signed
  /// in, and discarded if the server rejects it.
  Future<void> restore() async {
    _setState(AuthState.restoring);

    await _tokenStore.load();
    if (!_tokenStore.hasToken) {
      _setState(AuthState.signedOut);
      return;
    }

    try {
      _user = await _api.me();
      _setState(AuthState.signedIn);
    } on ApiException catch (error) {
      if (error.isAuthFailure) {
        // The stored token is no good — clear it rather than leaving a dead
        // token to fail again on the next request.
        await _tokenStore.clear();
        _setState(AuthState.signedOut);
        return;
      }

      // Anything else (server down, no network) is not an auth problem. Keep the
      // token so a retry can still succeed once connectivity returns, and report
      // the failure rather than silently showing the login screen.
      _error = error;
      _setState(AuthState.signedOut);
    }
  }

  /// Registers a new account and signs in. Returns true on success.
  Future<bool> register({
    required String phoneNumber,
    required String name,
    required String password,
  }) {
    return _authenticate(() => _api.register(
          phoneNumber: phoneNumber,
          name: name,
          password: password,
        ));
  }

  /// Signs in with an existing account. Returns true on success.
  Future<bool> signIn({
    required String phoneNumber,
    required String password,
  }) {
    return _authenticate(() => _api.login(
          phoneNumber: phoneNumber,
          password: password,
        ));
  }

  /// Re-reads the current user, picking up any points credited since sign-in.
  ///
  /// Used after a submission is verified (App Flow §1 has the client poll for
  /// the balance update) and on pull-to-refresh.
  Future<void> refresh() async {
    if (!_tokenStore.hasToken) return;

    try {
      _user = await _api.me();
      _error = null;
      notifyListeners();
    } on ApiException catch (error) {
      // A refresh failure must not tear down a working session — the user may
      // simply be in a tunnel. 401 is handled centrally by [handleUnauthorized].
      if (!error.isAuthFailure) {
        _error = error;
        notifyListeners();
      }
    }
  }

  /// Signs out and clears the stored token.
  Future<void> signOut() async {
    await _tokenStore.clear();
    _user = null;
    _error = null;
    _setState(AuthState.signedOut);
  }

  /// Called by [ApiClient] when any request returns 401.
  ///
  /// Centralising this means no screen has to handle "my session died mid-action"
  /// on its own; the app drops to signed-out and the login screen appears.
  void handleUnauthorized() {
    if (_state == AuthState.signedOut) return;
    // Fire-and-forget: this is invoked from a dio interceptor, which cannot wait
    // on a keystore write.
    signOut();
  }

  /// Clears the current error, so a stale message does not persist across
  /// screens or reappear when a form is reopened.
  void clearError() {
    if (_error == null) return;
    _error = null;
    notifyListeners();
  }

  /// Shared path for register and sign-in: run the call, store the token, hold
  /// the user.
  Future<bool> _authenticate(Future<AuthSession> Function() call) async {
    _busy = true;
    _error = null;
    notifyListeners();

    try {
      final session = await call();
      await _tokenStore.save(session.token);
      _user = session.user;
      _busy = false;
      _setState(AuthState.signedIn);
      return true;
    } on ApiException catch (error) {
      _error = error;
      _busy = false;
      notifyListeners();
      return false;
    } on FormatException catch (error) {
      _error = ApiException(
        code: ApiErrorCode.malformed,
        message: 'The server sent an unexpected response. ${error.message}',
      );
      _busy = false;
      notifyListeners();
      return false;
    }
  }

  void _setState(AuthState next) {
    _state = next;
    notifyListeners();
  }
}
