package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"ai-later-nav/internal/database"
)

func TestDatabaseOIDCUserRefreshesProfileForExistingIdentity(t *testing.T) {
	stub := &oauthProfileStub{}
	driverName := fmt.Sprintf("oauth-profile-%d", atomic.AddInt64(&oauthProfileStubID, 1))
	sql.Register(driverName, stub)
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	original := database.DB
	database.DB = db
	defer func() { database.DB = original }()

	user, err := databaseOIDCUser("github", oauthIdentityProfile{
		Subject: "42", PreferredUsername: "octocat", DisplayName: "Octo Cat",
		Email: "octo@example.com", Picture: "https://example.com/avatar.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if user.Nickname != "用户自己的昵称" {
		t.Fatalf("local nickname overwritten: %+v", user)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.execs) != 1 || !strings.Contains(stub.execs[0].query, "UPDATE oauth_identities SET preferred_username") {
		t.Fatalf("profile was not refreshed: %+v", stub.execs)
	}
	args := stub.execs[0].args
	want := []interface{}{"octocat", "Octo Cat", "https://example.com/avatar.png", "octo@example.com", "github", "42"}
	for i, expected := range want {
		if args[i].Value != expected {
			t.Errorf("arg %d = %v, want %v", i, args[i].Value, expected)
		}
	}
}

func TestDatabaseOIDCUserCreatesIdentityWithProfile(t *testing.T) {
	stub := &oauthProfileStub{newUser: true}
	driverName := fmt.Sprintf("oauth-new-profile-%d", atomic.AddInt64(&oauthProfileStubID, 1))
	sql.Register(driverName, stub)
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	original := database.DB
	database.DB = db
	defer func() { database.DB = original }()

	user, err := databaseOIDCUser("oidc", oauthIdentityProfile{
		Subject: "new-subject", PreferredUsername: "new-user", DisplayName: "新用户",
		Email: "new@example.com", Picture: "https://example.com/new.png",
	})
	if err != nil || user.ID != 8 || user.Nickname != "新用户" {
		t.Fatalf("new user = %+v, %v", user, err)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.execs) != 2 || !strings.Contains(stub.execs[1].query, "picture, email") {
		t.Fatalf("profile columns missing from insert: %+v", stub.execs)
	}
	args := stub.execs[1].args
	if args[5].Value != "https://example.com/new.png" || args[6].Value != "new@example.com" {
		t.Fatalf("profile insert args = %+v", args)
	}
	if stub.commits != 1 {
		t.Fatalf("commits = %d", stub.commits)
	}
}

var oauthProfileStubID int64

type oauthProfileCall struct {
	query string
	args  []driver.NamedValue
}

type oauthProfileStub struct {
	mu        sync.Mutex
	execs     []oauthProfileCall
	newUser   bool
	commits   int
	rollbacks int
}

func (s *oauthProfileStub) Open(string) (driver.Conn, error) {
	return &oauthProfileConn{stub: s}, nil
}

type oauthProfileConn struct{ stub *oauthProfileStub }

func (c *oauthProfileConn) Prepare(string) (driver.Stmt, error) { return nil, fmt.Errorf("unused") }
func (c *oauthProfileConn) Close() error                        { return nil }
func (c *oauthProfileConn) Begin() (driver.Tx, error)           { return &oauthProfileTx{stub: c.stub}, nil }
func (c *oauthProfileConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "FROM oauth_identities o JOIN users u") {
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
	if c.stub.newUser {
		return &oauthProfileRows{columns: []string{"id", "username", "nickname", "role", "status"}}, nil
	}
	return &oauthProfileRows{
		columns: []string{"id", "username", "nickname", "role", "status"},
		values:  [][]driver.Value{{int64(8), "oidc_member", "用户自己的昵称", "user", "active"}},
	}, nil
}
func (c *oauthProfileConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.stub.mu.Lock()
	c.stub.execs = append(c.stub.execs, oauthProfileCall{query: query, args: append([]driver.NamedValue(nil), args...)})
	c.stub.mu.Unlock()
	if strings.Contains(query, "INSERT INTO users") {
		return oauthProfileInsertResult(8), nil
	}
	return driver.RowsAffected(1), nil
}

type oauthProfileInsertResult int64

func (r oauthProfileInsertResult) LastInsertId() (int64, error) { return int64(r), nil }
func (r oauthProfileInsertResult) RowsAffected() (int64, error) { return 1, nil }

type oauthProfileTx struct{ stub *oauthProfileStub }

func (t *oauthProfileTx) Commit() error {
	t.stub.mu.Lock()
	t.stub.commits++
	t.stub.mu.Unlock()
	return nil
}
func (t *oauthProfileTx) Rollback() error {
	t.stub.mu.Lock()
	t.stub.rollbacks++
	t.stub.mu.Unlock()
	return nil
}

type oauthProfileRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *oauthProfileRows) Columns() []string { return r.columns }
func (r *oauthProfileRows) Close() error      { return nil }
func (r *oauthProfileRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

var _ driver.Driver = (*oauthProfileStub)(nil)
var _ driver.QueryerContext = (*oauthProfileConn)(nil)
var _ driver.ExecerContext = (*oauthProfileConn)(nil)
