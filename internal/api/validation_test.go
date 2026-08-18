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
