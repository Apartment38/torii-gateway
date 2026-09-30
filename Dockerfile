FROM golang:1.25.1-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/gateway ./cmd/gateway

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/gateway /gateway
VOLUME ["/state"]
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s CMD ["/gateway", "healthcheck", "--url", "http://127.0.0.1:8080/healthz"]
ENTRYPOINT ["/gateway"]
CMD ["serve", "--config", "/config/config.yaml"]
