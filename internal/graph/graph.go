package graph

type Node struct {
	Name     string
	Function func(state map[string]interface{}) (string, error)
}

type Graph struct {
	Nodes map[string]*Node
	Start string
}

func (g *Graph) Execute(state map[string]interface{}) error {
	current := g.Start
	for current != "" {
		node := g.Nodes[current]
		next, err := node.Function(state)
		if err != nil {
			return err
		}
		current = next
	}
	return nil
}
