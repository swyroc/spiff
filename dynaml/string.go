package dynaml

import (
	"github.com/vito/spiff/yaml"
)

type StringExpr struct {
	Value string
}

func (e StringExpr) Evaluate(Context) yaml.Node {
	return e.Value
}