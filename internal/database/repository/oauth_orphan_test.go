package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
)

type orphanBindDriver struct {
	ownerID     int64
	ownerExists bool
	newUserID   int64
	deleteRows  int64
	deleted     bool
	inserted    bool
	committed   bool
	sql         []string
}
type orphanBindConn struct{ state *orphanBindDriver }
type orphanBindTx struct{ state *orphanBindDriver }
type orphanBindRows struct {
	cols   []string
	values [][]driver.Value
	pos    int
}

var orphanBindDriverID int64

func (d *orphanBindDriver) Open(string) (driver.Conn, error) { return &orphanBindConn{state: d}, nil }
func (c *orphanBindConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *orphanBindConn) Close() error              { return nil }
func (c *orphanBindConn) Begin() (driver.Tx, error) { return &orphanBindTx{state: c.state}, nil }
func (tx *orphanBindTx) Commit() error              { tx.state.committed = true; return nil }
func (tx *orphanBindTx) Rollback() error            { return nil }
func (c *orphanBindConn) QueryContext(_ context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	c.state.sql = append(c.state.sql, q)
	switch {
	case strings.Contains(q, "FROM oauth_identities WHERE provider = ? AND subject = ?"):
		if strings.Contains(q, "FOR UPDATE") && c.state.ownerID != c.state.newUserID {
			lockedOwner := false
			for _, statement := range c.state.sql[:len(c.state.sql)-1] {
				lockedOwner = lockedOwner || strings.Contains(statement, "FROM users WHERE id = ? FOR UPDATE")
			}
			if !lockedOwner {
				return nil, errors.New("identity locked before owner user")
			}
		}
		if c.state.deleted {
			return &orphanBindRows{cols: []string{"user_id"}}, nil
		}
		return &orphanBindRows{cols: []string{"user_id"}, values: [][]driver.Value{{c.state.ownerID}}}, nil
	case strings.Contains(q, "FROM users WHERE id = ?"):
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong user lock: %v", args)
		}
		if args[0].Value == c.state.newUserID {
			return &orphanBindRows{cols: []string{"exists"}, values: [][]driver.Value{{int64(1)}}}, nil
		}
		if args[0].Value != c.state.ownerID {
			return nil, fmt.Errorf("wrong owner check: %v", args)
		}
		if c.state.ownerExists {
			return &orphanBindRows{cols: []string{"exists"}, values: [][]driver.Value{{int64(1)}}}, nil
		}
		return &orphanBindRows{cols: []string{"exists"}}, nil
	case strings.Contains(q, "COUNT(*) FROM oauth_identities WHERE user_id = ? AND provider = ?"):
		return &orphanBindRows{cols: []string{"count"}, values: [][]driver.Value{{int64(0)}}}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", q)
	}
}
func (c *orphanBindConn) ExecContext(_ context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	c.state.sql = append(c.state.sql, q)
	switch {
	case strings.Contains(q, "DELETE FROM oauth_identities WHERE provider = ? AND subject = ? AND user_id = ?"):
		if len(args) != 3 || args[2].Value != c.state.ownerID {
			return nil, fmt.Errorf("wrong orphan cleanup: %v", args)
		}
		if c.state.deleteRows == 0 {
			return driver.RowsAffected(0), nil
		}
		c.state.deleted = true
	case strings.Contains(q, "INSERT INTO oauth_identities"):
		if !c.state.deleted {
			return nil, errors.New("old identity not removed")
		}
		c.state.inserted = true
	case strings.Contains(q, "UPDATE oauth_identities"):
		return nil, errors.New("should not refresh another user's identity")
	default:
		return nil, fmt.Errorf("unexpected exec: %s", q)
	}
	return driver.RowsAffected(1), nil
}
func (r *orphanBindRows) Columns() []string { return r.cols }
func (r *orphanBindRows) Close() error      { return nil }
func (r *orphanBindRows) Next(dest []driver.Value) error {
	if r.pos == len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.pos])
	r.pos++
	return nil
}
func openOrphanBindDB(t *testing.T, state *orphanBindDriver) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("orphan-bind-%d", atomic.AddInt64(&orphanBindDriverID, 1))
	sql.Register(name, state)
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}
func TestBindDoesNotTransferIdentityWhenOrphanDeleteLosesRace(t *testing.T) {
	state := &orphanBindDriver{ownerID: 9, newUserID: 12, deleteRows: 0}
	repo := &OAuthIdentityRepository{db: openOrphanBindDB(t, state)}
	if err := repo.Bind(12, "github", OAuthIdentityProfile{Subject: "12345"}); err == nil {
		t.Fatal("expected error when orphan cleanup affected no rows")
	}
	if state.inserted || state.committed {
		t.Fatal("identity transferred without deleting original row")
	}
}

func TestBindReclaimsIdentityOnlyWhenPreviousOwnerIsGone(t *testing.T) {
	for _, tc := range []struct {
		name        string
		ownerExists bool
		wantErr     error
	}{
		{"deleted owner", false, nil}, {"existing owner", true, ErrIdentityOwnedElsewhere},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &orphanBindDriver{ownerID: 9, ownerExists: tc.ownerExists, newUserID: 12, deleteRows: 1}
			repo := &OAuthIdentityRepository{db: openOrphanBindDB(t, state)}
			err := repo.Bind(12, "github", OAuthIdentityProfile{Subject: "12345", DisplayName: "Octocat"})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Bind error = %v, want %v", err, tc.wantErr)
			}
			if tc.ownerExists && (state.deleted || state.inserted || state.committed) {
				t.Fatal("bound identity belonging to live user was altered")
			}
			if !tc.ownerExists && (!state.deleted || !state.inserted || !state.committed) {
				t.Fatalf("orphan not reclaimed: %+v", state)
			}
		})
	}
}
