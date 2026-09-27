package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	gitseercrypto "github.com/ncdlabs/gitseer/internal/crypto"
)

// RotateSealedSecrets decrypts every stored secret ciphertext with oldKey and
// re-seals under newKey inside a single transaction. Fail-closed: any decrypt
// or encrypt error rolls back with no writes. Returns the number of non-empty
// ciphertext fields that were re-sealed.
func (s *Store) RotateSealedSecrets(ctx context.Context, oldKey, newKey []byte) (int, error) {
	if len(oldKey) != 32 || len(newKey) != 32 {
		return 0, fmt.Errorf("encryption keys must be 32 bytes")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	updated := 0
	for _, t := range []struct {
		table   string
		pkCols  []string
		columns []string
	}{
		{"instances", []string{"id"}, []string{"sync_token_ciphertext", "webhook_secret_ciphertext", "oauth_client_secret_ciphertext"}},
		{"user_tokens", []string{"user_id", "instance_id"}, []string{"access_token_ciphertext", "refresh_token_ciphertext"}},
		{"app_settings", []string{"id"}, []string{"gitea_token_cipher", "gitea_webhook_secret_cipher", "oauth_client_secret_cipher"}},
		{"notification_settings", []string{"id"}, []string{"smtp_password_ciphertext", "slack_webhook_ciphertext", "discord_webhook_ciphertext", "webhook_url_ciphertext"}},
	} {
		for _, col := range t.columns {
			n, err := rotateColumn(ctx, tx, s, t.table, t.pkCols, col, oldKey, newKey)
			if err != nil {
				return 0, err
			}
			updated += n
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return updated, nil
}

func rotateColumn(ctx context.Context, tx *sql.Tx, s *Store, table string, pkCols []string, column string, oldKey, newKey []byte) (int, error) {
	selectCols := strings.Join(append(append([]string{}, pkCols...), column), ", ")
	q := s.sql(fmt.Sprintf(`SELECT %s FROM %s WHERE TRIM(COALESCE(%s, '')) != ''`, selectCols, table, column))
	rows, err := tx.QueryContext(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("list %s.%s: %w", table, column, err)
	}
	defer rows.Close()

	type row struct {
		pks []any
		ct  string
		neu string
	}
	var batch []row
	for rows.Next() {
		r := row{pks: make([]any, len(pkCols))}
		dest := make([]any, 0, len(pkCols)+1)
		pkPtrs := make([]any, len(pkCols))
		for i := range pkCols {
			var v int64
			r.pks[i] = &v
			pkPtrs[i] = &v
			dest = append(dest, pkPtrs[i])
		}
		var ct string
		dest = append(dest, &ct)
		if err := rows.Scan(dest...); err != nil {
			return 0, fmt.Errorf("scan %s.%s: %w", table, column, err)
		}
		// Materialize pk values (not pointers) for the UPDATE.
		pks := make([]any, len(pkCols))
		for i := range pkCols {
			pks[i] = *(pkPtrs[i].(*int64))
		}
		ct = strings.TrimSpace(ct)
		if ct == "" {
			continue
		}
		resealed, err := gitseercrypto.Reencrypt(oldKey, newKey, ct)
		if err != nil {
			return 0, fmt.Errorf("re-seal %s.%s: %w", table, column, err)
		}
		batch = append(batch, row{pks: pks, ct: ct, neu: resealed})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	whereParts := make([]string, len(pkCols))
	for i, c := range pkCols {
		whereParts[i] = c + " = ?"
	}
	uq := s.sql(fmt.Sprintf(`UPDATE %s SET %s = ? WHERE %s`, table, column, strings.Join(whereParts, " AND ")))
	for _, r := range batch {
		args := append([]any{r.neu}, r.pks...)
		if _, err := tx.ExecContext(ctx, uq, args...); err != nil {
			return 0, fmt.Errorf("update %s.%s: %w", table, column, err)
		}
	}
	return len(batch), nil
}
