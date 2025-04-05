package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"go-callgraph-db/sqlitegraph"
	"log/slog"
	"time"

	"golang.org/x/tools/go/callgraph"
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
}

type GNode struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Package string `json:"package"`
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
		callerNode := GNode{Name: item.Caller.Func.Name(), Type: "func"}
		b1, _ := json.Marshal(&callerNode)
		if _, err := sqlitegraph.AddNode(callerNode.Name, b1, db); err != nil && err.Error() != "UNIQUE constraint failed: nodes.id"{
			slog.Error("error adding node", "error", err)
		}
		if item.Callee == nil {
			return
		}
		if item.Callee.Func.Pkg == nil {
			return
		}
		calleeNode := GNode{Name: item.Callee.Func.Name(), Type: "func"}
		b2, _ := json.Marshal(&calleeNode)
		if _, err := sqlitegraph.AddNode(calleeNode.Name, b2, db); err != nil && err.Error() != "UNIQUE constraint failed: nodes.id" {
			slog.Error("error adding node", "error", err)
		}
		if _, err := sqlitegraph.ConnectNodes(callerNode.Name, calleeNode.Name, db); err != nil {
			slog.Error("error adding edge", "error", err)
		}
		slog.Info(fmt.Sprintf("funtion %s calls function %s:", callerNode.Name, calleeNode.Name))
		traverseOut(db, item.Callee.Out, filterPacks)
	}
}
