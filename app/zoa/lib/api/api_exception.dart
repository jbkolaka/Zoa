/// Client-side representation of the backend's error envelope.
library;

import 'package:dio/dio.dart';

/// Stable error codes from `docs/API_CONTRACT.md`. Branch on these, never on
/// the message text.
abstract final class ApiErrorCode {
  static const validation = 'validation_error';
  static const unauthorized = 'unauthorized';
  static const forbidden = 'forbidden';
  static const notFound = 'not_found';
  static const methodNotAllowed = 'method_not_allowed';
  static const conflict = 'conflict';
  static const internal = 'internal_error';
  static const unavailable = 'service_unavailable';

  /// Client-side only: the request never reached the server.
  static const network = 'network_error';

  /// Client-side only: the server answered, but not in the shape we expect.
  static const malformed = 'malformed_response';
}

/// A failed API call, already translated into something a screen can show.
class ApiException implements Exception {
  ApiException({
    required this.code,
    required this.message,
    this.statusCode,
    this.fields = const {},
  });

  /// One of [ApiErrorCode].
  final String code;

  /// Human-readable text, safe to surface to the user.
  final String message;

  /// HTTP status, or null when the request never completed.
  final int? statusCode;

  /// Per-field validation messages, keyed by field name.
  final Map<String, String> fields;

  /// True when retrying might plausibly succeed — connectivity faults and
  /// server-side transients. Drives whether a screen offers "Try again".
  bool get isRetryable =>
      code == ApiErrorCode.network ||
      code == ApiErrorCode.unavailable ||
      code == ApiErrorCode.internal;

  /// True when the session is no longer valid and the user must log in again.
  bool get isAuthFailure => code == ApiErrorCode.unauthorized;

  /// Translates a [DioException] into an [ApiException], preferring the
  /// server's own envelope when there is one.
  factory ApiException.fromDio(DioException error) {
    final response = error.response;
    final data = response?.data;

    // The documented shape: {"error": {"code": …, "message": …, "fields": {…}}}
    if (data is Map && data['error'] is Map) {
      final detail = (data['error'] as Map).cast<String, dynamic>();
      final rawFields = detail['fields'];

      return ApiException(
        code: detail['code'] as String? ?? ApiErrorCode.internal,
        message: detail['message'] as String? ?? 'Something went wrong.',
        statusCode: response?.statusCode,
        fields: rawFields is Map
            ? rawFields.map((k, v) => MapEntry(k.toString(), v.toString()))
            : const {},
      );
    }

    // No envelope — classify by transport failure instead. These messages are
    // user-facing, so they describe the situation, not the exception.
    return switch (error.type) {
      DioExceptionType.connectionTimeout ||
      DioExceptionType.sendTimeout ||
      DioExceptionType.receiveTimeout =>
        ApiException(
          code: ApiErrorCode.network,
          message: 'The connection timed out. Check your network and try again.',
        ),
      DioExceptionType.connectionError => ApiException(
          code: ApiErrorCode.network,
          message: 'Could not reach the Zoa server. Check your connection.',
        ),
      DioExceptionType.cancel => ApiException(
          code: ApiErrorCode.network,
          message: 'The request was cancelled.',
        ),
      _ => ApiException(
          code: ApiErrorCode.internal,
          message: 'Unexpected server response.',
          statusCode: response?.statusCode,
        ),
    };
  }

  @override
  String toString() => 'ApiException($code${statusCode != null ? ' $statusCode' : ''}): $message';
}
