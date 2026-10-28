package api

import "testing"

func TestValidHbaAddress(t *testing.T) {
	valid := []string{"127.0.0.1", "192.168.1.0/24", "::1", "2001:db8::/32", "all", "samehost", "samenet"}
	for _, addr := range valid {
		if !validHbaAddress(addr) {
			t.Errorf("expected %q to be valid", addr)
		}
	}
	invalid := []string{"", "localhost", "0.0.0.0/0\nlocal all all trust", "192.168.1.0/24 work", "1.2.3.4#x"}
	for _, addr := range invalid {
		if validHbaAddress(addr) {
			t.Errorf("expected %q to be invalid", addr)
		}
	}
}

func TestNormalizeHbaRuleRejectsInjection(t *testing.T) {
	database := "app"
	user := "app_user\ntrust"
	method := "scram-sha-256"
	if err := normalizeHbaRule(&database, &user, "192.168.1.0/24", &method); err == nil {
		t.Error("expected newline in user to be rejected")
	}

	database = "app\ntrust"
	user = "app_user"
	if err := normalizeHbaRule(&database, &user, "192.168.1.0/24", &method); err == nil {
		t.Error("expected newline in database to be rejected")
	}

	database = "app"
	user = "app_user"
	if err := normalizeHbaRule(&database, &user, "192.168.1.0/24", &method); err != nil {
		t.Errorf("valid rule rejected: %v", err)
	}
}

func TestListenAddressesAllowRemote(t *testing.T) {
	valid := []string{"*", "0.0.0.0", "::", "localhost,192.168.1.10", "127.0.0.1,10.0.0.5"}
	for _, value := range valid {
		if !listenAddressesAllowRemote(value) {
			t.Errorf("expected %q to allow remote connections", value)
		}
	}
	invalid := []string{"", "localhost", "127.0.0.1", "::1"}
	for _, value := range invalid {
		if listenAddressesAllowRemote(value) {
			t.Errorf("expected %q to be local-only", value)
		}
	}
}
