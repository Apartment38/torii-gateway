package assets

import "embed"

// Files contains the brand assets used by the embedded web console.
//
//go:embed torii-gateway-logo.png torii-gateway-icon.png
var Files embed.FS
