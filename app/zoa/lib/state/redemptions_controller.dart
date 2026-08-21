/// Redemption state: the user's own codes, and the partner's code check.
///
/// Both live here for the same reason [SubmissionsController] holds the user's
/// submissions alongside the collector queue: the two halves act on one resource,
/// and splitting them would duplicate the loading and error plumbing.
library;

import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import '../api/api_exception.dart';
import '../api/api_models.dart';

class RedemptionsController extends ChangeNotifier {
  RedemptionsController(this._api);

  final ApiClient _api;

  List<Redemption> _mine = const [];
  ApiException? _error;
  bool _loading = false;
  bool _submitting = false;

  /// The signed-in user's codes, newest first.
  List<Redemption> get mine => _mine;

  ApiException? get error => _error;
  bool get loading => _loading;

  /// True while a redeem or verify call is in flight — drives button spinners, and
  /// is what stops a double-tap becoming a second request. The server refuses one
  /// anyway; this keeps the user from having to see that.
  bool get submitting => _submitting;

  /// How many codes are still spendable — what the "Your codes" card counts.
  ///
  /// Uses [Redemption.isRedeemable] rather than the status alone, so a code that
  /// has quietly passed its expiry is not advertised as available.
  int get activeCount => _mine.where((r) => r.isRedeemable).length;

  /// Loads the caller's redemption history.
  Future<void> load() async {
    _loading = true;
    _error = null;
    notifyListeners();

    try {
      _mine = await _api.redemptions();
    } on ApiException catch (error) {
      _error = error;
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  /// Spends points on a voucher. Returns the result, or null on failure with
  /// [error] set.
  ///
  /// A 409 here is not a bug and not a lost payment — the server's transaction
  /// refuses before deducting — so the caller should surface [error]'s message and
  /// reload the catalogue rather than retrying blindly.
  Future<RedemptionResult?> redeem(int voucherId) async {
    _submitting = true;
    _error = null;
    notifyListeners();

    try {
      final result = await _api.createRedemption(voucherId: voucherId);

      // Prepend rather than reloading: one fewer round trip, and the new code is
      // in the history the moment the user backs out of the confirmation screen.
      // The voucher is reattached because a create response carries it as a
      // sibling field, while the listing embeds it — so both paths render alike.
      _mine = [result.redemption.withVoucher(result.voucher), ..._mine];
      return result;
    } on ApiException catch (error) {
      _error = error;
      return null;
    } on FormatException catch (error) {
      _error = ApiException(
        code: ApiErrorCode.malformed,
        message: 'The server sent an unexpected response. ${error.message}',
      );
      return null;
    } finally {
      _submitting = false;
      notifyListeners();
    }
  }

  /// Verifies a code as partner staff. Returns the verification, or null on
  /// failure with [error] set.
  ///
  /// A 409 is the interesting case rather than an error to hide: it means the code
  /// was already spent or has expired, and the cashier must be told plainly not to
  /// apply the discount.
  Future<RedemptionVerification?> verify(String code) async {
    _submitting = true;
    _error = null;
    notifyListeners();

    try {
      return await _api.verifyRedemption(normaliseRedemptionCode(code));
    } on ApiException catch (error) {
      _error = error;
      return null;
    } on FormatException catch (error) {
      _error = ApiException(
        code: ApiErrorCode.malformed,
        message: 'The server sent an unexpected response. ${error.message}',
      );
      return null;
    } finally {
      _submitting = false;
      notifyListeners();
    }
  }

  void clearError() {
    if (_error == null) return;
    _error = null;
    notifyListeners();
  }

  /// Clears everything on sign-out. Codes are bearer-like, so the next user on a
  /// shared handset must not find the previous one's still on screen.
  void reset() {
    _mine = const [];
    _error = null;
    _loading = false;
    _submitting = false;
    notifyListeners();
  }
}
