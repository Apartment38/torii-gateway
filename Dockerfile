FROM golang:1.25.1-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/gateway ./cmd/gateway
RUN mkdir -p /out/data && chmod 0700 /out/data
RUN for target in \
      "linux amd64" \
      "linux arm64" \
      "darwin amd64" \
      "darwin arm64" \
      "windows amd64"; do \
      set -- $target; suffix=""; if [ "$1" = "windows" ]; then suffix=".exe"; fi; \
      CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -trimpath -ldflags='-s -w' -o "/out/torii-auth-$1-$2$suffix" ./cmd/torii-auth; \
    done

FROM gcr.io/distroless/static-debian12:nonroot
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
