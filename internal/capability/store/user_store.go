package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

// isDuplicateEntry checks if a MySQL error is a duplicate key violation.
func isDuplicateEntry(err error) bool {
	if me, ok := err.(*mysql.MySQLError); ok {
		return me.Number == 1062
	}
	return strings.Contains(err.Error(), "Duplicate entry")
}

// UserStore handles user authentication with MySQL persistence.
type UserStore struct {
	db *MySQLStore
}

// NewUserStore creates a new user store.
func NewUserStore(db *MySQLStore) *UserStore {
	return &UserStore{db: db}
}

// CreateUser registers a new user with hashed password.
// Returns an error if the username already exists.
func (s *UserStore) CreateUser(ctx context.Context, username, password string) error {
	if len(username) < 3 {
		return fmt.Errorf("username must be at least 3 characters")
	}
	if len(password) < 6 {
		return fmt.Errorf("password must be at least 6 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = s.db.Exec(ctx,
		"INSERT INTO users (username, password_hash) VALUES (?, ?)",
		username, string(hash),
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return fmt.Errorf("username already exists")
		}
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

// ValidateCredentials checks username and password against the database.
// Returns (true, nil) on success, (false, nil) on wrong credentials.
func (s *UserStore) ValidateCredentials(ctx context.Context, username, password string) (bool, error) {
	var hash string
	err := s.db.QueryRow(ctx, "SELECT password_hash FROM users WHERE username = ?", username).Scan(&hash)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to query user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return false, nil
	}
	return true, nil
}

// UserExists checks if a username is already registered.
func (s *UserStore) UserExists(ctx context.Context, username string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)", username,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}
	return exists, nil
}
