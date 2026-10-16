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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RotaGenerateRepository writes a generated month of a rota. It is separate
// from ScheduleRepository on purpose: the lead edit path is untouched.
//
// The generator's rows are found by their owner, not by source alone: source
// GENERATED is also written by the local seed and by rota imports, and those
// rows must never be replaced by a regeneration. Every row the generator
// writes carries RotaGeneratorOwner(rota) as created_by; the person who ran it
// is updated_by and the audit actor.
type RotaGenerateRepository interface {
	ReplaceGeneratedMonth(ctx context.Context, w RotaMonthWrite) (RotaMonthWriteResult, error)
	// GeneratedRows is which assignments and absences in the window the
	// generator wrote for owner (and that nobody has since edited by hand: an
	// edit makes an assignment MANUAL or SWAP).
	GeneratedRows(ctx context.Context, owner string, teamKeys []string, from, to string) (GeneratedRowIDs, error)
	// NightCrew is the TZ3 people chosen for each month of the rota in
	// fromMonth..toMonth (first days of months), in month order.
	NightCrew(ctx context.Context, rotaCode, fromMonth, toMonth string) ([]RotaNightCrewRow, error)
}

// RotaNightCrewRow is one person chosen for a month's TZ3 night crew.
type RotaNightCrewRow struct {
	Month   string // YYYY-MM-DD, the first of the month
	UserID  string
	TeamKey string
}

// RotaGeneratorOwner is the created_by the generator writes for a rota's rows.
// No person has it as an email, so it can never be mistaken for one.
func RotaGeneratorOwner(rotaCode string) string { return "rota-generator:" + rotaCode }

// GeneratedRowIDs are the ids of rows the generator owns.
type GeneratedRowIDs struct {
	Assignments map[string]bool
	Absences    map[string]bool
}

// RotaTurnRow is one generated turn.
type RotaTurnRow struct {
	UserID    string
	TeamKey   string
	ShiftCode string
	Tier      string
	RotaDate  string
}

// RotaLieuRow is one generated lieu leave span; Note says which weekend it is
// for, and is how a regeneration finds it again.
type RotaLieuRow struct {
	UserID  string
	TeamKey string
	From    string
	To      string
	Note    string
}

// RotaMonthWrite is everything one generation writes, and the bounds of what
// it replaces.
type RotaMonthWrite struct {
	ActorEmail string
	// Owner is the generator's created_by (RotaGeneratorOwner). Only rows that
	// carry it are replaced; it is what every new row is written with.
	Owner string
	// TeamKeys and ShiftCodes bound the replace: only GENERATED rows of these
	// teams on these shifts, dated From..To, are removed.
	TeamKeys   []string
	ShiftCodes []string
	From       string
	To         string
	// LieuNotes are the notes of the lieu leave this generation owns; earlier
	// copies carrying one of them are removed before the new ones go in.
	LieuNotes []string
	Turns     []RotaTurnRow
	Lieu      []RotaLieuRow
	// RotaCode and Month (YYYY-MM-DD, the first of the month) say whose night
	// crew this generation records; NightCrew replaces that month's earlier
	// choice, and an empty one clears it. Month empty leaves the choice alone.
	RotaCode  string
	Month     string
	NightCrew []RotaNightCrewRow
}

// RotaMonthWriteResult counts what the write did.
type RotaMonthWriteResult struct {
	Replaced    int
	Written     int
	LieuWritten int
}

type rotaGenerateRepository struct{ db *pgxpool.Pool }

// NewRotaGenerateRepository constructs a RotaGenerateRepository over the pool.
func NewRotaGenerateRepository(db *pgxpool.Pool) RotaGenerateRepository {
	return &rotaGenerateRepository{db: db}
}

// ReplaceGeneratedMonth swaps a month's generated turns for new ones, in one
// transaction.
//
// Only rows the generator wrote (source GENERATED and created_by w.Owner) are
// removed; a GENERATED row anything else wrote (the seed, a rota import) is
// left alone. A turn a lead
// set or changed by hand is MANUAL, SWAP or MOVE and is never touched: an
// edit of a generated turn rewrites its source (UpdateAssignment). A new turn
// that would overlap a turn the person already holds is skipped by ON
// CONFLICT DO NOTHING, which covers the no-overlap exclusion as well as the
// one-turn-per-slot key, so the write never fails on the rota having moved
// since it was read.
//
// No activity rows are written: the Month roster marks a cell as edited by a
// person from that history, and nobody edited these by hand. The audit trigger
// (migration 0155) still records every row, under the actor named here.
func (r *rotaGenerateRepository) ReplaceGeneratedMonth(ctx context.Context, w RotaMonthWrite) (RotaMonthWriteResult, error) {
	var out RotaMonthWriteResult
	if w.Owner == "" {
		return out, fmt.Errorf("generated month: no owner")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return out, fmt.Errorf("begin generated month: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// One generation of a rota at a time. Two leads pressing Generate together
	// would otherwise each delete only the rows committed before they began,
	// and the month would come out as a mix of both runs. The second waits
	// here, then replaces what the first wrote.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, w.Owner); err != nil {
		return out, fmt.Errorf("lock generated month: %w", err)
	}

	if err := nameTheActor(ctx, tx, w.ActorEmail); err != nil {
		return out, err
	}

	tag, err := tx.Exec(ctx, `
		DELETE FROM team_schedule_assignment a
		 USING team_schedule_shift s
		 WHERE s.id = a.shift_id
		   AND a.source = 'GENERATED'
		   AND a.created_by = $5
		   AND a.team_key = ANY($1::text[])
		   AND s.code = ANY($2::text[])
		   AND a.rota_date BETWEEN $3::date AND $4::date`,
		w.TeamKeys, w.ShiftCodes, w.From, w.To, w.Owner)
	if err != nil {
		return out, fmt.Errorf("remove generated turns: %w", err)
	}
	out.Replaced = int(tag.RowsAffected())

	if len(w.LieuNotes) > 0 {
		rows, err := tx.Query(ctx, `
			DELETE FROM team_schedule_absence ab
			 USING team_schedule_absence_kind k
			 WHERE k.id = ab.kind_id
			   AND k.code = 'LIEU_LEAVE'
			   AND ab.created_by = $3
			   AND ab.team_key = ANY($1::text[])
			   AND ab.note = ANY($2::text[])
			RETURNING ab.user_id::text, ab.starts_on, ab.ends_on`,
			w.TeamKeys, w.LieuNotes, w.Owner)
		if err != nil {
			return out, fmt.Errorf("remove generated lieu leave: %w", err)
		}
		type gap struct {
			user     string
			from, to time.Time
		}
		var gaps []gap
		for rows.Next() {
			var g gap
			var to *time.Time
			if err := rows.Scan(&g.user, &g.from, &to); err != nil {
				rows.Close()
				return out, fmt.Errorf("scan removed lieu leave: %w", err)
			}
			g.to = g.from
			if to != nil {
				g.to = *to
			}
			gaps = append(gaps, g)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return out, fmt.Errorf("remove generated lieu leave: %w", err)
		}
		// Lieu leave inside a move (the Night cover) had cut the move in two;
		// with the lieu gone, the move is whole again.
		for _, g := range gaps {
			if err := mendMoveAround(ctx, tx, g.user, g.from.Format(time.DateOnly), g.to.Format(time.DateOnly), w.ActorEmail); err != nil {
				return out, err
			}
		}
	}

	if len(w.Turns) > 0 {
		users := make([]string, len(w.Turns))
		teams := make([]string, len(w.Turns))
		shifts := make([]string, len(w.Turns))
		tiers := make([]string, len(w.Turns))
		dates := make([]string, len(w.Turns))
		for i, t := range w.Turns {
			users[i], teams[i], shifts[i], tiers[i], dates[i] = t.UserID, t.TeamKey, t.ShiftCode, t.Tier, t.RotaDate
		}
		// The instants come from the shift, as CreateAssignment's do.
		tag, err := tx.Exec(ctx, `
			INSERT INTO team_schedule_assignment
			  (id, created_on, updated_on, created_by, updated_by, user_id, team_id, team_key,
			   shift_id, zone_id, tier, rota_date, starts_at, ends_at, is_on_call, source)
			SELECT gen_random_uuid(), now(), now(), $7, $1, v.user_id,
			       (SELECT t.id FROM team t WHERE t.key = v.team_key), v.team_key,
			       s.id, s.zone_id, NULLIF(v.tier, '')::team_schedule_tier_enum, v.rota_date,
			       (v.rota_date::timestamp + make_interval(mins => s.start_minute)) AT TIME ZONE s.authoring_time_zone,
			       (v.rota_date::timestamp + make_interval(mins => s.end_minute))   AT TIME ZONE s.authoring_time_zone,
			       s.is_on_call, 'GENERATED'
			  FROM unnest($2::uuid[], $3::text[], $4::text[], $5::text[], $6::date[])
			       AS v(user_id, team_key, shift_code, tier, rota_date)
			  JOIN team_schedule_shift s ON s.code = v.shift_code
			ON CONFLICT DO NOTHING`,
			w.ActorEmail, users, teams, shifts, tiers, dates, w.Owner)
		if err != nil {
			return out, fmt.Errorf("insert generated turns: %w", err)
		}
		out.Written = int(tag.RowsAffected())
	}

	if len(w.Lieu) > 0 {
		users := make([]string, len(w.Lieu))
		teams := make([]string, len(w.Lieu))
		froms := make([]string, len(w.Lieu))
		tos := make([]string, len(w.Lieu))
		notes := make([]string, len(w.Lieu))
		for i, l := range w.Lieu {
			users[i], teams[i], froms[i], tos[i], notes[i] = l.UserID, l.TeamKey, l.From, l.To, l.Note
		}
		// Lieu leave earned on the Night cover falls inside the move that put
		// the person there: the move steps aside for those days, as it does
		// for leave a lead marks. Any other time away is left as it is.
		for i := range w.Lieu {
			if err := cutMoveFor(ctx, tx, users[i], froms[i], tos[i], w.ActorEmail); err != nil {
				return out, err
			}
		}
		// A day the person is already away is left as it is: the no-overlap
		// exclusion turns that span into a skip, never an error.
		tag, err := tx.Exec(ctx, `
			INSERT INTO team_schedule_absence
			  (id, created_on, updated_on, created_by, updated_by, user_id, team_key,
			   kind_id, starts_on, ends_on, note)
			SELECT gen_random_uuid(), now(), now(), $7, $1, v.user_id, v.team_key,
			       k.id, v.starts_on, v.ends_on, v.note
			  FROM unnest($2::uuid[], $3::text[], $4::date[], $5::date[], $6::text[])
			       AS v(user_id, team_key, starts_on, ends_on, note)
			  JOIN team_schedule_absence_kind k ON k.code = 'LIEU_LEAVE'
			ON CONFLICT DO NOTHING`,
			w.ActorEmail, users, teams, froms, tos, notes, w.Owner)
		if err != nil {
			return out, fmt.Errorf("insert generated lieu leave: %w", err)
		}
		out.LieuWritten = int(tag.RowsAffected())
		for i := range w.Lieu {
			if err := syncMoveShifts(ctx, tx, users[i], froms[i], tos[i], w.ActorEmail); err != nil {
				return out, err
			}
		}
	}

	// The generator files a night under the team its person is on; this
	// keeps that true for anything it read before a move changed.
	if err := fileTurnsWhereTheyAre(ctx, tx, nil, w.From, w.To, w.ActorEmail); err != nil {
		return out, err
	}

	if w.Month != "" {
		// The month's TZ3 people, as chosen for this generation: the earlier
		// choice goes, so somebody left out is back on their team.
		if _, err := tx.Exec(ctx, `
			DELETE FROM team_schedule_night_crew WHERE rota_code = $1 AND month = $2::date`,
			w.RotaCode, w.Month); err != nil {
			return out, fmt.Errorf("remove night crew: %w", err)
		}
		if len(w.NightCrew) > 0 {
			users := make([]string, len(w.NightCrew))
			teams := make([]string, len(w.NightCrew))
			for i, c := range w.NightCrew {
				users[i], teams[i] = c.UserID, c.TeamKey
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO team_schedule_night_crew (rota_code, month, user_id, team_key, created_by)
				SELECT $1, $2::date, v.user_id, v.team_key, $3
				  FROM unnest($4::uuid[], $5::text[]) AS v(user_id, team_key)
				ON CONFLICT DO NOTHING`,
				w.RotaCode, w.Month, w.ActorEmail, users, teams); err != nil {
				return out, fmt.Errorf("insert night crew: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return out, fmt.Errorf("commit generated month: %w", err)
	}
	return out, nil
}

// NightCrew implements RotaGenerateRepository.
func (r *rotaGenerateRepository) NightCrew(ctx context.Context, rotaCode, fromMonth, toMonth string) ([]RotaNightCrewRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT to_char(month, 'YYYY-MM-DD'), user_id::text, team_key
		  FROM team_schedule_night_crew
		 WHERE rota_code = $1 AND month BETWEEN $2::date AND $3::date
		 ORDER BY month, team_key, user_id`,
		rotaCode, fromMonth, toMonth)
	if err != nil {
		return nil, fmt.Errorf("query night crew: %w", err)
	}
	defer rows.Close()
	out := []RotaNightCrewRow{}
	for rows.Next() {
		var c RotaNightCrewRow
		if err := rows.Scan(&c.Month, &c.UserID, &c.TeamKey); err != nil {
			return nil, fmt.Errorf("scan night crew: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GeneratedRows implements RotaGenerateRepository.
func (r *rotaGenerateRepository) GeneratedRows(ctx context.Context, owner string, teamKeys []string, from, to string) (GeneratedRowIDs, error) {
	out := GeneratedRowIDs{Assignments: map[string]bool{}, Absences: map[string]bool{}}
	rows, err := r.db.Query(ctx, `
		SELECT id::text FROM team_schedule_assignment
		 WHERE source = 'GENERATED' AND created_by = $1
		   AND team_key = ANY($2::text[])
		   AND rota_date BETWEEN $3::date AND $4::date`,
		owner, teamKeys, from, to)
	if err != nil {
		return out, fmt.Errorf("read generated turns: %w", err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return out, fmt.Errorf("scan generated turn: %w", err)
		}
		out.Assignments[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("read generated turns: %w", err)
	}

	rows, err = r.db.Query(ctx, `
		SELECT id::text FROM team_schedule_absence
		 WHERE created_by = $1
		   AND team_key = ANY($2::text[])
		   AND daterange(starts_on, ends_on, '[]') && daterange($3::date, $4::date, '[]')`,
		owner, teamKeys, from, to)
	if err != nil {
		return out, fmt.Errorf("read generated lieu leave: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return out, fmt.Errorf("scan generated lieu leave: %w", err)
		}
		out.Absences[id] = true
	}
	return out, rows.Err()
}

// cutMoveFor makes room for [from, to] in a person's moves to another team
// (the Night cover): a move straddling it is split around it, one overlapping
// an end is shortened, and one inside it is removed -- what ApplyAbsence does
// to a span a lead marks leave over. Only moves: leave and other allocations
// are never cut for generated lieu leave.
func cutMoveFor(ctx context.Context, tx pgx.Tx, userID, from, to, actorEmail string) error {
	rows, err := tx.Query(ctx, `
		SELECT ab.id::text, ab.starts_on < $2::date, ab.ends_on IS NULL OR ab.ends_on > $3::date, ab.ends_on
		  FROM team_schedule_absence ab
		  JOIN team_schedule_absence_kind k ON k.id = ab.kind_id
		 WHERE ab.user_id = $1::uuid AND k.moves_to_team_key IS NOT NULL
		   AND daterange(ab.starts_on, ab.ends_on, '[]') && daterange($2::date, $3::date, '[]')`,
		userID, from, to)
	if err != nil {
		return fmt.Errorf("read moves under lieu leave: %w", err)
	}
	type hit struct {
		id                      string
		startsBefore, endsAfter bool
		endsOn                  *time.Time
	}
	var hits []hit
	for rows.Next() {
		var h hit
		if err := rows.Scan(&h.id, &h.startsBefore, &h.endsAfter, &h.endsOn); err != nil {
			rows.Close()
			return fmt.Errorf("scan move under lieu leave: %w", err)
		}
		hits = append(hits, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read moves under lieu leave: %w", err)
	}
	for _, h := range hits {
		var err error
		switch {
		case h.startsBefore:
			_, err = tx.Exec(ctx, `UPDATE team_schedule_absence
				SET ends_on = $2::date - 1, updated_on = now(), updated_by = $3 WHERE id = $1::uuid`,
				h.id, from, actorEmail)
		case h.endsAfter:
			_, err = tx.Exec(ctx, `UPDATE team_schedule_absence
				SET starts_on = $2::date + 1, updated_on = now(), updated_by = $3 WHERE id = $1::uuid`,
				h.id, to, actorEmail)
		default:
			_, err = tx.Exec(ctx, `DELETE FROM team_schedule_absence WHERE id = $1::uuid`, h.id)
		}
		if err != nil {
			return fmt.Errorf("make room in a move for lieu leave: %w", err)
		}
		if h.startsBefore && h.endsAfter {
			// The part after the lieu leave, as a copy of the move.
			if _, err := tx.Exec(ctx, `
				INSERT INTO team_schedule_absence
				  (id, created_on, updated_on, created_by, updated_by, user_id, team_key,
				   kind_id, starts_on, ends_on, note, allocated_to, home_team_key)
				SELECT gen_random_uuid(), now(), now(), $3, $3, ab.user_id, ab.team_key,
				       ab.kind_id, $2::date + 1, $4::date, ab.note, ab.allocated_to, ab.home_team_key
				  FROM team_schedule_absence ab WHERE ab.id = $1::uuid`,
				h.id, to, actorEmail, h.endsOn); err != nil {
				return fmt.Errorf("split a move for lieu leave: %w", err)
			}
		}
	}
	return nil
}

// mendMoveAround joins the two halves of a move that [from, to] -- lieu leave
// just removed -- had split: the same tag, team and home team, one ending the
// day before and the other starting the day after. A move that only touches
// one side is left as it is: nothing says it once covered the gap.
func mendMoveAround(ctx context.Context, tx pgx.Tx, userID, from, to, actorEmail string) error {
	var head, tail string
	var tailEnds *time.Time
	err := tx.QueryRow(ctx, `
		SELECT h.id::text, t.id::text, t.ends_on
		  FROM team_schedule_absence h
		  JOIN team_schedule_absence t
		    ON t.user_id = h.user_id AND t.kind_id = h.kind_id
		   AND lower(t.team_key) = lower(h.team_key)
		   AND t.home_team_key IS NOT DISTINCT FROM h.home_team_key
		  JOIN team_schedule_absence_kind k ON k.id = h.kind_id
		 WHERE h.user_id = $1::uuid AND k.moves_to_team_key IS NOT NULL
		   AND h.ends_on = $2::date - 1 AND t.starts_on = $3::date + 1
		 LIMIT 1`, userID, from, to).Scan(&head, &tail, &tailEnds)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find move to mend after lieu leave: %w", err)
	}
	// The tail goes before the head grows over its dates: the two cannot both
	// stand under the no-overlap exclusion.
	if _, err := tx.Exec(ctx, `DELETE FROM team_schedule_absence WHERE id = $1::uuid`, tail); err != nil {
		return fmt.Errorf("mend move after lieu leave: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE team_schedule_absence SET ends_on = $2, updated_on = now(), updated_by = $3 WHERE id = $1::uuid`,
		head, tailEnds, actorEmail); err != nil {
		return fmt.Errorf("mend move after lieu leave: %w", err)
	}
	return syncMoveShifts(ctx, tx, userID, from, to, actorEmail)
}
