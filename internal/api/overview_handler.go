package api

import "net/http"

type instanceSummary struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Connected        bool   `json:"connected"`
	Version          string `json:"version"`
	DatabaseCount    int    `json:"database_count"`
	AccountCount     int    `json:"account_count"`
	ProtectedCount   int    `json:"protected_count"`
	UnprotectedCount int    `json:"unprotected_count"`
	LastBackupAt     string `json:"last_backup_at"`
	LastBackupStatus string `json:"last_backup_status"`
	RecentFailed     int    `json:"recent_failed"`
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	insts, err := s.instances.List(r.Context())
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	policies, err := s.policies.List(r.Context())
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}

	out := make([]instanceSummary, 0, len(insts))
	for _, inst := range insts {
		sum := instanceSummary{ID: inst.ID, Name: inst.Name}

		params, err := s.instances.ConnParams(r.Context(), inst.ID)
		if err == nil {
			res, _ := s.pg.TestConnection(r.Context(), params)
			if res != nil && res.OK {
				sum.Connected = true
				sum.Version = res.Version
				sum.DatabaseCount = res.DatabaseCount
				accounts, aerr := s.pg.ListAccounts(r.Context(), params)
				if aerr == nil {
					sum.AccountCount = len(accounts)
				}
			}
		}

		// protection = distinct databases referenced by enabled policies
		seen := map[string]bool{}
		for _, p := range policies {
			if p.InstanceID == inst.ID && p.Enabled && p.DatabaseName != "" {
				seen[p.DatabaseName] = true
			}
		}
		sum.ProtectedCount = len(seen)
		unprotected := sum.DatabaseCount - sum.ProtectedCount
		if unprotected < 0 {
			unprotected = 0
		}
		sum.UnprotectedCount = unprotected

		// recent backups
		recs, _ := s.backups.ListByInstance(r.Context(), inst.ID, "", 20)
		if len(recs) > 0 {
			for _, rec := range recs {
				if rec.BackupType == "database" && sum.LastBackupAt == "" {
					sum.LastBackupAt = rec.FinishedAt
					sum.LastBackupStatus = rec.Status
				}
				if rec.Status == "failed" {
					sum.RecentFailed++
				}
			}
		}
		out = append(out, sum)
	}
	writeOK(w, out)
}
