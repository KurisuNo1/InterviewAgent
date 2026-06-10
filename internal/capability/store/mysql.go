package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// MySQLClient wraps MySQL database operations.
type MySQLClient interface {
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	Query(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context) (*sql.Tx, error)
}

// MySQLStore implements MySQLClient using go-sql-driver/mysql.
type MySQLStore struct {
	db *sql.DB
}

// MySQLConfig holds MySQL connection configuration.
type MySQLConfig struct {
	Host            string
	Port            int
	User            string
	PasswordEnv     string
	Database        string
	Charset         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// NewMySQLStore creates a new MySQL client wrapper.
func NewMySQLStore(cfg MySQLConfig) (*MySQLStore, error) {
	password := os.Getenv(cfg.PasswordEnv)

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		cfg.User, password, cfg.Host, cfg.Port, cfg.Database, cfg.Charset)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open MySQL connection: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(5 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping MySQL: %w", err)
	}

	store := &MySQLStore{db: db}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return store, nil
}

// migrate creates the necessary tables if they don't exist.
func (m *MySQLStore) migrate(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			username VARCHAR(64) NOT NULL UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS interview_sessions (
			id VARCHAR(36) PRIMARY KEY,
			user_id VARCHAR(64) DEFAULT '',
			status VARCHAR(32) DEFAULT 'created',
			jd_text TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS interview_results (
			id VARCHAR(64) PRIMARY KEY,
			session_id VARCHAR(36) NOT NULL,
			evaluations JSON,
			overall_score DECIMAL(5,2),
			dimension_scores JSON,
			report_json JSON,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES interview_sessions(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS review_plans (
			id VARCHAR(64) PRIMARY KEY,
			session_id VARCHAR(36) NOT NULL,
			plan_json JSON,
			resources_json JSON,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES interview_sessions(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS interview_answers (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			session_id VARCHAR(36) NOT NULL,
			question_id VARCHAR(64) NOT NULL,
			question TEXT NOT NULL,
			answer TEXT,
			score DECIMAL(5,2),
			feedback TEXT,
			question_num INT NOT NULL DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES interview_sessions(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

			`CREATE TABLE IF NOT EXISTS chat_messages (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				session_id VARCHAR(36) NOT NULL,
				role VARCHAR(16) NOT NULL,
				content TEXT NOT NULL,
				created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
				INDEX idx_session (session_id, created_at),
				FOREIGN KEY (session_id) REFERENCES interview_sessions(id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}

	for _, q := range queries {
		if _, err := m.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migration failed: %w\nQuery: %s", err, q)
		}
	}

	// WeChat Mini-Program login support: alter existing users table.
	// These are idempotent — "Duplicate column" errors (1060) are ignored.
	wechatMigrations := []string{
		`ALTER TABLE users ADD COLUMN wechat_openid VARCHAR(128) DEFAULT NULL`,
		`ALTER TABLE users ADD UNIQUE INDEX idx_wechat_openid (wechat_openid)`,
		`ALTER TABLE users ADD COLUMN wechat_unionid VARCHAR(128) DEFAULT NULL`,
		`ALTER TABLE users ADD COLUMN nickname VARCHAR(64) DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN avatar_url VARCHAR(512) DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN auth_provider VARCHAR(16) DEFAULT 'password'`,
		`ALTER TABLE users ADD COLUMN updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP`,
		`ALTER TABLE users MODIFY COLUMN username VARCHAR(64) NULL`,
	}
	for _, q := range wechatMigrations {
		_, err := m.db.ExecContext(ctx, q)
		if err != nil {
			if isDuplicateColumn(err) {
				continue
			}
			return fmt.Errorf("wechat migration failed: %w\nQuery: %s", err, q)
		}
	}

	return nil
}

// isDuplicateColumn checks if a MySQL error is a duplicate column (1060) or
// duplicate index (1061) error, meaning the migration was already applied.
func isDuplicateColumn(err error) bool {
	if me, ok := err.(*mysql.MySQLError); ok {
		return me.Number == 1060 || me.Number == 1061
	}
	s := err.Error()
	return strings.Contains(s, "Duplicate column") || strings.Contains(s, "Duplicate key")
}

// Exec executes a statement.
func (m *MySQLStore) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return m.db.ExecContext(ctx, query, args...)
}

// Query executes a query returning rows.
func (m *MySQLStore) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return m.db.QueryContext(ctx, query, args...)
}

// QueryRow executes a query returning a single row.
func (m *MySQLStore) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return m.db.QueryRowContext(ctx, query, args...)
}

// BeginTx starts a transaction.
func (m *MySQLStore) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return m.db.BeginTx(ctx, &sql.TxOptions{})
}

// Close closes the database connection.
func (m *MySQLStore) Close() error {
	return m.db.Close()
}
