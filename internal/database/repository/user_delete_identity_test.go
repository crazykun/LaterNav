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

// This fake database deliberately does not implement ON DELETE CASCADE: it
// reproduces a database whose oauth_identities table lacks the expected FK.
type deleteIdentityDriver struct {
	failIdentityDelete bool
	failFollowingStep  bool
	identityOwner      int64
	deletedUser        bool
	committed          bool
	statements         []string
	rolledBack         bool
}
type deleteIdentityConn struct{ state *deleteIdentityDriver }
type deleteIdentityTx struct{ state *deleteIdentityDriver }
type deleteIdentityRows struct {
	values  [][]driver.Value
	columns []string
	pos     int
}

var deleteIdentityDriverID int64

func (s *deleteIdentityDriver) Open(string) (driver.Conn, error) {
	return &deleteIdentityConn{state: s}, nil
}
func (c *deleteIdentityConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *deleteIdentityConn) Close() error { return nil }
func (c *deleteIdentityConn) Begin() (driver.Tx, error) {
	return &deleteIdentityTx{state: c.state}, nil
}
func (tx *deleteIdentityTx) Commit() error { tx.state.committed = true; return nil }
func (tx *deleteIdentityTx) Rollback() error {
	tx.state.rolledBack = true
	tx.state.identityOwner = 9
	tx.state.deletedUser = false
	return nil
}
func (c *deleteIdentityConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "SELECT 1 FROM users WHERE id = ? FOR UPDATE"):
		if c.state.deletedUser {
			return &deleteIdentityRows{columns: []string{"exists"}}, nil
		}
		return &deleteIdentityRows{columns: []string{"exists"}, values: [][]driver.Value{{int64(1)}}}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}
func (c *deleteIdentityConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.state.statements = append(c.state.statements, query)
	switch {
	case strings.Contains(query, "DELETE FROM oauth_identities WHERE user_id = ?"):
		if c.state.failIdentityDelete {
			return nil, errors.New("identity delete failed")
		}
		if len(args) != 1 || args[0].Value != int64(9) {
			return nil, fmt.Errorf("wrong identity delete arguments: %v", args)
		}
		c.state.identityOwner = 0
	case strings.Contains(query, "DELETE FROM site_submissions WHERE user_id = ?"):
		if c.state.failFollowingStep {
			return nil, errors.New("submission delete failed")
		}
	case strings.Contains(query, "DELETE FROM users WHERE id = ?"):
		if c.state.identityOwner != 0 {
			return nil, errors.New("identity still points to deleted account")
		}
		c.state.deletedUser = true
	}
	return driver.RowsAffected(1), nil
}
func (r *deleteIdentityRows) Columns() []string { return r.columns }
func (r *deleteIdentityRows) Close() error      { return nil }
func (r *deleteIdentityRows) Next(dest []driver.Value) error {
	if r.pos == len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.pos])
	r.pos++
	return nil
}
func openDeleteIdentityDB(t *testing.T, state *deleteIdentityDriver) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("delete-identity-%d", atomic.AddInt64(&deleteIdentityDriverID, 1))
	sql.Register(name, state)
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}
func TestDeleteByIDReleasesOAuthIdentityWithoutCascade(t *testing.T) {
	state := &deleteIdentityDriver{identityOwner: 9}
	repo := &UserRepository{db: openDeleteIdentityDB(t, state)}
	if err := repo.DeleteByID(9); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if state.identityOwner != 0 {
		t.Fatal("GitHub identity is still bound to deleted account")
	}
	if !state.deletedUser {
		t.Fatal("user was not deleted")
	}
}
func TestDeleteByIDRollsBackIdentityCleanupIfLaterStepFails(t *testing.T) {
	state := &deleteIdentityDriver{identityOwner: 9, failFollowingStep: true}
	repo := &UserRepository{db: openDeleteIdentityDB(t, state)}
	if err := repo.DeleteByID(9); err == nil {
		t.Fatal("expected submission cleanup error")
	}
	if !state.rolledBack || state.committed || state.deletedUser || state.identityOwner != 9 {
		t.Fatalf("user and identity should remain unchanged after rollback: %+v", state)
	}
}

func TestDeleteByIDDoesNotDeleteUserIfIdentityCleanupFails(t *testing.T) {
	state := &deleteIdentityDriver{identityOwner: 9, failIdentityDelete: true}
	repo := &UserRepository{db: openDeleteIdentityDB(t, state)}
	if err := repo.DeleteByID(9); err == nil {
		t.Fatal("expected identity cleanup error")
	}
	if state.deletedUser {
		t.Fatal("user deleted although identity cleanup failed")
	}
	if !state.rolledBack {
		t.Fatal("transaction was not rolled back")
	}
}
