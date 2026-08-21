/// Voucher catalogue state (Phase 3).
library;

import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import '../api/api_exception.dart';
import '../api/api_models.dart';

class VouchersController extends ChangeNotifier {
  VouchersController(this._api);

  final ApiClient _api;

  List<Voucher> _vouchers = const [];
  List<VoucherPartner> _partners = const [];
  int _pointsBalance = 0;
  ApiException? _error;
  bool _loading = false;

  bool _affordableOnly = false;
  int? _partnerId;

  /// The catalogue as last loaded, cheapest first (the server's ordering is kept
  /// rather than re-sorted here, so one place decides what "first" means).
  List<Voucher> get vouchers => _vouchers;

  /// Active partners, for the filter row.
  List<VoucherPartner> get partners => _partners;

  /// The balance the current affordability flags were computed against.
  int get pointsBalance => _pointsBalance;

  ApiException? get error => _error;
  bool get loading => _loading;

  /// Whether the affordable-only filter is on.
  bool get affordableOnly => _affordableOnly;

  /// The partner filter, or null for all partners.
  int? get partnerId => _partnerId;

  /// True when a filter is hiding something, which is what distinguishes "you
  /// cannot afford anything yet" from "this partner has nothing".
  bool get filtered => _affordableOnly || _partnerId != null;

  /// The cheapest voucher in the catalogue, for the "next reward" hint.
  ///
  /// Reads the first affordable-from-zero entry rather than sorting: the server
  /// already returns cheapest-first.
  Voucher? get cheapest => _vouchers.isEmpty ? null : _vouchers.first;

  /// Loads the catalogue under the current filters.
  Future<void> load() async {
    _loading = true;
    _error = null;
    notifyListeners();

    try {
      final catalogue = await _api.vouchers(
        affordableOnly: _affordableOnly,
        partnerId: _partnerId,
      );
      _vouchers = catalogue.vouchers;
      _pointsBalance = catalogue.pointsBalance;

      // Partners are fetched once and kept: the filter row must list every
      // partner, including any whose vouchers are currently filtered out — a row
      // derived from the visible list would shrink as the user filters and make
      // the control unusable.
      if (_partners.isEmpty) {
        _partners = await _api.partners();
      }
    } on ApiException catch (error) {
      _error = error;
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  /// Toggles the affordable-only filter and reloads.
  Future<void> setAffordableOnly(bool value) async {
    if (_affordableOnly == value) return;
    _affordableOnly = value;
    await load();
  }

  /// Filters to one partner, or clears the filter with null.
  Future<void> setPartner(int? id) async {
    if (_partnerId == id) return;
    _partnerId = id;
    await load();
  }

  /// Re-reads one voucher — used by the detail screen so a stale list entry is
  /// not what the user acts on.
  Future<Voucher?> refreshOne(int id) async {
    try {
      final voucher = await _api.voucher(id);
      _vouchers = [
        for (final item in _vouchers)
          if (item.id == id) voucher else item,
      ];
      _error = null;
      notifyListeners();
      return voucher;
    } on ApiException catch (error) {
      _error = error;
      notifyListeners();
      return null;
    }
  }

  void clearError() {
    if (_error == null) return;
    _error = null;
    notifyListeners();
  }

  /// Clears everything on sign-out, filters included — the next user starts on an
  /// unfiltered catalogue against their own balance.
  void reset() {
    _vouchers = const [];
    _partners = const [];
    _pointsBalance = 0;
    _error = null;
    _loading = false;
    _affordableOnly = false;
    _partnerId = null;
    notifyListeners();
  }
}
