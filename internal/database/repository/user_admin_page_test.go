package repository

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
	"time"
)

// TestAdminUserPageQueries verifies search, pagination, and the single identity query.
func TestAdminUserPageQueries(t *testing.T) {
	stub := &adminUserPageStub{}
	driverName := fmt.Sprintf("admin-user-page-%d", atomic.AddInt64(&adminUserPageStubID, 1))
	sql.Register(driverName, stub)
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := &UserRepository{db: db}

	total, err := repo.CountUsersMatching("昵称")
	if err != nil || total != 21 {
		t.Fatalf("CountUsersMatching = %d, %v", total, err)
	}
	users, err := repo.ListUsersPage("昵称", 20, 20)
	if err != nil || len(users) != 1 || users[0].DisplayName() != "会员昵称" {
		t.Fatalf("ListUsersPage = %+v, %v", users, err)
	}
	identities, err := repo.ListOAuthIdentitiesByUserIDs([]int64{8, 9})
	if err != nil {
		t.Fatal(err)
	}
	if len(identities[8]) != 1 || identities[8][0].Picture != "https://example.com/avatar.png" || identities[8][0].Email != "oauth@example.com" {
		t.Fatalf("unexpected identities: %+v", identities)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.queries) != 3 {
		t.Fatalf("query count = %d, want 3: %v", len(stub.queries), stub.queries)
	}
	for i, call := range stub.queries[:2] {
		if !strings.Contains(call.query, "EXISTS") {
			t.Errorf("query %d does not search oauth email: %s", i, call.query)
		}
		for _, arg := range call.args[:4] {
			if arg.Value != "%昵称%" {
				t.Errorf("query %d search arg = %v", i, arg.Value)
			}
		}
	}
	if got := stub.queries[1].args[4].Value; got != int64(20) {
		t.Errorf("limit = %v", got)
	}
	if got := stub.queries[1].args[5].Value; got != int64(20) {
		t.Errorf("offset = %v", got)
	}
	if len(stub.queries[2].args) != 2 || stub.queries[2].args[0].Value != int64(8) || stub.queries[2].args[1].Value != int64(9) {
		t.Errorf("identity args = %+v", stub.queries[2].args)
	}
}

var adminUserPageStubID int64

type adminUserPageCall struct {
	query string
	args  []driver.NamedValue
}

type adminUserPageStub struct {
	mu      sync.Mutex
	queries []adminUserPageCall
}

func (s *adminUserPageStub) Open(string) (driver.Conn, error) {
	return &adminUserPageConn{stub: s}, nil
}

type adminUserPageConn struct{ stub *adminUserPageStub }

func (c *adminUserPageConn) Prepare(string) (driver.Stmt, error) { return nil, fmt.Errorf("unused") }
func (c *adminUserPageConn) Close() error                        { return nil }
func (c *adminUserPageConn) Begin() (driver.Tx, error)           { return nil, fmt.Errorf("unused") }
func (c *adminUserPageConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.stub.mu.Lock()
	c.stub.queries = append(c.stub.queries, adminUserPageCall{query: query, args: append([]driver.NamedValue(nil), args...)})
	c.stub.mu.Unlock()
	switch {
	case strings.Contains(query, "SELECT COUNT(*) FROM users"):
		return &adminUserPageRows{columns: []string{"count"}, values: [][]driver.Value{{int64(21)}}}, nil
	case strings.Contains(query, "LIMIT ? OFFSET ?"):
		return &adminUserPageRows{
			columns: []string{"id", "username", "nickname", "email", "role", "status", "created_at", "updated_at"},
			values:  [][]driver.Value{{int64(8), "member", "会员昵称", "member@example.com", "user", "active", time.Now(), time.Now()}},
		}, nil
	case strings.Contains(query, "FROM oauth_identities oi"):
		return &adminUserPageRows{
			columns: []string{"user_id", "provider", "name", "type", "preferred_username", "display_name", "picture", "email"},
			values:  [][]driver.Value{{int64(8), "github", "GitHub", "github", "octocat", "Octo Cat", "https://example.com/avatar.png", "oauth@example.com"}},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

type adminUserPageRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *adminUserPageRows) Columns() []string { return r.columns }
func (r *adminUserPageRows) Close() error      { return nil }
func (r *adminUserPageRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

var _ driver.Driver = (*adminUserPageStub)(nil)
var _ driver.QueryerContext = (*adminUserPageConn)(nil)
