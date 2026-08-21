/// Submission state: the user's own list, and the collector's queue.
library;

import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import '../api/api_exception.dart';
import '../api/api_models.dart';

class SubmissionsController extends ChangeNotifier {
  SubmissionsController(this._api);

  final ApiClient _api;

  List<Submission> _mine = const [];
  List<Submission> _queue = const [];
  ApiException? _error;
  bool _loading = false;
  bool _submitting = false;
  bool _classifying = false;

  /// The signed-in user's own submissions, newest first.
  List<Submission> get mine => _mine;

  /// Open submissions awaiting a collector — populated only for collectors.
  List<Submission> get queue => _queue;

  ApiException? get error => _error;
  bool get loading => _loading;

  /// True while a create or verify call is in flight.
  bool get submitting => _submitting;

  /// True while a photo is being classified. Separate from [submitting] so the
  /// submit button does not spin for an optional assist the user can skip.
  bool get classifying => _classifying;

  /// The most recent submission, used for the home screen's activity summary.
  Submission? get latest => _mine.isEmpty ? null : _mine.first;

  /// Loads the caller's own submissions.
  Future<void> loadMine() async {
    _loading = true;
    _error = null;
    notifyListeners();

    try {
      final page = await _api.submissions(limit: 50);
      _mine = page.submissions;
    } on ApiException catch (error) {
      _error = error;
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  /// Loads the collector queue.
  ///
  /// Fetches `pending` and `collected` separately and merges: a submission that
  /// has been picked up but not yet weighed is still the collector's to finish,
  /// so showing only `pending` would lose it from the queue.
  Future<void> loadQueue() async {
    _loading = true;
    _error = null;
    notifyListeners();

    try {
      final pending = await _api.submissions(status: SubmissionStatus.pending, limit: 100);
      final collected = await _api.submissions(status: SubmissionStatus.collected, limit: 100);

      _queue = [...pending.submissions, ...collected.submissions]
        ..sort((a, b) => a.createdAt.compareTo(b.createdAt)); // oldest first: FIFO
    } on ApiException catch (error) {
      _error = error;
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  /// Creates a submission. Returns it on success, null on failure with [error]
  /// set.
  Future<Submission?> create({
    required String materialType,
    required double estimatedQtyKg,
    String? location,
    String? sourceType,
    String? predictedCategory,
    double? predictedConfidence,
  }) async {
    _submitting = true;
    _error = null;
    notifyListeners();

    try {
      final submission = await _api.createSubmission(
        materialType: materialType,
        estimatedQtyKg: estimatedQtyKg,
        location: location,
        sourceType: sourceType,
        predictedCategory: predictedCategory,
        predictedConfidence: predictedConfidence,
      );
      // Prepend rather than reloading the list: one fewer round trip, and the
      // new submission is visible immediately.
      _mine = [submission, ..._mine];
      return submission;
    } on ApiException catch (error) {
      _error = error;
      return null;
    } finally {
      _submitting = false;
      notifyListeners();
    }
  }

  /// Verifies a submission as a collector. Returns the result, or null on
  /// failure with [error] set.
  Future<VerifyResult?> verify(
    int id, {
    double? verifiedQtyKg,
    String? materialType,
    String status = SubmissionStatus.verified,
  }) async {
    _submitting = true;
    _error = null;
    notifyListeners();

    try {
      final result = await _api.verifySubmission(
        id,
        verifiedQtyKg: verifiedQtyKg,
        materialType: materialType,
        status: status,
      );

      // Drop it from the queue if it reached a terminal state; otherwise replace
      // it in place so a `collected` submission stays visible with its new status.
      if (result.submission.isOpen) {
        _queue = [
          for (final item in _queue)
            if (item.id == id) result.submission else item,
        ];
      } else {
        _queue = _queue.where((item) => item.id != id).toList();
      }

      return result;
    } on ApiException catch (error) {
      _error = error;
      return null;
    } finally {
      _submitting = false;
      notifyListeners();
    }
  }

  /// Re-reads one submission — used by the status screen to poll for the
  /// collector's decision (App Flow §1 has the client poll rather than push).
  Future<Submission?> refreshOne(int id) async {
    try {
      final submission = await _api.submission(id);
      _mine = [
        for (final item in _mine)
          if (item.id == id) submission else item,
      ];
      // Clear on success like every other method here. A poll that fails once
      // and then recovers must not leave an error behind: the banner on the New
      // Submission screen keys off this field, so a stale value would surface a
      // network warning on an unrelated screen.
      _error = null;
      notifyListeners();
      return submission;
    } on ApiException catch (error) {
      _error = error;
      notifyListeners();
      return null;
    }
  }

  /// Classifies a photo (Phase 2.5).
  ///
  /// Never throws and never sets [error]: classification is an assist, and a
  /// failure must not put an error banner over a form the user can still
  /// complete by hand (FR-23). A null return and a `degraded` result mean the
  /// same thing to the caller — ask the user to pick the material.
  Future<Classification?> classify({
    required List<int> photoBytes,
    required String filename,
  }) async {
    _classifying = true;
    notifyListeners();

    try {
      return await _api.classify(photoBytes: photoBytes, filename: filename);
    } on ApiException {
      // Swallowed deliberately. The photo is optional; the flow continues.
      return null;
    } finally {
      _classifying = false;
      notifyListeners();
    }
  }

  void clearError() {
    if (_error == null) return;
    _error = null;
    notifyListeners();
  }

  /// Clears everything on sign-out, so the next user never sees the previous
  /// one's submissions.
  void reset() {
    _mine = const [];
    _queue = const [];
    _error = null;
    _loading = false;
    _submitting = false;
    _classifying = false;
    notifyListeners();
  }
}
