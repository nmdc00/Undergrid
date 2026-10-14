package network

import _ "embed"

// The immutable canonical network contract is public documentation, not an
// authorization adapter. It must remain available without a live database.
//
//go:embed api/openapi.json
var networkOpenAPI []byte

// NetworkOpenAPI returns a copy of the canonical public document for protocol
// adapters which need the same wire schema. Schema access grants no authority.
func NetworkOpenAPI() []byte {
	return append([]byte(nil), networkOpenAPI...)
}
