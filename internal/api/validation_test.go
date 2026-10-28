package api

import "testing"

func TestValidateIdentifier(t *testing.T) {
	valid := []string{"mydb", "my_db", "_private", "a1"}
	for _, v := range valid {
		if err := validateIdentifier(v, "database"); err != nil {
			t.Errorf("expected %q valid, got %v", v, err)
		}
	}

	invalid := []string{"", "MyDB", "1db", "has space", "with-dash", "with.dot", "a'b", "x;drop"}
	for _, v := range invalid {
		if err := validateIdentifier(v, "database"); err == nil {
			t.Errorf("expected %q invalid", v)
		}
	}

	long := make([]byte, 64)
	for i := range long {
		long[i] = 'a'
	}
	if err := validateIdentifier(string(long), "database"); err == nil {
		t.Error("expected over-length identifier invalid")
	}
}

func TestValidConnectionHost(t *testing.T) {
	valid := []string{"", "127.0.0.1", "192.168.1.10", "db.example.com", "2001:db8::1", "my_host"}
	for _, v := range valid {
		if !validConnectionHost(v) {
			t.Errorf("expected connection host %q valid", v)
		}
	}

	invalid := []string{"http://example.com", "192.168.1.0/24", "host name", "user@host", "a\nb", "..", "-bad.example.com", "bad-.example.com"}
	for _, v := range invalid {
		if validConnectionHost(v) {
			t.Errorf("expected connection host %q invalid", v)
		}
	}
}
