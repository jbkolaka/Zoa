# Zoa backend — multi-stage build.
#
# modernc.org/sqlite is pure Go, so CGO stays off and the result is a static
# binary on a distroless base: no C toolchain, no glibc, nothing to patch.
# (docs/03_Tech_Stack.md §3 recommends exactly this for containerising.)

# Must track the toolchain go.mod asks for (currently `go 1.25.0`). An older
# image does not simply warn: Go either refuses the build or silently downloads
# a second toolchain mid-build, which is slow and fails on a sealed builder.
FROM golang:1.25-alpine AS build

WORKDIR /src

# Copy manifests first so dependency download caches independently of source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
        -ldflags="-s -w" \
        -o /out/zoa-api ./cmd/api

# ---

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /data

COPY --from=build /out/zoa-api /usr/local/bin/zoa-api

# The SQLite file lives on a volume so it survives container replacement
# (docs/03_Tech_Stack.md §4).
#
# Caveat when running without a volume: WORKDIR creates /data owned by root,
# and this image drops to nonroot below, so SQLite cannot create app.db there.
# A mounted volume supplies its own ownership and is unaffected. Hosts with no
# volume (Render's free tier, which has no persistent disks) must point DB_PATH
# at a writable location instead — see render.yaml, which uses /tmp.
VOLUME ["/data"]

ENV DB_PATH=/data/app.db \
    PORT=8080 \
    APP_ENV=demo

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/zoa-api"]
