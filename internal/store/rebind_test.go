package store

import "testing"

func TestRebindPostgres(t *testing.T) {
	got := rebindPostgres(`SELECT a FROM t WHERE x = ? AND y = ?`)
	if got != `SELECT a FROM t WHERE x = $1 AND y = $2` {
		t.Fatalf("got %q", got)
	}
}
