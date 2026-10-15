package network

import "slices"

// Challenge metadata comes from the exact domain check that rejected the
// command. It cannot grant authority or change the effective scope intersection.
type scopeDenial struct {
	cause        problem
	alternatives [][]string
	granted      []string
}

func (e scopeDenial) Error() string                 { return e.cause.Error() }
func (e scopeDenial) Unwrap() error                 { return e.cause }
func insufficientScope(a actor, scope string) error { return insufficientAnyScope(a, scope) }
func insufficientAnyScope(a actor, scopes ...string) error {
	alternatives := make([][]string, 0, len(scopes))
	for _, scope := range scopes {
		alternatives = append(alternatives, []string{scope})
	}
	return scopeDenial{cause: problem{403, "insufficient_scope"}, alternatives: alternatives, granted: append([]string(nil), a.grantedScopes...)}
}
func scopesInclude(granted, required []string) bool {
	for _, scope := range required {
		if !slices.Contains(granted, scope) {
			return false
		}
	}
	return true
}
func (e scopeDenial) challengeScopes(token []string) []string {
	for _, alternative := range e.alternatives {
		if scopesInclude(token, alternative) {
			return nil
		}
	}
	for _, alternative := range e.alternatives {
		if scopesInclude(e.granted, alternative) {
			return alternative
		}
	}
	return nil
}
