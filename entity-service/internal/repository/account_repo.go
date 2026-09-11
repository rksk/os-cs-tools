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

package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"golang.org/x/sync/errgroup"
)

// AccountRow is the raw shape of one row read from the account table, together
// with the joined technical-owner/account-manager person refs. It is mapped to
// domain.AccountView / domain.AccountDetail by the service layer.
type AccountRow struct {
	ID                  string
	Name                string
	Classification      *string
	Pod                 *string
	SfID                *string
	Region              *string
	ActivationDate      *time.Time
	DeactivationDate    *time.Time
	TechnicalOwnerID    *string
	TechnicalOwnerName  *string
	TechnicalOwnerEmail *string
	AccountManagerID    *string
	AccountManagerName  *string
	AccountManagerEmail *string
	HasAgent            *bool
	HasKbReferences     *bool
	CreatedOn           time.Time
	CreatedBy           string
	UpdatedOn           time.Time
}

// AccountRepository defines the persistence operations for the account table.
type AccountRepository interface {
	// SearchAccounts returns a filtered, paginated slice of accounts together
	// with the total count of matching rows before pagination.
	// COUNT and SELECT are executed concurrently on separate pool connections.
	SearchAccounts(ctx context.Context, req domain.SearchAccountsRequest) ([]AccountRow, int, error)
	// GetAccountByID returns the account with the given UUID, or a NotFoundError
	// if no such account exists.
	GetAccountByID(ctx context.Context, id string) (AccountRow, error)
}

type accountRepo struct {
	db *pgxpool.Pool
}

// NewAccountRepository constructs an AccountRepository backed by the given connection pool.
func NewAccountRepository(db *pgxpool.Pool) AccountRepository {
	return &accountRepo{db: db}
}

const accountSelectColumns = `
	a.id, a.name, a.classification, a.global_pod, a.sf_id, a.region,
	a.activation_date, a.deactivation_date,
	tow.id, COALESCE(tow.name, NULLIF(TRIM(CONCAT_WS(' ', tow.first_name, tow.last_name)), '')), tow.email,
	mgr.id, COALESCE(mgr.name, NULLIF(TRIM(CONCAT_WS(' ', mgr.first_name, mgr.last_name)), '')), mgr.email,
	a.ai_gen_response_enabled, a.smart_knowledge_base_suggestions_enabled,
	a.created_on, a.created_by, a.updated_on`

const accountFromJoins = `
	FROM account a
	LEFT JOIN "user" tow ON tow.id = a.technical_owner_id
	LEFT JOIN "user" mgr ON mgr.id = a.account_manager_id`

func scanAccountRow(row interface{ Scan(...any) error }) (AccountRow, error) {
	var a AccountRow
	err := row.Scan(
		&a.ID, &a.Name, &a.Classification, &a.Pod, &a.SfID, &a.Region,
		&a.ActivationDate, &a.DeactivationDate,
		&a.TechnicalOwnerID, &a.TechnicalOwnerName, &a.TechnicalOwnerEmail,
		&a.AccountManagerID, &a.AccountManagerName, &a.AccountManagerEmail,
		&a.HasAgent, &a.HasKbReferences,
		&a.CreatedOn, &a.CreatedBy, &a.UpdatedOn,
	)
	return a, err
}

// SearchAccounts implements AccountRepository.
func (r *accountRepo) SearchAccounts(ctx context.Context, req domain.SearchAccountsRequest) ([]AccountRow, int, error) {
	filterArgs := []any{}
	argIdx := 1

	where := "WHERE 1=1"

	if req.Filters.SearchQuery != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(req.Filters.SearchQuery)
		pattern := "%" + escaped + "%"
		where += fmt.Sprintf(" AND (a.name ILIKE $%d ESCAPE '\\' OR a.sf_id ILIKE $%d ESCAPE '\\')", argIdx, argIdx)
		filterArgs = append(filterArgs, pattern)
		argIdx++
	}
	if req.Filters.Pod != "" {
		where += fmt.Sprintf(" AND a.global_pod = $%d", argIdx)
		filterArgs = append(filterArgs, req.Filters.Pod)
		argIdx++
	}
	if req.Filters.Classification != "" {
		where += fmt.Sprintf(" AND a.classification = $%d", argIdx)
		filterArgs = append(filterArgs, req.Filters.Classification)
		argIdx++
	}
	if req.Filters.Active != nil {
		if *req.Filters.Active {
			where += " AND a.deactivation_date IS NULL"
		} else {
			where += " AND a.deactivation_date IS NOT NULL"
		}
	}

	countQuery := "SELECT COUNT(*) " + accountFromJoins + " " + where

	dataQuery := fmt.Sprintf(
		"SELECT %s %s %s ORDER BY a.created_on DESC, a.id LIMIT $%d OFFSET $%d",
		accountSelectColumns, accountFromJoins, where, argIdx, argIdx+1,
	)
	dataArgs := append(append([]any{}, filterArgs...), req.Pagination.Limit, req.Pagination.Offset)

	// Run COUNT and SELECT in parallel goroutines — each uses its own pool connection.
	var total int
	var accounts []AccountRow

	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		if err := r.db.QueryRow(egCtx, countQuery, filterArgs...).Scan(&total); err != nil {
			return fmt.Errorf("count accounts: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		rows, err := r.db.Query(egCtx, dataQuery, dataArgs...)
		if err != nil {
			return fmt.Errorf("query accounts: %w", err)
		}
		defer rows.Close()

		result := make([]AccountRow, 0, req.Pagination.Limit)
		for rows.Next() {
			a, err := scanAccountRow(rows)
			if err != nil {
				return fmt.Errorf("scan account: %w", err)
			}
			result = append(result, a)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate accounts: %w", err)
		}
		accounts = result
		return nil
	})

	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}

	return accounts, total, nil
}

// GetAccountByID implements AccountRepository.
func (r *accountRepo) GetAccountByID(ctx context.Context, id string) (AccountRow, error) {
	query := "SELECT " + accountSelectColumns + " " + accountFromJoins + " WHERE a.id = $1"
	a, err := scanAccountRow(r.db.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountRow{}, &apierror.NotFoundError{Msg: "account not found"}
	}
	if err != nil {
		return AccountRow{}, fmt.Errorf("get account by id: %w", err)
	}
	return a, nil
}
