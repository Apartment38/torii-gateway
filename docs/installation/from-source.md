# Compile from source

Build the gateway.
You need Git, Go 1.25.1 or newer, and network access to fetch Go dependencies.

1. Check your toolchain in a terminal:

   ```sh
   go version
   git --version
   ```

2. Clone and build:

   ```sh
   git clone https://github.com/Apartment38/torii-gateway.git
   cd torii-gateway
   mkdir -p bin
   go build -trimpath -o ./bin/gateway ./cmd/gateway
   ./bin/gateway version
   ```

   A source build reports `dev` unless release metadata is supplied. SQLite and
   the console are embedded. No separate frontend build is required for a normal
   build; contributors editing the console should read [Contributing](../development/contributing.md).

3. Continue with [Native installation](native.md) to create your configuration,
   master key, and administrator. A successful build does not configure a provider.

On Windows PowerShell, replace the build commands with:

```powershell
New-Item -ItemType Directory -Force bin | Out-Null
go build -trimpath -o ./bin/gateway.exe ./cmd/gateway
./bin/gateway.exe version
```

For a release source version, select its actual tag before building, for example
`git checkout vX.Y.Z` after replacing the placeholder with an existing tag.
A static cross-build uses `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build
-trimpath -o ./bin/gateway-linux-arm64 ./cmd/gateway`. Run the result on that target
platform, not on a host with a different architecture.

If compilation fails, confirm your Go version and module-download connectivity.
For reusable developer checks, see [Contributing](../development/contributing.md).
