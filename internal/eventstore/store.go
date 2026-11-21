// Package eventstore persists only the privacy-safe event summary, never raw API data.
package eventstore

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Event struct {
	EventID  string  `json:"eventId"`
	Title    *string `json:"title"`
	Start    *string `json:"start"`
	End      *string `json:"end"`
	Timezone *string `json:"timezone"`
	State    *string `json:"state"`
	UserRole *string `json:"userRole"`
	MyRSVP   *string `json:"myRsvp"`
}
type Filter struct {
	Status, Since, Until string
	Limit                int
}
type Store struct{ db *sql.DB }

var ErrAccountMismatch = errors.New("database belongs to another account")

func Open(path string, readOnly bool) (*Store, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if !readOnly {
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if e != nil {
			return nil, e
		}
		if e = f.Close(); e != nil {
			return nil, e
		}
	}
	mode := "rw"
	if readOnly {
		mode = "ro"
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := url.Values{"mode": {mode}, "_pragma": {"busy_timeout(5000)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error { return s.db.Close() }

// Upsert atomically updates a bounded snapshot. The account fingerprint prevents
// accidentally combining different accounts in the same local database.
func (s *Store) Upsert(ctx context.Context, account string, events []Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS events (event_id TEXT PRIMARY KEY, title TEXT, start TEXT, end TEXT, timezone TEXT, state TEXT, user_role TEXT, my_rsvp TEXT, synced_at TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS events_status_start ON events(my_rsvp,start);`)
	if err != nil {
		return err
	}
	var owner string
	err = tx.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key='account'").Scan(&owner)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && owner != account {
		return ErrAccountMismatch
	}
	if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO metadata(key,value) VALUES('account',?)", account); err != nil {
		return err
	}
	for _, e := range events {
		// Canonical UTC strings sort chronologically regardless of source offsets.
		canonical := func(v *string) (*string, error) {
			if v == nil {
				return nil, nil
			}
			t, err := time.Parse(time.RFC3339, *v)
			if err != nil {
				return nil, err
			}
			value := t.UTC().Format("2006-01-02T15:04:05.000000000Z")
			return &value, nil
		}
		e.Start, err = canonical(e.Start)
		if err != nil {
			return err
		}
		e.End, err = canonical(e.End)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO events VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(event_id) DO UPDATE SET title=excluded.title,start=excluded.start,end=excluded.end,timezone=excluded.timezone,state=excluded.state,user_role=excluded.user_role,my_rsvp=excluded.my_rsvp,synced_at=excluded.synced_at`, e.EventID, e.Title, e.Start, e.End, e.Timezone, e.State, e.UserRole, e.MyRSVP, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) List(ctx context.Context, f Filter) ([]Event, error) {
	if f.Limit < 1 || f.Limit > 1000 {
		return nil, errors.New("limit must be 1 through 1000")
	}
	query := "SELECT event_id,title,start,end,timezone,state,user_role,my_rsvp FROM events WHERE 1=1"
	args := []any{}
	if f.Status != "" {
		query += " AND my_rsvp=?"
		args = append(args, f.Status)
	}
	for _, bound := range []struct{ value, op string }{{f.Since, ">="}, {f.Until, "<="}} {
		if bound.value != "" {
			t, err := time.Parse(time.RFC3339, bound.value)
			if err != nil {
				return nil, err
			}
			query += " AND start" + bound.op + "?"
			args = append(args, t.UTC().Format("2006-01-02T15:04:05.000000000Z"))
		}
	}
	query += " ORDER BY start DESC,event_id ASC LIMIT ?"
	args = append(args, f.Limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Event{}
	for rows.Next() {
		var e Event
		if err = rows.Scan(&e.EventID, &e.Title, &e.Start, &e.End, &e.Timezone, &e.State, &e.UserRole, &e.MyRSVP); err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}
