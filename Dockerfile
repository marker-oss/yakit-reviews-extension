# syntax=docker/dockerfile:1

# ---- web builder ----
# Runs on the native build platform; the SPA bundle is architecture-independent.
FROM --platform=$BUILDPLATFORM node:22-bookworm AS web
WORKDIR /web/admin
COPY web/admin/package.json web/admin/package-lock.json ./
RUN npm ci
COPY web/admin ./
RUN npm run build

# ---- builder ----
# Runs on the native build platform and cross-compiles to the target arch
# (CGO is disabled, so multi-arch images build without QEMU emulation).
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS builder
WORKDIR /src
ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /web/admin/dist ./internal/server/admin_dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
    -trimpath -ldflags "-s -w" \
    -o /out/reviews ./cmd/reviews
RUN mkdir -p /out/data

# ---- final ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

# Static widget assets served by `serve --static-dir`.
COPY --from=builder /src/web/reviews-widget ./web/reviews-widget
COPY --from=builder /out/reviews /usr/local/bin/reviews
COPY --from=builder --chown=nonroot:nonroot /out/data /data

# SQLite database lives on a mounted volume.
ENV REVIEWS_DB_DSN=/data/reviews.db
EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/reviews"]
CMD ["serve", "--addr", "0.0.0.0:8080", "--with-sync"]
