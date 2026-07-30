package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"
)

type apiKeyInfo struct {
	Owner string
	Role  string
}

// generateAPIKey creates a random 32-byte key, stores only its hash, returns the raw key ONCE.
// requesterKey must belong to an active admin, UNLESS this is the very first key ever created (bootstrap).
func generateAPIKey(db *sql.DB, owner string, role string, requesterKey string) (string, error) {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM api_keys").Scan(&count); err != nil {
		return "", err
	}

	if count == 0 {
		role = "admin" // first key is always admin — there's no one else to grant permissions yet
	} else {
		if requesterKey == "" {
			return "", fmt.Errorf("an existing admin key is required to generate new keys (use -admin-key)")
		}
		info, err := verifyAPIKey(db, requesterKey)
		if err != nil {
			return "", fmt.Errorf("invalid admin key")
		}
		if info.Role != "admin" {
			return "", fmt.Errorf("only admin keys can generate new keys")
		}
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	key := hex.EncodeToString(raw)

	hash := sha256.Sum256([]byte(key))
	hashHex := hex.EncodeToString(hash[:])

	_, err := db.Exec(
		"INSERT INTO api_keys (key_hash, owner, role, created_at, active) VALUES ($1, $2, $3, $4, true)",
		hashHex, owner, role, time.Now().Unix(),
	)
	if err != nil {
		return "", err
	}
	return key, nil
}

// verifyAPIKey hashes the incoming key and looks up a matching, active row.
func verifyAPIKey(db *sql.DB, rawKey string) (*apiKeyInfo, error) {
	hash := sha256.Sum256([]byte(rawKey))
	hashHex := hex.EncodeToString(hash[:])

	row := db.QueryRow("SELECT owner, role FROM api_keys WHERE key_hash = $1 AND active = true", hashHex)
	var info apiKeyInfo
	if err := row.Scan(&info.Owner, &info.Role); err != nil {
		return nil, err
	}
	return &info, nil
}

// role ranking: higher number = more access
var roleRank = map[string]int{"viewer": 1, "admin": 2}

func roleAllows(have string, need string) bool {
	return roleRank[have] >= roleRank[need]
}

// requireRole wraps a handler, requiring a valid key with at least the given role.
func requireRole(db *sql.DB, minRole string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("X-API-Key")
		if provided == "" {
			http.Error(w, "missing API key", http.StatusUnauthorized)
			return
		}

		info, err := verifyAPIKey(db, provided)
		if err != nil {
			http.Error(w, "invalid API key", http.StatusUnauthorized)
			return
		}

		if !roleAllows(info.Role, minRole) {
			http.Error(w, "insufficient permissions", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}
