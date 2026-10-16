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

// The generated month's write, against a live PostgreSQL built from every
// migration. Skipped unless ENTITY_TEST_DATABASE_URL is set, like
// schedule_repo_integration_test.go, whose helpers it uses:
//
//	ENTITY_TEST_DATABASE_URL="postgres:///entity_test" go test -v -run TestRotaGenerateIntegration ./internal/repository/

package repository

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	genTeamID  = "6e4e0000-0000-4000-8000-000000000001"
	genTeamKey = "saasgenfixture"
	genUserA   = "6e4e0000-0000-4000-8000-000000000002"
	genUserB   = "6e4e0000-0000-4000-8000-000000000003"
	genUserC   = "6e4e0000-0000-4000-8000-000000000004"
	genDay     = "2026-11-04" // a Wednesday
	genDay2    = "2026-11-05"
	genLieu    = "Lieu leave for the weekend on-call of 2026-11-07"
)

var genOwner = RotaGeneratorOwner("SRE_SAAS")

func newRotaGenerateIntegrationRepo(t *testing.T) (RotaGenerateRepository, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("ENTITY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ENTITY_TEST_DATABASE_URL is not set; skipping the live-database tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	clean := func() {
		mustExec(t, pool, `DELETE FROM team_schedule_assignment WHERE team_key = $1`, genTeamKey)
		mustExec(t, pool, `DELETE FROM team_schedule_absence WHERE team_key = $1`, genTeamKey)
		mustExec(t, pool, `DELETE FROM team_member WHERE team_id = $1`, genTeamID)
		mustExec(t, pool, `DELETE FROM team WHERE id = $1`, genTeamID)
		mustExec(t, pool, `DELETE FROM "user" WHERE id IN ($1, $2, $3)`, genUserA, genUserB, genUserC)
	}
	clean()
	t.Cleanup(clean)

	mustExec(t, pool, `
		INSERT INTO team (id, created_on, updated_on, created_by, updated_by, name, key, type)
		VALUES ($1, NOW(), NOW(), 'fixture', 'fixture', $2, $2, 'sre-abt')`, genTeamID, genTeamKey)
	for _, u := range []struct{ id, email string }{
		{genUserA, "gen.a@example.test"}, {genUserB, "gen.b@example.test"}, {genUserC, "gen.c@example.test"},
	} {
		mustExec(t, pool, `
			INSERT INTO "user" (id, created_on, updated_on, created_by, updated_by, user_name, first_name, last_name, email)
			VALUES ($1, NOW(), NOW(), 'fixture', 'fixture', $2, 'Gen', 'Fixture', $2)`, u.id, u.email)
	}
	return NewRotaGenerateRepository(pool), pool
}

func genCountRows(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestRotaGenerateIntegration_WritesAndReplacesOnlyGeneratedRows(t *testing.T) {
	repo, pool := newRotaGenerateIntegrationRepo(t)
	ctx := context.Background()

	// B already holds TZ1 L1 by hand on the day.
	mustExec(t, pool, `
		INSERT INTO team_schedule_assignment
		  (id, created_on, updated_on, created_by, updated_by, user_id, team_key, shift_id, zone_id, tier,
		   rota_date, starts_at, ends_at, is_on_call, source)
		SELECT gen_random_uuid(), now(), now(), 'lead@example.test', 'lead@example.test', $1, $2, s.id, s.zone_id, 'L1',
		       $3::date,
		       ($3::date::timestamp + make_interval(mins => s.start_minute)) AT TIME ZONE s.authoring_time_zone,
		       ($3::date::timestamp + make_interval(mins => s.end_minute)) AT TIME ZONE s.authoring_time_zone,
		       s.is_on_call, 'MANUAL'
		  FROM team_schedule_shift s WHERE s.code = 'SRE_TZ1_L1'`, genUserB, genTeamKey, genDay)
	// A GENERATED turn something else wrote (the seed, a rota import): it is not
	// the generator's, so no regeneration may remove it.
	mustExec(t, pool, `
		INSERT INTO team_schedule_assignment
		  (id, created_on, updated_on, created_by, updated_by, user_id, team_key, shift_id, zone_id, tier,
		   rota_date, starts_at, ends_at, is_on_call, source)
		SELECT gen_random_uuid(), now(), now(), 'rota-import', 'rota-import', $1, $2, s.id, s.zone_id, 'L1',
		       '2026-11-12'::date,
		       ('2026-11-12'::date::timestamp + make_interval(mins => s.start_minute)) AT TIME ZONE s.authoring_time_zone,
		       ('2026-11-12'::date::timestamp + make_interval(mins => s.end_minute)) AT TIME ZONE s.authoring_time_zone,
		       s.is_on_call, 'GENERATED'
		  FROM team_schedule_shift s WHERE s.code = 'SRE_TZ2_L1'`, genUserC, genTeamKey)

	// A lead's own TZ3 SUP mark: regular hours are in the replace now, but only
	// the generator's own rows are, so this one must survive every regeneration.
	mustExec(t, pool, `
		INSERT INTO team_schedule_assignment
		  (id, created_on, updated_on, created_by, updated_by, user_id, team_key, shift_id, zone_id,
		   rota_date, starts_at, ends_at, is_on_call, source)
		SELECT gen_random_uuid(), now(), now(), 'lead@example.test', 'lead@example.test', $1, $2, s.id, s.zone_id,
		       '2026-11-13'::date,
		       ('2026-11-13'::date::timestamp + make_interval(mins => s.start_minute)) AT TIME ZONE s.authoring_time_zone,
		       ('2026-11-13'::date::timestamp + make_interval(mins => s.end_minute)) AT TIME ZONE s.authoring_time_zone,
		       s.is_on_call, 'MANUAL'
		  FROM team_schedule_shift s WHERE s.code = 'SRE_TZ3_REGULAR'`, genUserB, genTeamKey)

	write := RotaMonthWrite{
		ActorEmail: "lead@example.test",
		Owner:      genOwner,
		TeamKeys:   []string{genTeamKey},
		ShiftCodes: []string{"SRE_TZ1_L1", "SRE_TZ1", "SRE_TZ2_L1", "SRE_TZ2", "SRE_TZ3", "SRE_WE_TZ1", "SRE_TZ1_REGULAR", "SRE_TZ2_REGULAR", "SRE_TZ3_REGULAR"},
		From:       "2026-11-01",
		To:         "2026-11-30",
		LieuNotes:  []string{genLieu},
		Turns: []RotaTurnRow{
			{UserID: genUserA, TeamKey: genTeamKey, ShiftCode: "SRE_TZ2_L1", Tier: "L1", RotaDate: genDay},
			// Overlaps B's own TZ1 L1: skipped, never an error.
			{UserID: genUserB, TeamKey: genTeamKey, ShiftCode: "SRE_TZ1", Tier: "L2", RotaDate: genDay},
			{UserID: genUserC, TeamKey: genTeamKey, ShiftCode: "SRE_TZ1", Tier: "L3", RotaDate: genDay},
			// A SUP (regular hours): no tier, written as NULL.
			{UserID: genUserA, TeamKey: genTeamKey, ShiftCode: "SRE_TZ2_REGULAR", RotaDate: genDay2},
		},
		Lieu: []RotaLieuRow{{UserID: genUserA, TeamKey: genTeamKey, From: "2026-11-09", To: "2026-11-10", Note: genLieu}},
	}
	res, err := repo.ReplaceGeneratedMonth(ctx, write)
	if err != nil {
		t.Fatal(err)
	}
	if res.Written != 3 || res.LieuWritten != 1 || res.Replaced != 0 {
		t.Fatalf("first run: %+v", res)
	}
	if n := genCountRows(t, pool, `SELECT count(*) FROM team_schedule_assignment WHERE team_key = $1 AND source = 'GENERATED' AND created_by = $2 AND updated_by = 'lead@example.test'`, genTeamKey, genOwner); n != 3 {
		t.Fatalf("generated rows owned by the generator = %d", n)
	}
	owned, err := repo.GeneratedRows(ctx, genOwner, []string{genTeamKey}, "2026-11-01", "2026-11-30")
	if err != nil {
		t.Fatal(err)
	}
	if n := genCountRows(t, pool, `SELECT count(*) FROM team_schedule_assignment a JOIN team_schedule_shift s ON s.id = a.shift_id WHERE a.team_key = $1 AND s.code = 'SRE_TZ2_REGULAR' AND a.tier IS NULL`, genTeamKey); n != 1 {
		t.Fatalf("SUP rows written with no tier = %d", n)
	}
	if len(owned.Assignments) != 3 || len(owned.Absences) != 1 {
		t.Fatalf("owned rows: %d turns, %d lieu (the imported GENERATED turn must not count)", len(owned.Assignments), len(owned.Absences))
	}
	// The instants come from the shift: TZ2 L1 is 13:30-21:00 Colombo.
	if n := genCountRows(t, pool, `
		SELECT count(*) FROM team_schedule_assignment a JOIN team_schedule_shift s ON s.id = a.shift_id
		 WHERE a.user_id = $1 AND s.code = 'SRE_TZ2_L1'
		   AND a.starts_at = TIMESTAMPTZ '2026-11-04 13:30 Asia/Colombo'
		   AND a.ends_at   = TIMESTAMPTZ '2026-11-04 21:00 Asia/Colombo'`, genUserA); n != 1 {
		t.Fatal("generated turn does not carry the shift's own instants")
	}
	// Nothing written to the history the roster reads its "edited by hand" marks from.
	if n := genCountRows(t, pool, `SELECT count(*) FROM team_schedule_assignment_activity WHERE team_key = $1`, genTeamKey); n != 0 {
		t.Fatalf("activity rows = %d", n)
	}

	// A regeneration replaces its own rows and its own lieu, and nothing else.
	write.Turns = []RotaTurnRow{{UserID: genUserC, TeamKey: genTeamKey, ShiftCode: "SRE_TZ2", Tier: "L2", RotaDate: genDay2}}
	res, err = repo.ReplaceGeneratedMonth(ctx, write)
	if err != nil {
		t.Fatal(err)
	}
	if res.Replaced != 3 || res.Written != 1 || res.LieuWritten != 1 {
		t.Fatalf("regeneration: %+v", res)
	}
	if n := genCountRows(t, pool, `SELECT count(*) FROM team_schedule_assignment WHERE team_key = $1 AND source = 'MANUAL'`, genTeamKey); n != 2 {
		t.Fatal("a regeneration removed a turn or SUP set by hand")
	}
	if n := genCountRows(t, pool, `SELECT count(*) FROM team_schedule_assignment WHERE team_key = $1 AND created_by = 'rota-import'`, genTeamKey); n != 1 {
		t.Fatal("a regeneration removed a GENERATED turn it did not write")
	}
	if n := genCountRows(t, pool, `SELECT count(*) FROM team_schedule_absence WHERE team_key = $1 AND note = $2`, genTeamKey, genLieu); n != 1 {
		t.Fatalf("lieu spans after regeneration = %d", n)
	}

	// A regeneration from a later day leaves earlier generated turns alone.
	write.From = "2026-11-06"
	write.Turns = nil
	write.Lieu = nil
	write.LieuNotes = nil
	res, err = repo.ReplaceGeneratedMonth(ctx, write)
	if err != nil {
		t.Fatal(err)
	}
	if res.Replaced != 0 {
		t.Fatalf("removed %d generated turns dated before from", res.Replaced)
	}
}

func TestRotaGenerateIntegration_LieuOverLeaveIsSkipped(t *testing.T) {
	repo, pool := newRotaGenerateIntegrationRepo(t)
	mustExec(t, pool, `
		INSERT INTO team_schedule_absence (id, created_on, updated_on, created_by, updated_by, user_id, team_key, kind_id, starts_on, ends_on)
		SELECT gen_random_uuid(), now(), now(), 'lead@example.test', 'lead@example.test', $1, $2, k.id, '2026-11-09', '2026-11-13'
		  FROM team_schedule_absence_kind k WHERE k.code = 'ANNUAL_LEAVE'`, genUserA, genTeamKey)
	res, err := repo.ReplaceGeneratedMonth(context.Background(), RotaMonthWrite{
		ActorEmail: "lead@example.test", Owner: genOwner, TeamKeys: []string{genTeamKey}, ShiftCodes: []string{"SRE_TZ1"},
		From: "2026-11-01", To: "2026-11-30", LieuNotes: []string{genLieu},
		Lieu: []RotaLieuRow{{UserID: genUserA, TeamKey: genTeamKey, From: "2026-11-09", To: "2026-11-10", Note: genLieu}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.LieuWritten != 0 {
		t.Fatal("lieu written over leave already booked")
	}
}

// A generation records the month's TZ3 people in the same transaction, and the
// next generation of that month replaces them -- with nobody chosen, clears
// them. Other months are left alone. A rota code of its own keeps the test off
// any real rota's months.
func TestRotaGenerateIntegration_RecordsAndReplacesTheNightCrew(t *testing.T) {
	repo, pool := newRotaGenerateIntegrationRepo(t)
	ctx := context.Background()
	const rota = "GEN_TEST"
	clean := func() { mustExec(t, pool, `DELETE FROM team_schedule_night_crew WHERE rota_code = $1`, rota) }
	clean()
	t.Cleanup(clean)

	write := func(month string, crew ...RotaNightCrewRow) {
		t.Helper()
		if _, err := repo.ReplaceGeneratedMonth(ctx, RotaMonthWrite{
			ActorEmail: "lead@example.test", Owner: RotaGeneratorOwner(rota), TeamKeys: []string{genTeamKey},
			ShiftCodes: []string{"SRE_TZ3"}, From: month, To: month,
			RotaCode: rota, Month: month, NightCrew: crew,
		}); err != nil {
			t.Fatal(err)
		}
	}
	write("2026-11-01", RotaNightCrewRow{UserID: genUserA, TeamKey: genTeamKey}, RotaNightCrewRow{UserID: genUserB, TeamKey: genTeamKey})
	write("2026-12-01", RotaNightCrewRow{UserID: genUserC, TeamKey: genTeamKey})

	got, err := repo.NightCrew(ctx, rota, "2026-10-01", "2026-12-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Month != "2026-11-01" || got[2].Month != "2026-12-01" || got[2].UserID != genUserC {
		t.Fatalf("night crew: %+v", got)
	}
	if n := genCountRows(t, pool, `SELECT count(*) FROM team_schedule_night_crew WHERE rota_code = $1 AND created_by = 'lead@example.test'`, rota); n != 3 {
		t.Fatalf("want 3 rows naming who chose them, got %d", n)
	}

	// November again, with only C: A and B are back on their team.
	write("2026-11-01", RotaNightCrewRow{UserID: genUserC, TeamKey: genTeamKey})
	got, _ = repo.NightCrew(ctx, rota, "2026-11-01", "2026-11-01")
	if len(got) != 1 || got[0].UserID != genUserC {
		t.Fatalf("november after the second run: %+v", got)
	}
	// November with nobody chosen: cleared; December untouched.
	write("2026-11-01")
	got, _ = repo.NightCrew(ctx, rota, "2026-11-01", "2026-12-01")
	if len(got) != 1 || got[0].Month != "2026-12-01" {
		t.Fatalf("after clearing november: %+v", got)
	}
}

// Lieu leave earned on the SaaS Night cover falls inside the move that put the
// person there. It is written, the move stepping aside for those days (as for
// leave a lead marks); regenerating the month without that lieu makes the
// move whole again.
func TestRotaGenerateIntegration_LieuInsideTheNightCoverSplitsTheMove(t *testing.T) {
	repo, pool := newRotaGenerateIntegrationRepo(t)
	ctx := context.Background()
	if genCountRows(t, pool, `SELECT count(*) FROM team_schedule_absence_kind WHERE code = 'SRE_NIGHT_COVER'`) == 0 {
		t.Fatal("no SRE_NIGHT_COVER tag; apply migration 0232")
	}
	t.Cleanup(func() {
		mustExec(t, pool, `DELETE FROM team_schedule_assignment WHERE user_id = $1::uuid`, genUserA)
		mustExec(t, pool, `DELETE FROM team_schedule_absence WHERE user_id = $1::uuid`, genUserA)
	})
	mustExec(t, pool, `
		INSERT INTO team_schedule_absence
		  (id, created_on, updated_on, created_by, updated_by, user_id, team_key, kind_id, starts_on, ends_on, home_team_key)
		SELECT gen_random_uuid(), now(), now(), 'lead@example.test', 'lead@example.test', $1, 'saas-night-cover', k.id,
		       '2026-11-02', '2026-11-27', $2
		  FROM team_schedule_absence_kind k WHERE k.code = 'SRE_NIGHT_COVER'`, genUserA, genTeamKey)
	moves := func() string {
		rows, err := pool.Query(ctx, `
			SELECT ab.starts_on::text || '..' || ab.ends_on::text
			  FROM team_schedule_absence ab JOIN team_schedule_absence_kind k ON k.id = ab.kind_id
			 WHERE ab.user_id = $1::uuid AND k.code = 'SRE_NIGHT_COVER' ORDER BY ab.starts_on`, genUserA)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		return strings.Join(out, " ")
	}

	write := RotaMonthWrite{
		ActorEmail: "lead@example.test", Owner: genOwner, TeamKeys: []string{genTeamKey, "saas-night-cover"},
		ShiftCodes: []string{"SRE_TZ3"}, From: "2026-11-01", To: "2026-11-30", LieuNotes: []string{genLieu},
		Lieu: []RotaLieuRow{{UserID: genUserA, TeamKey: genTeamKey, From: "2026-11-09", To: "2026-11-10", Note: genLieu}},
	}
	res, err := repo.ReplaceGeneratedMonth(ctx, write)
	if err != nil {
		t.Fatal(err)
	}
	if res.LieuWritten != 1 {
		t.Fatalf("lieu written %d times inside the Night cover, want 1", res.LieuWritten)
	}
	if got, want := moves(), "2026-11-02..2026-11-08 2026-11-11..2026-11-27"; got != want {
		t.Fatalf("the move around the lieu leave is %q, want %q", got, want)
	}

	// Regenerated without that lieu: the move is one span again.
	write.Lieu = nil
	if _, err := repo.ReplaceGeneratedMonth(ctx, write); err != nil {
		t.Fatal(err)
	}
	if got, want := moves(), "2026-11-02..2026-11-27"; got != want {
		t.Fatalf("after the lieu went, the move is %q, want %q", got, want)
	}
}
