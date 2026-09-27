package store

import (
	"time"

	postgressqlc "github.com/ncdlabs/gitseer/internal/store/sqlc/postgres"
	sqlitesqlc "github.com/ncdlabs/gitseer/internal/store/sqlc/sqlite"
)

func (s *Store) initSQLC() {
	if s.db == nil {
		return
	}
	if s.driver == "postgres" {
		s.pg = postgressqlc.New(s.db)
		return
	}
	s.sqlite = sqlitesqlc.New(s.db)
}

func parseOptionalTimePtr(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := parseTime(*s)
	if err != nil {
		return nil
	}
	return &t
}

func parseRequiredTime(s string) time.Time {
	t, _ := parseTime(s)
	return t
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
