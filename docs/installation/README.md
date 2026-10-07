# Install Torii Gateway

Choose how you want to run Torii. Every installation needs persistent storage,
a database master key, and a reachable model provider. The master key encrypts
saved secrets; your generated gateway API key is a separate key used by clients.

| Method | Use when | Requirements |
| --- | --- | --- |
| [Compile from source](from-source.md) | You want to build the current code | Git and Go 1.25.1+ |
| [Run natively](native.md) | You want a process on your computer | Built gateway or matching release binary; writable configuration/data directory |
| [Docker or Podman](containers.md) | You want a container deployment | Engine and Compose provider; writable config directory and persistent volume |

Source builds are the dependable path when no published release matches your
platform. Release automation targets Linux amd64/arm64, macOS amd64/arm64, and
Windows amd64; OCI image automation targets Linux amd64/arm64. This describes
build targets, not a claim that every release/platform is available or tested.

Use a local filesystem for SQLite. Keep one gateway process per data volume.
No external database, Redis, queue, frontend package installation, or Python
runtime is needed to run Torii.

The POSIX examples use Bash or a similar shell. Windows native commands are
provided in [Native installation](native.md). Provider software/accounts are
installed separately. After installation, continue with [Getting Started](../getting-started.md).

## Obtain prerequisites

Install [Go](https://go.dev/doc/install) and [Git](https://git-scm.com/downloads)
for source builds. For containers, follow [Docker installation](https://docs.docker.com/get-started/)
or [Podman installation](https://podman.io/docs/installation), then confirm your
chosen runtime's Compose command works. The key-generation POSIX examples use
OpenSSL; Windows has a built-in PowerShell alternative in the native guide.
Install [Ollama](https://ollama.com/download) separately if following the local-model
tutorial. A hosted provider can be used instead with its own account/key.
