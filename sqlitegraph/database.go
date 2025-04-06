package sqlitegraph

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const (
	ID_CONSTRAINT        = "NOT NULL constraint failed: nodes.id"
	UNIQUE_ID_CONSTRAINT = "UNIQUE constraint failed: nodes.id"
	NO_ROWS_FOUND        = "sql: no rows in result set"
)


func Initialize(db *sql.DB) error {
	for _, statement := range strings.Split(Schema, ";") {
		sql := strings.TrimSpace(statement)
		if len(sql) > 0 {
			stmt, err := db.Prepare(sql)
			if err != nil {
				return err
			}
			_, err = stmt.Exec()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func AddNode(nodeBody NodeBody, db *sql.DB) error {
 	stmt, stmtErr := db.Prepare(InsertNode)
	if stmtErr != nil {
		return stmtErr
	}
	b, _ := json.Marshal(nodeBody)
	_, err := stmt.Exec(string(b))
	return err
}

func AddEdge(edge Edge, db *sql.DB) error {
	stmt, stmtErr := db.Prepare(InsertEdge)
   if stmtErr != nil {
	   return stmtErr
   }
    _, err := stmt.Exec(edge.Source, edge.Target, nil)
   return err
}


func AddNodes(nodes []Node, db *sql.DB) error {
	var queryBuilder strings.Builder
	queryBuilder.WriteString(InsertNode)
	for i := 0; i < len(nodes); i++ {
		b, _ := json.Marshal(nodes[i])
		queryBuilder.WriteString(fmt.Sprintf("('%s')", string(b)))
		if i < len(nodes)-1 {
			queryBuilder.WriteString(",\n")
		} else {
			queryBuilder.WriteString(";")
		}
	}
	_, err := db.Exec(queryBuilder.String())
	return err
}

func AddEdges(edges []Edge, db *sql.DB) error {
	var queryBuilder strings.Builder
	queryBuilder.WriteString("INSERT INTO edges (source, target) VALUES\n")
	for i := 0; i < len(edges); i++ {
		body := strings.ReplaceAll(edges[i].Source, "'", "''")
		id := strings.ReplaceAll(edges[i].Target, "'", "''")
		queryBuilder.WriteString(fmt.Sprintf("('%s', '%s')", id, body))
		if i < len(edges)-1 {
			queryBuilder.WriteString(",\n")
		} else {
			queryBuilder.WriteString(";")
		}
	}
	_, err := db.Exec(queryBuilder.String())
	return err
}