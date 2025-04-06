package sqlitegraph

type NodeBody struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type Node struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}

type Edge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}
