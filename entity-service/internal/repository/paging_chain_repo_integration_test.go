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

// The paging chain's role changes, against a live PostgreSQL built from every
// migration. Skipped unless ENTITY_TEST_DATABASE_URL is set:
//
//	ENTITY_TEST_DATABASE_URL="postgres:///entity_test" go test -v -run TestPagingChainIntegration ./internal/repository/

package repository

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	pcTeamID  = "9a6e0000-0000-4000-8000-000000000001"
	pcTeamKey = "pagingrolefixture"
	pcUserA   = "9a6e0000-0000-4000-8000-000000000002"
	pcUserB   = "9a6e0000-0000-4000-8000-000000000003"
	pcMemberA = "9a6e0000-0000-4000-8000-000000000004"
	pcMemberB = "9a6e0000-0000-4000-8000-000000000005"
	pcActor   = "paging.fixture@example.test"
)

func newPagingChainIntegrationRepo(t *testing.T) (PagingChainRepository, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("ENTITY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ENTITY_TEST_DATABASE_URL is not set; skipping the live-database tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	clean := func() {
		mustExec(t, pool, `DELETE FROM team_member WHERE team_id = $1`, pcTeamID)
		mustExec(t, pool, `DELETE FROM team WHERE id = $1`, pcTeamID)
		mustExec(t, pool, `DELETE FROM "user" WHERE id IN ($1, $2)`, pcUserA, pcUserB)
	}
	clean()
	t.Cleanup(clean)
	mustExec(t, pool, `
		INSERT INTO team (id, created_on, updated_on, created_by, updated_by, name, key, type)
		VALUES ($1, NOW(), NOW(), 'fixture', 'fixture', $2, $2, 'cre')`, pcTeamID, pcTeamKey)
	for _, u := range [][2]string{{pcUserA, "paging.a@example.test"}, {pcUserB, "paging.b@example.test"}} {
		mustExec(t, pool, `
			INSERT INTO "user" (id, created_on, updated_on, created_by, updated_by, user_name, first_name, last_name, email)
			VALUES ($1, NOW(), NOW(), 'fixture', 'fixture', $2, 'Paging', 'Fixture', $2)`, u[0], u[1])
	}
	for _, m := range [][3]string{{pcMemberA, pcUserA, "lead"}, {pcMemberB, pcUserB, "engineer"}} {
		mustExec(t, pool, `
			INSERT INTO team_member (id, created_on, updated_on, created_by, updated_by, team_id, user_id, role)
			VALUES ($1, NOW(), NOW(), 'fixture', 'fixture', $2, $3, $4)`, m[0], pcTeamID, m[1], m[2])
	}
	return NewPagingChainRepository(pool), pool
}

func pcRoleOf(t *testing.T, pool *pgxpool.Pool, membershipID string) string {
	t.Helper()
	var role string
	if err := pool.QueryRow(context.Background(), `SELECT role FROM team_member WHERE id = $1`, membershipID).Scan(&role); err != nil {
		t.Fatalf("read role: %v", err)
	}
	return role
}

// Appointing a team's America lead, then another, then stepping one down: the
// writes Case Alerts makes when someone sets those positions. Each used to fail
// with "inconsistent types deduced for parameter $2", as the role was both the
// column's new value and a value compared as text in the same statement.
func TestPagingChainIntegration_SetRoleAppointsAndStepsDown(t *testing.T) {
	repo, pool := newPagingChainIntegrationRepo(t)
	ctx := context.Background()

	if err := repo.SetRole(ctx, pcActor, pcMemberB, "americas_team_lead"); err != nil {
		t.Fatalf("appoint B as America lead: %v", err)
	}
	if got := pcRoleOf(t, pool, pcMemberB); got != "americas_team_lead" {
		t.Fatalf("B is %q, want americas_team_lead", got)
	}

	// One America lead per team: appointing A steps B back to a Team lead.
	if err := repo.SetRole(ctx, pcActor, pcMemberA, "americas_team_lead"); err != nil {
		t.Fatalf("appoint A as America lead: %v", err)
	}
	if a, b := pcRoleOf(t, pool, pcMemberA), pcRoleOf(t, pool, pcMemberB); a != "americas_team_lead" || b != "lead" {
		t.Fatalf("A is %q and B is %q, want americas_team_lead and lead", a, b)
	}

	// Stepping the America lead down, and a Team lead back to engineer.
	if err := repo.SetRole(ctx, pcActor, pcMemberA, "lead"); err != nil {
		t.Fatalf("step A down to lead: %v", err)
	}
	if err := repo.SetRole(ctx, pcActor, pcMemberB, "engineer"); err != nil {
		t.Fatalf("step B down to engineer: %v", err)
	}
	if a, b := pcRoleOf(t, pool, pcMemberA), pcRoleOf(t, pool, pcMemberB); a != "lead" || b != "engineer" {
		t.Fatalf("A is %q and B is %q, want lead and engineer", a, b)
	}
}
