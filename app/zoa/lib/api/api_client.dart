/// The HTTP client for the Zoa backend.
///
/// One instance is shared through `provider`. Every call funnels through
/// [_get]/[_post] so error translation and auth live in exactly one place.
library;

import 'package:dio/dio.dart';

import '../config.dart';
import 'api_exception.dart';
import 'api_models.dart';

/// Supplies the bearer token for outgoing requests, or null when signed out.
/// Phase 1 wires this to secure storage; Phase 0 leaves it unset.
typedef TokenProvider = String? Function();

/// Called when the server rejects the session, so the app can sign out.
typedef UnauthorizedHandler = void Function();

class ApiClient {
  ApiClient({
    String? baseUrl,
    TokenProvider? tokenProvider,
    this.onUnauthorized,
    Dio? dio,
  })  : _tokenProvider = tokenProvider,
        _dio = dio ?? Dio() {
    _dio.options = _dio.options.copyWith(
      baseUrl: baseUrl ?? ZoaConfig.apiBaseUrl,
      connectTimeout: ZoaConfig.requestTimeout,
      receiveTimeout: ZoaConfig.requestTimeout,
      sendTimeout: ZoaConfig.requestTimeout,
      responseType: ResponseType.json,
      contentType: Headers.jsonContentType,
      // Non-2xx is handled by our own translation layer rather than dio's
      // default throw-on-status, so the error envelope is always parsed.
      validateStatus: (status) => status != null && status < 400,
    );

    _dio.interceptors.add(
      InterceptorsWrapper(
        onRequest: (options, handler) {
          final token = _tokenProvider?.call();
          if (token != null && token.isNotEmpty) {
            options.headers['Authorization'] = 'Bearer $token';
          }
          handler.next(options);
        },
        onError: (error, handler) {
          // A 401 means the token is gone or stale; let the app tear down the
          // session once, centrally, instead of in every screen.
          if (error.response?.statusCode == 401) onUnauthorized?.call();
          handler.next(error);
        },
      ),
    );
  }

  final Dio _dio;
  final TokenProvider? _tokenProvider;

  /// Invoked when the server reports 401. Mutable because [AuthController] needs
  /// this client to exist before it can supply the handler — a final constructor
  /// parameter would make the two mutually dependent at construction.
  UnauthorizedHandler? onUnauthorized;

  /// The base URL in use — surfaced on the splash screen so a misconfigured
  /// host is visible immediately rather than looking like a dead server.
  String get baseUrl => _dio.options.baseUrl;

  // ---------- Phase 0 ----------

  /// `GET /health` — also the connectivity probe behind the splash screen.
  Future<HealthStatus> health() async {
    final json = await _get('/health');
    return HealthStatus.fromJson(json);
  }

  /// `GET /meta` — material taxonomy and live points rates.
  Future<MetaCatalog> meta() async {
    final json = await _get('/meta');
    return MetaCatalog.fromJson(json);
  }

  // ---------- Phase 1: auth & user core ----------

  /// `POST /auth/register` — create an account and receive a session.
  ///
  /// The phone number may be in any format the user types; the server
  /// normalises it to `+254…` before storing.
  Future<AuthSession> register({
    required String phoneNumber,
    required String name,
    required String password,
  }) async {
    final json = await _post('/auth/register', body: {
      'phone_number': phoneNumber,
      'name': name,
      'password': password,
    });
    return AuthSession.fromJson(json);
  }

  /// `POST /auth/login` — authenticate and receive a session.
  Future<AuthSession> login({
    required String phoneNumber,
    required String password,
  }) async {
    final json = await _post('/auth/login', body: {
      'phone_number': phoneNumber,
      'password': password,
    });
    return AuthSession.fromJson(json);
  }

  /// `GET /me` — profile plus the live points balance.
  ///
  /// The balance is read from the database server-side, so calling this after a
  /// collector verifies a submission is how the app sees points arrive.
  Future<ZoaUser> me() async {
    final json = await _get('/me');
    return ZoaUser.fromJson(json);
  }

  // ---------- Phase 2: submission flow ----------

  /// `POST /submissions` — log a recycling submission.
  ///
  /// Status is never sent: the server sets `pending` and owns every transition
  /// from there (App Flow §3).
  ///
  /// [sourceType] is required by the server for organic materials (FR-24), and
  /// [predictedCategory]/[predictedConfidence] carry a prior classification
  /// through so predicted-vs-verified accuracy stays measurable (FR-22). All
  /// three are optional here: a submission with no photo and no organics simply
  /// omits them.
  Future<Submission> createSubmission({
    required String materialType,
    required double estimatedQtyKg,
    String? location,
    String? sourceType,
    String? predictedCategory,
    double? predictedConfidence,
  }) async {
    final json = await _post('/submissions', body: {
      'material_type': materialType,
      'estimated_qty_kg': estimatedQtyKg,
      if (location != null && location.trim().isNotEmpty) 'location': location.trim(),
      if (sourceType != null && sourceType.isNotEmpty) 'source_type': sourceType,
      if (predictedCategory != null && predictedCategory.isNotEmpty)
        'predicted_category': predictedCategory,
      if (predictedConfidence != null) 'predicted_confidence': predictedConfidence,
    });
    return Submission.fromJson(json);
  }

  /// `GET /submissions/:id` — one submission's current state.
  Future<Submission> submission(int id) async {
    final json = await _get('/submissions/$id');
    return Submission.fromJson(json);
  }

  /// `GET /submissions` — the caller's own submissions, or every submission when
  /// the caller is a collector or admin.
  ///
  /// Passing `status: 'pending'` as a collector is the verification queue.
  Future<SubmissionPage> submissions({
    String? status,
    int? limit,
    int? offset,
  }) async {
    final json = await _get('/submissions', query: {
      if (status != null && status.isNotEmpty) 'status': status,
      if (limit != null) 'limit': limit,
      if (offset != null) 'offset': offset,
    });
    return SubmissionPage.fromJson(json);
  }

  /// `PATCH /submissions/:id/verify` — the collector's decision.
  ///
  /// [verifiedQtyKg] is required when confirming; [materialType] corrects the
  /// submitted type and changes which rate applies. A 409 means someone else got
  /// there first, which the caller should treat as "reload the queue".
  Future<VerifyResult> verifySubmission(
    int id, {
    double? verifiedQtyKg,
    String? materialType,
    String status = SubmissionStatus.verified,
  }) async {
    final json = await _patch('/submissions/$id/verify', body: {
      'status': status,
      if (verifiedQtyKg != null) 'verified_qty_kg': verifiedQtyKg,
      if (materialType != null && materialType.isNotEmpty)
        'material_type': materialType,
    });
    return VerifyResult.fromJson(json);
  }

  // ---------- Phase 2.5: AI material classification ----------

  /// `POST /submissions/classify` — predict the material from a photo.
  ///
  /// Sends the bytes rather than a path so the caller is free to compress first,
  /// and so this works identically on every platform.
  ///
  /// The returned [Classification] may be `degraded`, which is not an error: the
  /// server answers 200 for every internal failure precisely so the submission
  /// flow can continue with manual selection (FR-23). An [ApiException] here
  /// means the request never completed at all — no network, or a rejected
  /// upload — and the caller should treat that the same way: carry on manually.
  Future<Classification> classify({
    required List<int> photoBytes,
    required String filename,
  }) async {
    final form = FormData.fromMap({
      'photo': MultipartFile.fromBytes(photoBytes, filename: filename),
    });

    try {
      final response = await _dio.post<dynamic>(
        '/submissions/classify',
        data: form,
        options: Options(
          // A photo upload plus the server's model budget needs far longer than
          // an ordinary JSON call; see ZoaConfig.classifyRequestTimeout.
          sendTimeout: ZoaConfig.classifyRequestTimeout,
          receiveTimeout: ZoaConfig.classifyRequestTimeout,
          // Let dio set the multipart boundary itself.
          contentType: null,
        ),
      );
      return Classification.fromJson(_asMap(response.data));
    } on DioException catch (error) {
      throw ApiException.fromDio(error);
    }
  }

  // ---------- Phase 3: voucher catalogue ----------

  /// `GET /vouchers` — the partner catalogue with affordability resolved.
  ///
  /// [affordableOnly] maps to `?affordable=true`. The flag is not sent when
  /// false: the server reads any other value as "no filter", and omitting it
  /// keeps the URL honest about what was asked for.
  Future<VoucherCatalogue> vouchers({
    bool affordableOnly = false,
    int? partnerId,
  }) async {
    final json = await _get('/vouchers', query: {
      if (affordableOnly) 'affordable': 'true',
      if (partnerId != null) 'partner_id': partnerId,
    });
    return VoucherCatalogue.fromJson(json);
  }

  /// `GET /vouchers/:id` — one voucher. 404 for missing or inactive alike.
  Future<Voucher> voucher(int id) async {
    final json = await _get('/vouchers/$id');
    return Voucher.fromJson(json);
  }

  /// `GET /partners` — active partners, for the catalogue's filter row.
  Future<List<VoucherPartner>> partners() async {
    final json = await _get('/partners');
    final raw = json['partners'] as List? ?? const [];
    return raw
        .whereType<Map>()
        .map((p) => VoucherPartner.fromJson(p.cast<String, dynamic>()))
        .toList(growable: false);
  }

  // ---------- Phase 4: redemption & verification ----------

  /// `POST /redemptions` — spend points on a voucher and receive a code.
  ///
  /// The whole deduction is one server-side transaction, so the balance in the
  /// result is authoritative and the app never subtracts locally. A 409 means the
  /// server refused and **nothing was spent** — either the balance was short, or
  /// the last unit went to someone else a moment earlier.
  Future<RedemptionResult> createRedemption({required int voucherId}) async {
    final json = await _post('/redemptions', body: {'voucher_id': voucherId});
    return RedemptionResult.fromJson(json);
  }

  /// `GET /redemptions` — the caller's own codes, newest first, each with its
  /// voucher and partner embedded so the history renders in one call.
  Future<List<Redemption>> redemptions() async {
    final json = await _get('/redemptions');
    final raw = json['redemptions'] as List? ?? const [];
    return raw
        .whereType<Map>()
        .map((r) => Redemption.fromJson(r.cast<String, dynamic>()))
        .toList(growable: false);
  }

  /// `POST /redemptions/:code/verify` — mark a code used. Partner staff or admin.
  ///
  /// Idempotent server-side: a second call for the same code answers 409 rather
  /// than accepting it twice, which is what makes this safe to retry when a till's
  /// connection drops mid-request. Pass a bare code — use
  /// [normaliseRedemptionCode] first if the value came from a paste or a scan.
  Future<RedemptionVerification> verifyRedemption(String code) async {
    final json = await _post('/redemptions/${Uri.encodeComponent(code)}/verify');
    return RedemptionVerification.fromJson(json);
  }

  // ---------- Phase 5: admin ----------

  /// `GET /admin/stats` — the platform overview. Admin only; anything else gets a
  /// 403, so callers should gate the entry point on the role rather than on a
  /// failed request.
  Future<AdminStats> adminStats() async {
    final json = await _get('/admin/stats');
    return AdminStats.fromJson(json);
  }

  // ---------- plumbing ----------

  Future<Map<String, dynamic>> _get(
    String path, {
    Map<String, dynamic>? query,
  }) async {
    try {
      final response = await _dio.get<dynamic>(path, queryParameters: query);
      return _asMap(response.data);
    } on DioException catch (error) {
      throw ApiException.fromDio(error);
    }
  }

  Future<Map<String, dynamic>> _post(String path, {Object? body}) async {
    try {
      final response = await _dio.post<dynamic>(path, data: body);
      return _asMap(response.data);
    } on DioException catch (error) {
      throw ApiException.fromDio(error);
    }
  }

  Future<Map<String, dynamic>> _patch(String path, {Object? body}) async {
    try {
      final response = await _dio.patch<dynamic>(path, data: body);
      return _asMap(response.data);
    } on DioException catch (error) {
      throw ApiException.fromDio(error);
    }
  }

  /// Guards against a proxy or captive portal returning HTML where JSON was
  /// expected — a common failure on public wifi, and one that would otherwise
  /// surface as an unreadable cast error.
  Map<String, dynamic> _asMap(Object? data) {
    if (data is Map) return data.cast<String, dynamic>();
    throw ApiException(
      code: ApiErrorCode.malformed,
      message: 'Expected a JSON object from the server.',
    );
  }
}
