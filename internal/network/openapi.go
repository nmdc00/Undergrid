package network

import _ "embed"

// The immutable canonical network contract is public documentation, not an
// authorization adapter. It must remain available without a live database.
//
//go:embed api/openapi.json
var networkOpenAPI []byte
