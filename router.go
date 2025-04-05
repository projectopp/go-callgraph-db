package main

import (
	"database/sql"
	"go-callgraph-db/sqlitegraph"
	"net/http"
)

func RouteHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sqlitegraph.HandleGetAllData(w, db)
	}
}
