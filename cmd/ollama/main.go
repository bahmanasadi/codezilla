package main

import (
	"codezilla/internal/graph"
	"fmt"
)

func main() {
	state := map[string]interface{}{}
	g := graph.New()
	if err := g.Execute(state); err != nil {
		fmt.Println("Graph execution error:", err)
	}
}
