package backup

import "testing"

func TestCheckVersionCompat(t *testing.T) {
	cases := []struct {
		dump, server string
		globals      bool
		wantErr      bool
	}{
		{"18.6", "18.6", false, false},
		{"18.6", "18.0", false, false},
		{"19.0", "18.6", false, false}, // newer tool, older server -> ok (downgrade)
		{"17.11", "18.6", false, true}, // older tool, newer server -> refuse
		{"17.11", "18.6", true, true},
		{"", "18.6", false, false}, // unknown -> skip
		{"18.6", "", false, false},
	}
	for _, c := range cases {
		err := checkVersionCompat(c.dump, c.server, c.globals)
		if c.wantErr && err == nil {
			t.Errorf("dump=%s server=%s: expected error, got nil", c.dump, c.server)
		}
		if !c.wantErr && err != nil {
			t.Errorf("dump=%s server=%s: unexpected error %v", c.dump, c.server, err)
		}
	}
}
