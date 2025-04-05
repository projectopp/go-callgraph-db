package sqlitegraph

type Node struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}

type Edge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}
