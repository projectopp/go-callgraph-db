package sqlitegraph

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

func HandleGetAllData(w http.ResponseWriter, db *sql.DB) {
	response, err := GetAllData(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(response)
}

type GraphResponseDTO struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

func GetAllData(db *sql.DB) (*GraphResponseDTO, error) {
	response := GraphResponseDTO{}
	rows, err := db.Query("SELECT source, target FROM edges")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var edge Edge
		if err := rows.Scan(&edge.Source, &edge.Target); err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}
	response.Edges = edges

	rows2, err := db.Query("SELECT id, body FROM nodes")
	if err != nil {
		return nil, err
	}
	defer rows2.Close()

	var nodes []Node
	for rows2.Next() {
		var node Node
		if err := rows2.Scan(&node.ID, &node.Body); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	response.Nodes = nodes
	return &response, nil
}
