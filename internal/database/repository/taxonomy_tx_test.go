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
)

func TestCategoryRenameSynchronizesLiveReferences(t *testing.T) {
	stub := &taxonomyTxStub{}
	name := fmt.Sprintf("category-rename-%d", atomic.AddInt64(&taxonomyStubID, 1))
	sql.Register(name, stub)
	db, _ := sql.Open(name, "")
	defer db.Close()
	repo := &CategoryRepository{db: db}
	if err := repo.Rename(7, "旧分类", "新分类", "chat"); err != nil {
		t.Fatal(err)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.commits != 1 {
		t.Fatalf("commits = %d", stub.commits)
	}
	for _, fragment := range []string{"UPDATE categories SET name", "UPDATE sites SET category", "UPDATE site_submissions SET category", "UPDATE nav_items SET url"} {
		found := false
		for _, exec := range stub.execs {
			if strings.Contains(exec.query, fragment) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %s: %+v", fragment, stub.execs)
		}
	}
	for _, exec := range stub.execs {
		if strings.Contains(exec.query, "site_submissions") && !strings.Contains(exec.query, "status = 'pending'") {
			t.Errorf("historical submissions were modified: %s", exec.query)
		}
	}
}

func TestTagMergeDeduplicatesAndRemovesSource(t *testing.T) {
	stub := &taxonomyTxStub{}
	name := fmt.Sprintf("tag-merge-%d", atomic.AddInt64(&taxonomyStubID, 1))
	sql.Register(name, stub)
	db, _ := sql.Open(name, "")
	defer db.Close()
	repo := &TagRepository{db: db}
	if err := repo.Merge(3, 9, "旧标签", "新标签"); err != nil {
		t.Fatal(err)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.execs) != 4 || stub.commits != 1 {
		t.Fatalf("merge queries = %+v, commits = %d", stub.execs, stub.commits)
	}
	if !strings.Contains(stub.execs[0].query, "INSERT IGNORE INTO site_tags") || stub.execs[0].args[0].Value != int64(9) || stub.execs[0].args[1].Value != int64(3) {
		t.Fatalf("unexpected merge copy: %+v", stub.execs[0])
	}
	if !strings.Contains(stub.execs[1].query, "DELETE FROM site_tags") || !strings.Contains(stub.execs[2].query, "DELETE FROM tags") || !strings.Contains(stub.execs[3].query, "UPDATE site_submissions SET tags") {
		t.Fatalf("source tag not removed: %+v", stub.execs)
	}
}

var taxonomyStubID int64

type taxonomyExec struct {
	query string
	args  []driver.NamedValue
}

type taxonomyTxStub struct {
	mu             sync.Mutex
	execs          []taxonomyExec
	commits        int
	rollbacks      int
	submissionTags []string
}

func (s *taxonomyTxStub) Open(string) (driver.Conn, error) { return &taxonomyConn{stub: s}, nil }

type taxonomyConn struct{ stub *taxonomyTxStub }

func (c *taxonomyConn) Prepare(string) (driver.Stmt, error) { return nil, fmt.Errorf("unused") }
func (c *taxonomyConn) Close() error                        { return nil }
func (c *taxonomyConn) Begin() (driver.Tx, error)           { return &taxonomyTx{stub: c.stub}, nil }
func (c *taxonomyConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "FROM site_submissions") {
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
	return &taxonomyRows{columns: []string{"id", "tags"}, values: [][]driver.Value{{int64(1), "旧标签,新标签,其他"}}}, nil
}
func (c *taxonomyConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.stub.mu.Lock()
	c.stub.execs = append(c.stub.execs, taxonomyExec{query: query, args: args})
	c.stub.mu.Unlock()
	return driver.RowsAffected(1), nil
}

type taxonomyRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *taxonomyRows) Columns() []string { return r.columns }
func (r *taxonomyRows) Close() error      { return nil }
func (r *taxonomyRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

type taxonomyTx struct{ stub *taxonomyTxStub }

func (t *taxonomyTx) Commit() error   { t.stub.commits++; return nil }
func (t *taxonomyTx) Rollback() error { t.stub.rollbacks++; return nil }

var _ driver.Driver = (*taxonomyTxStub)(nil)
var _ driver.ExecerContext = (*taxonomyConn)(nil)
