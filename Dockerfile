FROM golang:1.25.1-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-$(go env GOARCH)} go build -trimpath \
    -ldflags="-s -w -X github.com/Apartment38/torii-gateway/internal/buildinfo.Version=$VERSION -X github.com/Apartment38/torii-gateway/internal/buildinfo.Commit=$COMMIT -X github.com/Apartment38/torii-gateway/internal/buildinfo.BuildDate=$BUILD_DATE" \
    -o /out/gateway ./cmd/gateway
RUN mkdir -p /out/data && chmod 0700 /out/data
RUN for target in \
      "linux amd64" \
      "linux arm64" \
      "darwin amd64" \
      "darwin arm64" \
      "windows amd64"; do \
      set -- $target; suffix=""; if [ "$1" = "windows" ]; then suffix=".exe"; fi; \
      CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -trimpath \
        -ldflags="-s -w -X github.com/Apartment38/torii-gateway/internal/buildinfo.Version=$VERSION -X github.com/Apartment38/torii-gateway/internal/buildinfo.Commit=$COMMIT -X github.com/Apartment38/torii-gateway/internal/buildinfo.BuildDate=$BUILD_DATE" \
        -o "/out/torii-auth-$1-$2$suffix" ./cmd/torii-auth; \
    done

FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="Torii Gateway" \
      org.opencontainers.image.source="https://github.com/Apartment38/torii-gateway" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$COMMIT" \
      org.opencontainers.image.created="$BUILD_DATE"
COPY --from=build /out/gateway /gateway
COPY --from=build /out/torii-auth-* /helpers/
COPY --from=build --chown=65532:65532 /out/data /data
VOLUME ["/data"]
WORKDIR /data
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s CMD ["/gateway", "healthcheck", "--url", "http://127.0.0.1:8080/healthz"]
ENTRYPOINT ["/gateway"]
CMD ["serve", "--config", "/config/config.yaml"]
