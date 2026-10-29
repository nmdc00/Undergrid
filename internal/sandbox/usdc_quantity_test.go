package sandbox

import "testing"

func TestUSDCQuantityRejectsNonCanonicalHex(t *testing.T) {
	for _, raw := range []string{"0x+1", "0x-0", "0x01", "0xA", "0x", "0x1_0"} {
		if n, err := chainQuantity(raw); err == nil {
			t.Errorf("accepted malformed quantity %q as %s", raw, n)
		}
	}
	for _, raw := range []string{"0x0", "0x1", "0xabcdef"} {
		if _, err := chainQuantity(raw); err != nil {
			t.Errorf("rejected canonical quantity %q: %v", raw, err)
		}
	}
}
