package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
)

// FindOrCreateByWeChat finds a user by openid, or creates one if not found.
// Returns the username (userID), whether the user is newly created, and an error.
func (s *UserStore) FindOrCreateByWeChat(ctx context.Context, openid, unionid string) (userID string, isNew bool, err error) {
	// Try to find by openid first
	var username string
	err = s.db.QueryRow(ctx,
		"SELECT username FROM users WHERE wechat_openid = ?", openid,
	).Scan(&username)
	if err == nil {
		// User exists — update unionid if we got one and it wasn't set before
		if unionid != "" {
			var existingUnionid sql.NullString
			_ = s.db.QueryRow(ctx,
				"SELECT wechat_unionid FROM users WHERE wechat_openid = ?", openid,
			).Scan(&existingUnionid)
			if !existingUnionid.Valid || existingUnionid.String == "" {
				_, _ = s.db.Exec(ctx,
					"UPDATE users SET wechat_unionid = ? WHERE wechat_openid = ?",
					unionid, openid,
				)
			}
		}
		return username, false, nil
	}
	if err != sql.ErrNoRows {
		return "", false, fmt.Errorf("failed to query user by openid: %w", err)
	}

	// User not found — create a new one with auto-generated username
	generatedName := "wx_" + randomHex(8)
	_, err = s.db.Exec(ctx,
		`INSERT INTO users (username, password_hash, wechat_openid, wechat_unionid, auth_provider)
		 VALUES (?, '', ?, ?, 'wechat')`,
		generatedName, openid, unionid,
	)
	if err != nil {
		// Race: another request created the user between our SELECT and INSERT
		if isDuplicateEntry(err) {
			err = s.db.QueryRow(ctx,
				"SELECT username FROM users WHERE wechat_openid = ?", openid,
			).Scan(&username)
			if err != nil {
				return "", false, fmt.Errorf("failed to query user after race: %w", err)
			}
			return username, false, nil
		}
		return "", false, fmt.Errorf("failed to create wechat user: %w", err)
	}

	return generatedName, true, nil
}

// FindByOpenID looks up a user by WeChat openid.
func (s *UserStore) FindByOpenID(ctx context.Context, openid string) (userID string, err error) {
	err = s.db.QueryRow(ctx,
		"SELECT username FROM users WHERE wechat_openid = ?", openid,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to query user by openid: %w", err)
	}
	return userID, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}
