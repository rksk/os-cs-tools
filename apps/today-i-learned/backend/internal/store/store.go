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

// Package store keeps TIL entries in MySQL (the til_entry table, see
// sql/create_til_tables.sql).
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/wso2-open-operations/cs-tools/apps/today-i-learned/backend/internal/entry"
)

// ErrNotFound is returned when no entry has the requested id.
var ErrNotFound = errors.New("entry not found")

// Config is how to reach the database.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	TLS      TLSMode
}

// TLSMode is how the connection to the database is secured, for a server
// that requires secure transport.
type TLSMode int

const (
	// TLSOff connects in plain text.
	TLSOff TLSMode = iota
	// TLSVerify encrypts the connection and checks the server's
	// certificate: it must come from a CA the system trusts and name Host.
	TLSVerify
	// TLSSkipVerify encrypts the connection without checking the server's
	// certificate, for a server whose certificate the system can't verify.
	// It doesn't guard against an impostor posing as the server.
	TLSSkipVerify
)

// Store reads and writes entries.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Author is who an entry is shared under: the byline as given, and the
// verified email of the person sharing it.
type Author struct {
	Name  string
	Email string
}

// Query selects a page of entries, newest first.
type Query struct {
	// Search matches the title, the learning's text, the author and the
	// source detail. Every term has to match; a term matches the start of a
	// word.
	Search string
	// From and Before bound the creation time: From inclusive, Before
	// exclusive. Nil means no bound. The webapp sends the start of the
	// reader's own first and last-plus-one days, so a day means their day.
	From, Before *time.Time
	// AuthorEmail, when set, keeps only that person's entries.
	AuthorEmail string
	// Author matches the author's name alone: every term has to match the
	// start of a word in it, so "ale sam" finds "Alex Sample".
	Author string
	// Source, when set, keeps only entries from that source.
	Source string
	Limit  int
	Offset int
}

// Page is one page of entries and how many entries match in total.
type Page struct {
	Items []entry.Entry
	Total int
}

// Open connects to the database described by cfg.
func Open(ctx context.Context, cfg Config) (*Store, error) {
	mc := mysql.NewConfig()
	mc.User = cfg.User
	mc.Passwd = cfg.Password
	mc.Net = "tcp"
	mc.Addr = net.JoinHostPort(cfg.Host, cfg.Port)
	mc.DBName = cfg.Name
	switch cfg.TLS {
	case TLSVerify:
		mc.TLSConfig = "true"
	case TLSSkipVerify:
		mc.TLSConfig = "skip-verify"
	}
	return openConfig(ctx, mc)
}

// openConfig applies the connection settings the store relies on: times read
// and written as UTC, and an UPDATE reporting the rows it matched rather than
// the rows it changed, so "no such entry" can't be mistaken for "no change".
func openConfig(ctx context.Context, mc *mysql.Config) (*Store, error) {
	mc.ParseTime = true
	mc.Loc = time.UTC
	mc.ClientFoundRows = true
	mc.Timeout = 10 * time.Second
	mc.ReadTimeout = 30 * time.Second
	mc.WriteTimeout = 30 * time.Second

	connector, err := mysql.NewConnector(mc)
	if err != nil {
		return nil, fmt.Errorf("store: configure connection: %w", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: connect: %w", err)
	}
	return &Store{db: db, now: time.Now}, nil
}

// Close releases the connection pool.
func (s *Store) Close() error { return s.db.Close() }

// CheckSchema fails when til_entry doesn't exist, naming the script to run,
// or predates a column this version reads, naming the change to make. The
// service never creates or changes tables itself.
func (s *Store) CheckSchema(ctx context.Context) error {
	var tables, notifiedAt int
	err := s.db.QueryRowContext(ctx,
		"SELECT (SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'til_entry'), "+
			"(SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'til_entry' AND column_name = 'notified_at')",
	).Scan(&tables, &notifiedAt)
	if err != nil {
		return fmt.Errorf("store: check schema: %w", err)
	}
	if tables == 0 {
		return errors.New("store: table til_entry does not exist; run sql/create_til_tables.sql against this database first")
	}
	if notifiedAt == 0 {
		return errors.New("store: table til_entry has no notified_at column; add it with: ALTER TABLE til_entry ADD COLUMN notified_at DATETIME(6) NULL AFTER updated_on")
	}
	return nil
}

const columns = "id, title, author_name, author_email, source, source_detail, content_html, created_on, updated_on, notified_at"

// Create stores a new entry and returns it.
func (s *Store) Create(ctx context.Context, author Author, c entry.Content) (entry.Entry, error) {
	id, err := newID()
	if err != nil {
		return entry.Entry{}, err
	}
	created := s.timestamp()
	_, err = s.db.ExecContext(ctx,
		"INSERT INTO til_entry (id, title, author_name, author_email, source, source_detail, content_html, content_text, created_on) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		id, c.Title, author.Name, author.Email, c.Where, c.WhereDetail, c.HTML, c.Text, created,
	)
	if err != nil {
		return entry.Entry{}, fmt.Errorf("store: create entry: %w", err)
	}
	return entry.Entry{
		ID:               id,
		Title:            c.Title,
		Who:              author.Name,
		Where:            c.Where,
		WhereDetail:      c.WhereDetail,
		What:             c.HTML,
		SubmittedByEmail: author.Email,
		CreatedAt:        created,
	}, nil
}

// Get returns one entry, or ErrNotFound.
func (s *Store) Get(ctx context.Context, id string) (entry.Entry, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+columns+" FROM til_entry WHERE id = ?", id)
	e, err := scanEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return entry.Entry{}, ErrNotFound
	}
	if err != nil {
		return entry.Entry{}, fmt.Errorf("store: get entry: %w", err)
	}
	return e, nil
}

// Update replaces an entry's content and marks it edited, or returns
// ErrNotFound. The byline and the creation time never change.
func (s *Store) Update(ctx context.Context, id string, c entry.Content) (entry.Entry, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE til_entry SET title = ?, source = ?, source_detail = ?, content_html = ?, content_text = ?, updated_on = ? WHERE id = ?",
		c.Title, c.Where, c.WhereDetail, c.HTML, c.Text, s.timestamp(), id,
	)
	if err != nil {
		return entry.Entry{}, fmt.Errorf("store: update entry: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return entry.Entry{}, fmt.Errorf("store: update entry: %w", err)
	} else if n == 0 {
		return entry.Entry{}, ErrNotFound
	}
	return s.Get(ctx, id)
}

// Delete removes an entry, or returns ErrNotFound.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM til_entry WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("store: delete entry: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete entry: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ClaimNotification marks an entry as announced now, before the announcement
// is sent, so two senders can't both announce it. It returns the time it set,
// and claimed false when the entry was already announced (or is gone).
func (s *Store) ClaimNotification(ctx context.Context, id string) (at time.Time, claimed bool, err error) {
	at = s.timestamp()
	res, err := s.db.ExecContext(ctx, "UPDATE til_entry SET notified_at = ? WHERE id = ? AND notified_at IS NULL", at, id)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("store: claim notification: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return time.Time{}, false, fmt.Errorf("store: claim notification: %w", err)
	}
	return at, n == 1, nil
}

// ReleaseNotification undoes the claim made at at, after its announcement
// failed, so the entry counts as not announced again. A later claim is left
// alone.
func (s *Store) ReleaseNotification(ctx context.Context, id string, at time.Time) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE til_entry SET notified_at = NULL WHERE id = ? AND notified_at = ?", id, at); err != nil {
		return fmt.Errorf("store: release notification: %w", err)
	}
	return nil
}

// List returns a page of the entries q selects, newest first.
func (s *Store) List(ctx context.Context, q Query) (Page, error) {
	where, args := filters(q)

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM til_entry"+where, args...).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("store: count entries: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		// #nosec G202 -- where is built only from fixed fragments (see filters); every value is a ? parameter.
		"SELECT "+columns+" FROM til_entry"+where+" ORDER BY created_on DESC, id DESC LIMIT ? OFFSET ?",
		append(args, q.Limit, q.Offset)...,
	)
	if err != nil {
		return Page{}, fmt.Errorf("store: list entries: %w", err)
	}
	defer rows.Close()

	items := []entry.Entry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return Page{}, fmt.Errorf("store: read entry: %w", err)
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("store: list entries: %w", err)
	}
	return Page{Items: items, Total: total}, nil
}

// filters builds the WHERE clause, with its arguments, shared by a page and
// its total.
func filters(q Query) (string, []any) {
	var conds []string
	var args []any
	if q.AuthorEmail != "" {
		conds = append(conds, "author_email = ?")
		args = append(args, q.AuthorEmail)
	}
	if q.Source != "" {
		conds = append(conds, "source = ?")
		args = append(args, q.Source)
	}
	if c, a := authorConditions(q.Author); len(c) > 0 {
		conds = append(conds, c...)
		args = append(args, a...)
	}
	if q.From != nil {
		conds = append(conds, "created_on >= ?")
		args = append(args, q.From.UTC())
	}
	if q.Before != nil {
		conds = append(conds, "created_on < ?")
		args = append(args, q.Before.UTC())
	}
	if c, a := searchConditions(q.Search); len(c) > 0 {
		conds = append(conds, c...)
		args = append(args, a...)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEntry(row scanner) (entry.Entry, error) {
	var e entry.Entry
	var detail sql.NullString
	var updated, notified sql.NullTime
	if err := row.Scan(&e.ID, &e.Title, &e.Who, &e.SubmittedByEmail, &e.Where, &detail, &e.What, &e.CreatedAt, &updated, &notified); err != nil {
		return entry.Entry{}, err
	}
	if detail.Valid {
		e.WhereDetail = &detail.String
	}
	if updated.Valid {
		t := updated.Time.UTC()
		e.UpdatedAt = &t
	}
	if notified.Valid {
		t := notified.Time.UTC()
		e.NotifiedAt = &t
	}
	e.CreatedAt = e.CreatedAt.UTC()
	return e, nil
}

// timestamp is the current time at the precision DATETIME(6) keeps, so a
// value returned straight after a write equals the one read back later.
func (s *Store) timestamp() time.Time {
	return s.now().UTC().Truncate(time.Microsecond)
}

// newID returns a random (version 4) UUID.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("store: generate id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
