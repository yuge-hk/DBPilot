package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/dbpilot/dbpilot/internal/provider"
)

// HbaRule is a parsed pg_hba.conf entry (host rules are the firewall-relevant
// remote-access rules).
type HbaRule struct {
	Type     string `json:"type"`
	Database string `json:"database"`
	User     string `json:"user"`
	Address  string `json:"address"`
	Method   string `json:"method"`
	Raw      string `json:"raw"`
}

type firewallState struct {
	ListenAddresses string    `json:"listen_addresses"`
	HbaFile         string    `json:"hba_file"`
	ConfigFile      string    `json:"config_file"`
	Rules           []HbaRule `json:"rules"`
	Writable        bool      `json:"writable"`
	Error           string    `json:"error,omitempty"`
}

func (s *Server) handleGetFirewall(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	params, apiErr := s.connParams(r, id)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	cfg, err := s.pg.ServerConfig(r.Context(), params)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}

	state := firewallState{
		ListenAddresses: cfg.ListenAddresses,
		HbaFile:         cfg.HbaFile,
		ConfigFile:      cfg.ConfigFile,
		Rules:           []HbaRule{},
	}

	content, err := s.readHbaFile(r.Context(), cfg.HbaFile)
	if err != nil {
		state.Writable = false
		state.Error = err.Error()
		writeOK(w, state)
		return
	}
	state.Writable = true
	state.Rules = parseHba(content)
	writeOK(w, state)
}

type addFirewallRuleRequest struct {
	Database string `json:"database"`
	User     string `json:"user"`
	Address  string `json:"address"`
	Method   string `json:"method"`
}

func (s *Server) handleAddFirewallRule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	var req addFirewallRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if err := normalizeHbaRule(&req.Database, &req.User, req.Address, &req.Method); err != nil {
		writeError(w, ErrBadRequest("invalid_request", err.Error()))
		return
	}

	params, apiErr := s.connParams(r, id)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if err := s.addFirewallRule(r.Context(), id, params, req); err != nil {
		writeError(w, NewError(http.StatusConflict, "firewall_failed", err.Error()))
		return
	}
	s.auditRecord(r, "add_firewall_rule", "instance", params.Host, "success", "address="+req.Address)
	writeOK(w, map[string]string{"status": "ok"})
}

func normalizeHbaRule(database, user *string, address string, method *string) error {
	if *database == "" {
		*database = "all"
	}
	if *user == "" {
		*user = "all"
	}
	if *method == "" {
		*method = "scram-sha-256"
	}
	if address == "" {
		return errors.New("address is required")
	}
	if !validHbaAddress(address) {
		return errors.New("address must be an IP address, CIDR, all, samehost or samenet")
	}
	if !validHbaToken(*database) {
		return errors.New("invalid database name")
	}
	if !validHbaToken(*user) {
		return errors.New("invalid user name")
	}
	if !validHbaMethod(*method) {
		return errors.New("invalid authentication method")
	}
	return nil
}

func validHbaAddress(value string) bool {
	if value == "" || len(value) > 128 || strings.ContainsAny(value, " \t\r\n#") {
		return false
	}
	if value == "all" || value == "samehost" || value == "samenet" {
		return true
	}
	if net.ParseIP(value) != nil {
		return true
	}
	_, _, err := net.ParseCIDR(value)
	return err == nil
}

func validHbaToken(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '_' || r == '-' || r == '.' || r == ',' {
			continue
		}
		return false
	}
	return true
}

func validHbaMethod(value string) bool {
	switch value {
	case "trust", "reject", "scram-sha-256", "md5", "password", "cert":
		return true
	default:
		return false
	}
}

func (s *Server) addFirewallRule(ctx context.Context, instanceID int64, params provider.ConnParams, req addFirewallRuleRequest) error {
	cfg, err := s.pg.ServerConfig(ctx, params)
	if err != nil {
		return err
	}
	rule := "host\t" + req.Database + "\t" + req.User + "\t" + req.Address + "\t" + req.Method
	if err := s.appendHbaLine(ctx, cfg.HbaFile, rule); err != nil {
		return err
	}
	return s.reloadPostgres(ctx, s.instanceSystemdUnit(ctx, instanceID))
}

func (s *Server) instanceSystemdUnit(ctx context.Context, instanceID int64) string {
	if s.instances == nil {
		return ""
	}
	inst, err := s.instances.Get(ctx, instanceID)
	if err != nil {
		return ""
	}
	return inst.SystemdUnit
}

func listenAddressesAllowRemote(value string) bool {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || part == "0.0.0.0" || part == "::" {
			return true
		}
		if ip := net.ParseIP(part); ip != nil && !ip.IsLoopback() {
			return true
		}
	}
	return false
}

func (s *Server) remoteAccessWarning(ctx context.Context, params provider.ConnParams) string {
	cfg, err := s.pg.ServerConfig(ctx, params)
	if err != nil {
		return "cannot read PostgreSQL configuration: " + err.Error()
	}
	if !listenAddressesAllowRemote(cfg.ListenAddresses) {
		return fmt.Sprintf("listen_addresses is local-only (%q); edit postgresql.conf and restart PostgreSQL", cfg.ListenAddresses)
	}
	return ""
}

type removeFirewallRuleRequest struct {
	Address  string `json:"address"`
	Database string `json:"database"`
	User     string `json:"user"`
}

func (s *Server) handleRemoveFirewallRule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	var req removeFirewallRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrBadRequest("invalid_request", "invalid request body"))
		return
	}
	if !validHbaAddress(req.Address) {
		writeError(w, ErrBadRequest("invalid_request", "invalid address"))
		return
	}
	if req.Database != "" && !validHbaToken(req.Database) {
		writeError(w, ErrBadRequest("invalid_request", "invalid database name"))
		return
	}
	if req.User != "" && !validHbaToken(req.User) {
		writeError(w, ErrBadRequest("invalid_request", "invalid user name"))
		return
	}

	params, apiErr := s.connParams(r, id)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	cfg, err := s.pg.ServerConfig(r.Context(), params)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}

	content, err := s.readHbaFile(r.Context(), cfg.HbaFile)
	if err != nil {
		writeError(w, NewError(http.StatusConflict, "firewall_failed", err.Error()))
		return
	}

	updated := removeHbaRule(content, req.Address, req.Database, req.User)
	if err := s.writeHbaFile(r.Context(), cfg.HbaFile, updated); err != nil {
		writeError(w, NewError(http.StatusConflict, "firewall_failed", err.Error()))
		return
	}
	if err := s.reloadPostgres(r.Context(), s.instanceSystemdUnit(r.Context(), id)); err != nil {
		writeError(w, NewError(http.StatusConflict, "firewall_reload_failed", err.Error()))
		return
	}
	s.auditRecord(r, "remove_firewall_rule", "instance", params.Host, "success", "address="+req.Address)
	writeOK(w, map[string]string{"status": "ok"})
}

func (s *Server) readHbaFile(ctx context.Context, path string) (string, error) {
	if s.runner == nil {
		return "", errors.New("command runner unavailable")
	}
	stdout, stderr, err := s.runner.Run(ctx, "sudo", []string{"-n", "cat", path}, nil)
	if err != nil {
		return "", errors.New("cannot read pg_hba.conf (dbpilot needs sudo access): " + strings.TrimSpace(string(stderr)))
	}
	return string(stdout), nil
}

func (s *Server) appendHbaLine(ctx context.Context, path, line string) error {
	content, err := s.readHbaFile(ctx, path)
	if err != nil {
		return err
	}
	target := strings.Join(strings.Fields(line), " ")
	for _, existing := range strings.Split(content, "\n") {
		if strings.Join(strings.Fields(existing), " ") == target {
			return nil
		}
	}
	return s.writeHbaFile(ctx, path, content+"\n# DBPilot\n"+line+"\n")
}

func (s *Server) writeHbaFile(ctx context.Context, path, content string) error {
	tmp, err := os.CreateTemp("", "dbpilot-hba-*.conf")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	// backup then install
	if _, _, err := s.runner.Run(ctx, "sudo", []string{"-n", "cp", path, path + ".dbpilot.bak"}, nil); err != nil {
		return errors.New("cannot backup pg_hba.conf")
	}
	if _, stderr, err := s.runner.Run(ctx, "sudo", []string{"-n", "cp", tmpPath, path}, nil); err != nil {
		return errors.New("cannot write pg_hba.conf: " + strings.TrimSpace(string(stderr)))
	}
	return nil
}

func (s *Server) reloadPostgres(ctx context.Context, unit string) error {
	if s.runner == nil {
		return errors.New("command runner unavailable")
	}
	if unit == "" {
		unit = "postgresql"
	}
	if _, stderr, err := s.runner.Run(ctx, "sudo", []string{"-n", "systemctl", "reload", unit}, nil); err != nil {
		return errors.New("cannot reload PostgreSQL: " + strings.TrimSpace(string(stderr)))
	}
	return nil
}

func parseHba(content string) []HbaRule {
	var rules []HbaRule
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 3 {
			continue
		}
		typ := fields[0]
		rule := HbaRule{Type: typ, Database: fields[1], User: fields[2], Raw: trimmed}
		switch typ {
		case "host", "hostssl", "hostnossl":
			if len(fields) >= 5 {
				rule.Address = fields[3]
				rule.Method = fields[4]
			}
		case "local":
			if len(fields) >= 4 {
				rule.Method = fields[3]
			}
		}
		rules = append(rules, rule)
	}
	return rules
}

func removeHbaRule(content, address, database, user string) string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)
		// match host rules by address (and optionally db/user)
		if len(fields) >= 5 && (fields[0] == "host" || fields[0] == "hostssl" || fields[0] == "hostnossl") {
			if fields[3] == address {
				if database == "" || database == "all" || fields[1] == database {
					if user == "" || user == "all" || fields[2] == user {
						continue // drop this rule
					}
				}
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
