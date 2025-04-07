package main

import (
	"database/sql"
	"fmt"
	"go-callgraph-db/sqlitegraph"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
)

func removeDB(dbPath string){
	_ = os.MkdirAll(filepath.Dir(dbPath), 0755)
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		slog.Error("failed to delete old database file", slog.Any("error", err))
		return
	}
}

func main() {

	removeDB(".data/app.data")
	removeDB(".data/app.data-shm")
	removeDB(".data/app.data-wal")

	db, err := sql.Open("sqlite3", "file:.data/app.data?_journal_mode=WAL&_foreign_keys=true&_busy_timeout=5000")
	if err != nil {
		slog.Error("failed to open writer db while pooling", slog.Any("error", err))
		return
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(10 * time.Minute)
	if err := db.Ping(); err != nil {
		slog.Error("failed to ping writer database", slog.Any("error", err))
		return
	}
	slog.Info("ping successfull to writer db")
	if err = sqlitegraph.Initialize(db); err != nil {
		slog.Error("failed to intialise graph schema", slog.Any("error", err))
		return
	}
	defer db.Close()

	//1. analyse
	a := new(analysis)
	if err = a.DoAnalysis(CallGraphTypeRta, "", false, nil); err != nil {
		slog.Error("failed to analyse", "error", err)
		return
	}
	allowedPacks := make(map[string]bool)
	allowedPacks["go-callgraph-db"] = true
	allowedPacks["go-callgraph-db/sqlitegraph"] = true

	ignorePacks := make(map[string]bool)
	ignorePacks["log/slog"] = true
	ignorePacks["database/sql"] = true
 	visited := make(map[string]bool)
 	traverseOut(db, a.callgraph.Root.Out, allowedPacks, ignorePacks, visited)
	slog.Info("finished analysis")

	fib(3)
	even(4)
	odd(3)

	http.HandleFunc("/api/", RouteHandler(db))
	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.ListenAndServe(":8080", nil)

}

func traverseOut(db *sql.DB, out []*callgraph.Edge, allowedPacks, ignorePacks, visited map[string]bool) {
	for _, item := range out {
		callerNodeBody := sqlitegraph.NodeBody{ID: FuncID(item.Caller.Func), Name: item.Caller.Func.Name(), Type: "func"}
		if err := sqlitegraph.AddNode(callerNodeBody, db); err != nil && err.Error() != "UNIQUE constraint failed: nodes.id" {
			slog.Error("error adding node", "error", err)
		}

		var calleePkgName string
		if item.Callee.Func.Pkg != nil && item.Callee.Func.Pkg.Pkg != nil {
			calleePkgName = item.Callee.Func.Pkg.Pkg.Path()
		} else if item.Callee.Func.Parent() != nil {
			parent := item.Callee.Func.Parent()
			if parent.Pkg != nil && parent.Pkg.Pkg != nil {
				calleePkgName = parent.Pkg.Pkg.Path()
			}
		}

		if _, ignore := ignorePacks[calleePkgName]; ignore {
			for k := range ignorePacks {
				if strings.HasPrefix(calleePkgName, k) {
					ignore = true
					break
				}
			}
			if ignore {
				continue
			}
 		}

		if _, allowed := allowedPacks[calleePkgName]; !allowed {
			for k := range allowedPacks {
				if strings.HasPrefix(calleePkgName, k) {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}

		callerID := FuncID(item.Caller.Func)
		calleeID := FuncID(item.Callee.Func)
		edgeKey := fmt.Sprintf("%s->%s", callerID, calleeID)
		v, exists := visited[edgeKey]
		if exists {
			if v {
				continue
			} else {
				visited[edgeKey] = true
			}
		} else {
			visited[edgeKey] = false
		}

		calleeNodeBody := sqlitegraph.NodeBody{ID: FuncID(item.Callee.Func), Name: item.Callee.Func.Name(), Type: "func"}
		if err := sqlitegraph.AddNode(calleeNodeBody, db); err != nil && err.Error() != "UNIQUE constraint failed: nodes.id" {
			slog.Error("error adding node", "error", err)
		}

		if err := sqlitegraph.AddEdge(sqlitegraph.Edge{Source: callerNodeBody.ID, Target: calleeNodeBody.ID}, db); err != nil {
			slog.Error("error adding edge", "error", err)
		}

		traverseOut(db, item.Callee.Out, allowedPacks, ignorePacks, visited)
	}
}

func FuncID(fn *ssa.Function) string {
	if fn == nil {
		return ""
	}
	var pkgPath string
	if fn.Pkg != nil && fn.Pkg.Pkg != nil {
		pkgPath = fn.Pkg.Pkg.Path()
	} else if fn.Parent() != nil {
		parent := fn.Parent()
		if parent.Pkg != nil && parent.Pkg.Pkg != nil {
			pkgPath = parent.Pkg.Pkg.Path()
		}
	}
	name := fn.Name()
	if idx := strings.Index(name, "$"); idx != -1 {
		name = name[:idx]
	}
	if fn.Signature.Recv() != nil {
		recv := fn.Signature.Recv().Type().String()
		if idx := strings.LastIndex(recv, "."); idx != -1 {
			recv = recv[idx+1:]
		}
		return fmt.Sprintf("%s.%s.%s", pkgPath, recv, name)
	}
	return fmt.Sprintf("%s.standalone.%s", pkgPath, name)
}

func fib(n int) int {
	if n <= 1 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

func even(n int) bool {
	if n == 0 {
		return true
	}
	return odd(n - 1)
}

func odd(n int) bool {
	if n == 0 {
		return false
	}
	return even(n - 1)
}
