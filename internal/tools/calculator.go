package tools

import (
	"fmt"

	"github.com/Knetic/govaluate"
)

func Calculate(expression string) (string, error) {
	exp, err := govaluate.NewEvaluableExpression(expression)
	if err != nil {
		return "", err
	}
	result, err := exp.Evaluate(nil)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Result: %v", result), nil
}
