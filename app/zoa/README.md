# Zoa app

Flutter client for the Zoa backend.

## Run the backend

From the repo root:

```sh
cd backend
go run ./cmd/api
```

Optional demo data:

```sh
go run ./cmd/api -seed-demo
```

## Run the app

### Web

```sh
cd app/zoa
flutter run -d chrome
```

Web defaults to `http://localhost:8080`.

### Physical Android device / scrcpy

This is the default target. Set up the reverse tunnel once per connect, then run:

```sh
adb reverse tcp:8080 tcp:8080
flutter run -d android
```

`127.0.0.1:8080` is the built-in default for every non-web platform, so no
`--dart-define` is needed here. Debug and profile builds also permit cleartext
HTTP (`android:usesCleartextTraffic`), which Android otherwise blocks at
targetSdk 28+ — including for `127.0.0.1`.

If you do not want to use `adb reverse`, pass your computer's LAN IP instead:

```sh
flutter run -d android --dart-define=ZOA_API_BASE_URL=http://192.168.1.42:8080
```

### Android emulator

`adb reverse` works on emulators too, so the default above is fine. Without the
tunnel, the emulator reaches your host at `10.0.2.2` and needs the flag:

```sh
flutter run -d android --dart-define=ZOA_API_BASE_URL=http://10.0.2.2:8080
```

## Notes

- `CORS_ORIGINS` defaults to `*` in dev, so Flutter web can call the API.
- The backend stores data in `backend/app.db` by default.
- Override the API host with `--dart-define=ZOA_API_BASE_URL=...` if needed.
