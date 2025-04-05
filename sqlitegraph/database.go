package sqlitegraph

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"text/template"

	_ "github.com/mattn/go-sqlite3"
)

const (
 	ID_CONSTRAINT        = "NOT NULL constraint failed: nodes.id"
	UNIQUE_ID_CONSTRAINT = "UNIQUE constraint failed: nodes.id"
	NO_ROWS_FOUND        = "sql: no rows in result set"
)

type NodeData struct {
	Identifier interface{} `json:"id"`
	Body       interface{}
}

type EdgeData struct {
	Source string
	Target string
	Label  string
}

type GraphData struct {
	Node NodeData
	Edge EdgeData
}

type SearchQuery struct {
	ResultColumn  string
	Key           string
	Tree          bool
	SearchClauses []string
}

type WhereClause struct {
	AndOr     string
	IdLookup  bool
	KeyValue  bool
	Key       string
	Tree      bool
	Predicate string
}

type Traversal struct {
	WithBodies bool
	Inbound    bool
	Outbound   bool
}

var (
	CLAUSE_TEMPLATE   = template.Must(template.New("where").Parse(SearchWhereTemplate))
	SEARCH_TEMPLATE   = template.Must(template.New("search").Parse(SearchNodeTemplate))
	TRAVERSE_TEMPLATE = template.Must(template.New("traverse").Parse(TraverseTemplate))
)

func evaluate(err error) {
	if err != nil {
		slog.Error("something went wrong", "error", err)
	}
}

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

func makeBulkInsertStatement(statement string, inserts int) string {
	pivot := "VALUES"
	parts := strings.Split(strings.TrimSpace(statement), pivot)
	if len(parts) == 2 {
		vals := make([]string, 0, inserts)
		for i := 0; i < inserts; i++ {
			vals = append(vals, parts[1])
		}
		return fmt.Sprintf("%s%s%s", parts[0], pivot, strings.Join(vals, ","))
	}
	return statement
}

func makeBulkEdgeInserts(sources []string, targets []string, properties []string) []interface{} {
	l := len(sources)
	if l != len(targets) && l != len(properties) {
		evaluate(errors.New("unequal edge lists"))
	}
	args := make([]interface{}, 0, l*3)
	for i := 0; i < l; i++ {
		args = append(args, sources[i])
		args = append(args, targets[i])
		args = append(args, properties[i])
	}
	return args
}

func insertMany(nodes []interface{}, db *sql.DB) (int64, error) {
	ins := func(db *sql.DB) (sql.Result, error) {
		stmt, stmtErr := db.Prepare(makeBulkInsertStatement(InsertNode, len(nodes)))
		evaluate(stmtErr)
		return stmt.Exec(nodes...)
	}

	in, inErr := ins(db)
	if inErr != nil {
		return 0, inErr
	}
	return in.RowsAffected()
}

func insertOne(node string, db *sql.DB) (int64, error) {
	ins := func(db *sql.DB) (sql.Result, error) {
		stmt, stmtErr := db.Prepare(InsertNode)
		evaluate(stmtErr)
		return stmt.Exec(node)
	}

	in, inErr := ins(db)
	if inErr != nil {
		return 0, inErr
	}
	return in.RowsAffected()
}

func connectMany(edges []interface{}, count int, db *sql.DB) (int64, error) {
	ins := func(db *sql.DB) (sql.Result, error) {
		stmt, stmtErr := db.Prepare(makeBulkInsertStatement(InsertEdge, count))
		evaluate(stmtErr)
		return stmt.Exec(edges...)
	}

	in, inErr := ins(db)
	if inErr != nil {
		return 0, inErr
	}
	return in.RowsAffected()
}

func needsIdentifier(node []byte) bool {
	var nodeData NodeData
	err := json.Unmarshal(node, &nodeData)
	evaluate(err)
	return nodeData.Identifier == nil
}

func setIdentifier(node []byte, identifier string) []byte {
	closingBraceIdx := bytes.LastIndexByte(node, '}')
	if closingBraceIdx > 0 {
		addId := []byte(fmt.Sprintf(", \"id\": %q", identifier))
		node = append(node[:closingBraceIdx], addId...)
		node = append(node, '}')
	}
	return node
}

func AddNode(identifier string, node []byte, db *sql.DB) (int64, error) {
	if needsIdentifier(node) {
		return insertOne(string(setIdentifier(node, identifier)), db)
	}
	return insertOne(string(node), db)
}

func AddNodes(identifiers []string, nodes [][]byte, db *sql.DB) (int64, error) {
	l := len(nodes)
	if l != len(identifiers) {
		evaluate(errors.New("unequal node, identifier lists"))
	}
	args := make([]interface{}, l)
	for i := 0; i < l; i++ {
		if needsIdentifier(nodes[i]) {
			args[i] = string(setIdentifier(nodes[i], identifiers[i]))
		} else {
			args[i] = string(nodes[i])
		}

	}
	return insertMany(args, db)
}

func ConnectNodesWithProperties(sourceId string, targetId string, properties []byte, db *sql.DB) (int64, error) {
	connect := func(db *sql.DB) (sql.Result, error) {
		stmt, stmtErr := db.Prepare(InsertEdge)
		evaluate(stmtErr)
		return stmt.Exec(sourceId, targetId, string(properties))
	}

	cx, cxErr := connect(db)
	if cxErr != nil {
		return 0, cxErr
	}
	return cx.RowsAffected()
}

func ConnectNodes(sourceId string, targetId string, db *sql.DB) (int64, error) {
	return ConnectNodesWithProperties(sourceId, targetId, []byte(`{}`), db)
}

func BulkConnectNodesWithProperties(sources []string, targets []string, properties []string, db *sql.DB) (int64, error) {
	l := len(sources)
	if l != len(targets) && l != len(properties) {
		evaluate(errors.New("unequal source, target, properties lists"))
	}
	return connectMany(makeBulkEdgeInserts(sources, targets, properties), l, db)
}

func BulkConnectNodes(sources []string, targets []string, db *sql.DB) (int64, error) {
	l := len(sources)
	props := make([]string, 0, l)
	for i := 0; i < l; i++ {
		props = append(props, `{}`)
	}
	return BulkConnectNodesWithProperties(sources, targets, props, db)
}

func RemoveNodes(identifiers []string, db *sql.DB) bool {
	delete := func(db *sql.DB) bool {
		edgeStmt, edgeErr := db.Prepare(DeleteEdge)
		evaluate(edgeErr)
		nodeStmt, nodeErr := db.Prepare(DeleteNode)
		evaluate(nodeErr)
		tx, txErr := db.Begin()
		evaluate(txErr)

		var err error
		for _, identifier := range identifiers {
			_, err = tx.Stmt(edgeStmt).Exec(identifier, identifier)
			if err != nil {
				tx.Rollback()
				return false
			}
			_, err = tx.Stmt(nodeStmt).Exec(identifier)
			if err != nil {
				tx.Rollback()
				return false
			}
		}
		tx.Commit()
		return true
	}

	return delete(db)
}

func FindNode(identifier string, db *sql.DB) (string, error) {
	find := func(db *sql.DB) (string, error) {
		clause := GenerateWhereClause(&WhereClause{IdLookup: true})
		search := GenerateSearchStatement(&SearchQuery{ResultColumn: "body", SearchClauses: []string{clause}})
		stmt, err := db.Prepare(search)
		evaluate(err)
		defer stmt.Close()
		var body string
		err = stmt.QueryRow(identifier).Scan(&body)
		if err == sql.ErrNoRows {
			return "", err
		}
		evaluate(err)
		return body, nil
	}

	return find(db)
}

func UpdateNodeBody(identifier string, body string, db *sql.DB) error {
	update := func(db *sql.DB) error {
		stmt, err := db.Prepare(UpdateNode)
		evaluate(err)
		defer stmt.Close()
		_, err = stmt.Exec(body, identifier)
		return err
	}

	return update(db)
}

func UpsertNode(identifier string, body string, db *sql.DB) error {
	update := []byte(body)
	node, err := FindNode(identifier, db)
	if node == "" && err == sql.ErrNoRows {
		_, err = AddNode(identifier, update, db)
		return err
	} else {
		if needsIdentifier(update) {
			return UpdateNodeBody(identifier, string(setIdentifier(update, identifier)), db)
		}
		return UpdateNodeBody(identifier, body, db)
	}
}

func GenerateWhereClause(properties *WhereClause) string {
	var clause bytes.Buffer
	err := CLAUSE_TEMPLATE.Execute(&clause, properties)
	evaluate(err)
	return clause.String()
}

func GenerateSearchStatement(properties *SearchQuery) string {
	var clause bytes.Buffer
	err := SEARCH_TEMPLATE.Execute(&clause, properties)
	evaluate(err)
	return clause.String()
}

func GenerateTraversal(properties *Traversal) string {
	var clause bytes.Buffer
	err := TRAVERSE_TEMPLATE.Execute(&clause, properties)
	evaluate(err)
	return clause.String()
}

func convertSearchBindingsToParameters(bindings []string) []interface{} {
	params := make([]interface{}, len(bindings))
	for i, binding := range bindings {
		params[i] = binding
	}
	return params
}

func FindNodes(statement string, bindings []string, db *sql.DB) ([]string, error) {
	find := func(db *sql.DB) ([]string, error) {
		stmt, stmtErr := db.Prepare(statement)
		evaluate(stmtErr)
		defer stmt.Close()

		results := []string{}
		rows, err := stmt.Query(convertSearchBindingsToParameters(bindings)...)
		if err != nil {
			results = append(results, "")
			return results, err
		}
		defer rows.Close()
		for rows.Next() {
			var body string
			err = rows.Scan(&body)
			if err != nil {
				results = append(results, "")
				return results, err
			}
			results = append(results, body)
		}
		err = rows.Err()
		return results, err
	}

	return find(db)
}

func traverse(source string, statement string, target string) func(*sql.DB) ([]string, error) {
	return func(db *sql.DB) ([]string, error) {
		stmt, stmtErr := db.Prepare(statement)
		evaluate(stmtErr)
		defer stmt.Close()

		results := []string{}
		rows, err := stmt.Query(source)
		if err != nil {
			results = append(results, "")
			return results, err
		}
		defer rows.Close()
		for rows.Next() {
			var identifier string
			err = rows.Scan(&identifier)
			if err != nil {
				results = append(results, "")
				return results, err
			}
			results = append(results, identifier)
			if len(target) > 0 && identifier == target {
				break
			}
		}
		err = rows.Err()
		return results, err
	}
}

func TraverseFromTo(source string, target string, traversal string, db *sql.DB) ([]string, error) {
	fn := traverse(source, traversal, target)
	return fn(db)
}

func TraverseFrom(source string, traversal string, db *sql.DB) ([]string, error) {
	fn := traverse(source, traversal, "")
	return fn(db)
}

func traverseWithBodies(source string, statement string, target string) func(*sql.DB) ([]GraphData, error) {
	return func(db *sql.DB) ([]GraphData, error) {
		stmt, stmtErr := db.Prepare(statement)
		evaluate(stmtErr)
		defer stmt.Close()

		results := []GraphData{}
		rows, err := stmt.Query(source)
		if err != nil {
			return results, err
		}
		defer rows.Close()

		currentId := ""
		for rows.Next() {
			var identifier string
			var object string
			var body string
			err = rows.Scan(&identifier, &object, &body)
			if err != nil {
				return results, err
			}
			if object == "()" {
				currentId = identifier
				results = append(results, GraphData{Node: NodeData{Identifier: identifier, Body: body}})
			} else {
				if object == "->" {
					results = append(results, GraphData{Edge: EdgeData{Source: currentId, Target: identifier, Label: body}})
				} else {
					results = append(results, GraphData{Edge: EdgeData{Source: identifier, Target: currentId, Label: body}})
				}
			}
			if len(target) > 0 && identifier == target && object == "()" {
				break
			}
		}
		err = rows.Err()
		return results, err
	}
}

func TraverseWithBodiesFromTo(source string, target string, traversal string, db *sql.DB) ([]GraphData, error) {
	fn := traverseWithBodies(source, traversal, target)
	return fn(db)
}

func TraverseWithBodiesFrom(source string, traversal string, db *sql.DB) ([]GraphData, error) {
	fn := traverseWithBodies(source, traversal, "")
	return fn(db)
}

func neighbors(statement string, queryBinding func(*sql.Stmt) (*sql.Rows, error)) func(*sql.DB) ([]EdgeData, error) {
	return func(db *sql.DB) ([]EdgeData, error) {
		stmt, stmtErr := db.Prepare(statement)
		evaluate(stmtErr)
		defer stmt.Close()

		results := []EdgeData{}
		rows, err := queryBinding(stmt)
		if err != nil {
			results = append(results, EdgeData{})
			return results, err
		}
		defer rows.Close()
		for rows.Next() {
			var result EdgeData
			var source string
			var target string
			var label string
			err = rows.Scan(&source, &target, &label)
			if err != nil {
				results = append(results, result)
				return results, err
			}
			result.Source = source
			result.Target = target
			if len(label) > 0 {
				result.Label = label
			}
			results = append(results, result)
		}
		err = rows.Err()
		return results, err
	}
}

func getConnectionsOneWay(identifier string, direction string, db *sql.DB) ([]EdgeData, error) {
	query := func(stmt *sql.Stmt) (*sql.Rows, error) {
		return stmt.Query(identifier)
	}
	fn := neighbors(direction, query)
	return fn(db)
}

func ConnectionsIn(identifier string, db *sql.DB) ([]EdgeData, error) {
	return getConnectionsOneWay(identifier, SearchEdgesInbound, db)
}

func ConnectionsOut(identifier string, db *sql.DB) ([]EdgeData, error) {
	return getConnectionsOneWay(identifier, SearchEdgesOutbound, db)
}

func Connections(identifier string, db *sql.DB) ([]EdgeData, error) {
	query := func(stmt *sql.Stmt) (*sql.Rows, error) {
		return stmt.Query(identifier, identifier)
	}
	fn := neighbors(SearchEdges, query)
	return fn(db)
}
