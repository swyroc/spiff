package flow

import (
	"github.com/mandelsoft/spiff/dynaml"
	"github.com/mandelsoft/spiff/yaml"
)

func PrepareStubs(outer dynaml.Binding, partial bool, stubs ...yaml.Node) ([]yaml.Node, error) {
	for i := len(stubs) - 1; i >= 0; i-- {
		flowed, err := NestedFlow(outer, stubs[i], stubs[i+1:]...)
		if !partial && err != nil {
			return nil, err
		}

		stubs[i] = Cleanup(flowed, testLocal)
	}
	return stubs, nil
}

func Apply(outer dynaml.Binding, template yaml.Node, prepared []yaml.Node) (yaml.Node, error) {
	result, err := NestedFlow(outer, template, prepared...)
	if err == nil {
		result = Cleanup(result, testTemporary)
	}
	return result, err
}

func Cascade(outer dynaml.Binding, template yaml.Node, partial bool, stubs ...yaml.Node) (yaml.Node, error) {
	prepared, err := PrepareStubs(outer, partial, stubs...)
	if err != nil {
		return nil, err
	}

	return Apply(outer, template, prepared)
}

func testTemporary(node yaml.Node) bool {
	return node.Temporary() || node.Local()
}
func testLocal(node yaml.Node) bool {
	return node.Local()
}

func Cleanup(node yaml.Node, test func(yaml.Node) bool) yaml.Node {
	if node == nil {
		return nil
	}
	value := node.Value()
	switch v := value.(type) {
	case []yaml.Node:
		r := []yaml.Node{}
		for _, e := range v {
			if !test(e) {
				r = append(r, Cleanup(e, test))
			}
		}
		value = r

	case map[string]yaml.Node:
		r := map[string]yaml.Node{}
		for k, e := range v {
			if !test(e) {
				r[k] = Cleanup(e, test)
			}
		}
		value = r
	}
	return yaml.ReplaceValue(value, node)
}
