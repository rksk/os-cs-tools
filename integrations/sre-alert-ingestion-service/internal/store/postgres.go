// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // Postgres driver registered under the "pgx" database/sql driver name
)

// PostgresStore is the Store implementation backing production use: a
// dedicated Postgres database via database/sql + the pgx driver
// (github.com/jackc/pgx/v5/stdlib), matching the driver choice already
// established in this repo by entity-service and
// integrations/sftpgo-authentication-service (both jackc/pgx/v5) — chosen
// over lib/pq for consistency with that existing convention, not
// re-evaluated independently here.
type PostgresStore struct {
	db *sql.DB
}

var _ Store = (*PostgresStore)(nil)

// NewPostgresStore opens a connection pool against dsn (a standard
// postgres:// connection string) and verifies it's reachable. The caller
// owns applying migrations/0001_create_alert_buffer.up.sql before first use
// — this constructor does not run migrations itself (matching
// sftpgo-authentication-service's convention: migrations are applied via
// `psql -f`, not from application code).
func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open database: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	return &PostgresStore{db: db}, nil
}

// Close closes the underlying connection pool.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *PostgresStore) Enqueue(ctx context.Context, payload []byte) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO alert_buffer (payload) VALUES ($1) RETURNING id`,
		payload,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("store: enqueue: %w", err)
	}
	return id, nil
}

func (s *PostgresStore) PendingBatch(ctx context.Context, limit int) ([]AlertRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, received_at, payload, status, retry_count, last_attempt_at, last_error, incident_id, escalated_at
		 FROM alert_buffer
		 WHERE status = $1
		 ORDER BY received_at ASC
		 LIMIT $2`,
		StatusPending, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("store: pending batch: %w", err)
	}
	defer rows.Close()

	var out []AlertRecord
	for rows.Next() {
		var rec AlertRecord
		var lastAttemptAt, escalatedAt sql.NullTime
		var lastError, incidentID sql.NullString
		if err := rows.Scan(&rec.ID, &rec.ReceivedAt, &rec.Payload, &rec.Status, &rec.RetryCount,
			&lastAttemptAt, &lastError, &incidentID, &escalatedAt); err != nil {
			return nil, fmt.Errorf("store: scan pending row: %w", err)
		}
		if lastAttemptAt.Valid {
			t := lastAttemptAt.Time
			rec.LastAttemptAt = &t
		}
		if escalatedAt.Valid {
			t := escalatedAt.Time
			rec.EscalatedAt = &t
		}
		rec.LastError = lastError.String
		rec.IncidentID = incidentID.String
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate pending rows: %w", err)
	}
	return out, nil
}

func (s *PostgresStore) MarkDelivered(ctx context.Context, id, incidentID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alert_buffer
		 SET status = $2, incident_id = $3, last_attempt_at = now(), last_error = NULL
		 WHERE id = $1`,
		id, StatusDelivered, incidentID,
	)
	if err != nil {
		return fmt.Errorf("store: mark delivered: %w", err)
	}
	return nil
}

func (s *PostgresStore) MarkAttemptFailed(ctx context.Context, id, lastError string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alert_buffer
		 SET retry_count = retry_count + 1, last_attempt_at = now(), last_error = $2
		 WHERE id = $1 AND status = $3`,
		id, lastError, StatusPending,
	)
	if err != nil {
		return fmt.Errorf("store: mark attempt failed: %w", err)
	}
	return nil
}

func (s *PostgresStore) MarkEscalated(ctx context.Context, id, lastError string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alert_buffer
		 SET status = $2, retry_count = retry_count + 1, last_attempt_at = now(),
		     last_error = $3, escalated_at = now()
		 WHERE id = $1`,
		id, StatusEscalated, lastError,
	)
	if err != nil {
		return fmt.Errorf("store: mark escalated: %w", err)
	}
	return nil
}

func (s *PostgresStore) MarkFailed(ctx context.Context, id, lastError string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alert_buffer
		 SET status = $2, last_attempt_at = now(), last_error = $3
		 WHERE id = $1`,
		id, StatusFailed, lastError,
	)
	if err != nil {
		return fmt.Errorf("store: mark failed: %w", err)
	}
	return nil
}
