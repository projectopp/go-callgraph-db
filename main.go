package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"go-callgraph-db/sqlitegraph"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
)

func main() {

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
	filterPacks := map[string]struct{}{
		"main":        struct{}{},
		"sqlitegraph": struct{}{},
	}
	//2. create graphs
	traverseOut(db, a.callgraph.Root.Out, filterPacks)

	http.HandleFunc("/api/", RouteHandler(db))
	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.ListenAndServe(":8080", nil)

}

type GNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

func traverseOut(db *sql.DB, out []*callgraph.Edge, filterPacks map[string]struct{}) {
	for _, item := range out {
		if item == nil {
			return
		}
		if item.Caller == nil {
			return
		}
		if item.Caller.Func.Pkg == nil {
			return
		}
		if _, exists := filterPacks[item.Caller.Func.Pkg.Pkg.Name()]; !exists {
			return
		}
		callerNode := GNode{ID: FuncIDFromSSA(item.Caller.Func), Name: item.Caller.Func.Name(), Type: "func"}
		b1, _ := json.Marshal(&callerNode)
		if _, err := sqlitegraph.AddNode(callerNode.ID, b1, db); err != nil && err.Error() != "UNIQUE constraint failed: nodes.id" {
			slog.Error("error adding node", "error", err)
		}
		if item.Callee == nil {
			return
		}
		if item.Callee.Func.Pkg == nil {
			return
		}
		// slog.Info(item.Callee.Func.Pkg.Pkg.Name())
		// if _, exists := filterPacks[item.Callee.Func.Pkg.Pkg.Name()]; !exists {
		// 	return
		// }
		calleeNode := GNode{ID: FuncIDFromSSA(item.Callee.Func), Name: item.Callee.Func.Name(), Type: "func"}
		// if strings.Contains(calleeNode.ID, "go/types.(Basic)") ||
		// 	strings.Contains(calleeNode.ID, "tool") ||
		// 	strings.Contains(calleeNode.ID, "slog") ||
		// 	strings.Contains(calleeNode.ID, "strings.") ||
		// 	strings.Contains(calleeNode.ID, "go/types") {
		// 	return
		// }
		b2, _ := json.Marshal(&calleeNode)
		if _, err := sqlitegraph.AddNode(calleeNode.ID, b2, db); err != nil && err.Error() != "UNIQUE constraint failed: nodes.id" {
			slog.Error("error adding node", "error", err)
		}
		if _, err := sqlitegraph.ConnectNodes(callerNode.ID, calleeNode.ID, db); err != nil {
			slog.Error("error adding edge", "error", err)
		}
		slog.Info(fmt.Sprintf("funtion %s calls function %s:", callerNode.ID, calleeNode.ID))
		traverseOut(db, item.Callee.Out, filterPacks)
	}
}

func FuncIDFromSSA(fn *ssa.Function) string {
	if fn == nil {
		return "<nil-func>"
	}

	pkgPath := ""
	if fn.Pkg != nil && fn.Pkg.Pkg != nil {
		pkgPath = fn.Pkg.Pkg.Path()
	} else if fn.Parent() != nil {
		// Nested function: use parent's package
		parent := fn.Parent()
		if parent.Pkg != nil && parent.Pkg.Pkg != nil {
			pkgPath = parent.Pkg.Pkg.Path()
		}
	}

	name := fn.Name()

	// Strip synthetic suffix like $1, $2, etc.
	if idx := strings.Index(name, "$"); idx != -1 {
		name = name[:idx]
	}

	// Check for method: has a receiver
	if fn.Signature.Recv() != nil {
		recv := fn.Signature.Recv().Type().String()
		if idx := strings.LastIndex(recv, "."); idx != -1 {
			recv = recv[idx+1:]
		}
		return fmt.Sprintf("%s.(%s).%s", pkgPath, recv, name)
	}

	return fmt.Sprintf("%s.%s", pkgPath, name)
}
