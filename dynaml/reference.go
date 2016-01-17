package dynaml

import (
	"fmt"
	"strings"

	"github.com/cloudfoundry-incubator/spiff/debug"
	"github.com/cloudfoundry-incubator/spiff/yaml"
)

type ReferenceExpr struct {
	Path []string
}

func (e ReferenceExpr) Evaluate(binding Binding) (yaml.Node, EvaluationInfo, bool) {
	fromRoot := e.Path[0] == ""

	debug.Debug("reference: %v\n", e.Path)
	return e.find(func(end int, path []string) (yaml.Node, bool) {
		if fromRoot {
			return binding.FindFromRoot(path[1 : end+1])
		} else {
			return binding.FindReference(path[:end+1])
		}
	})
}

func (e ReferenceExpr) String() string {
	return strings.Join(e.Path, ".")
}

func (e ReferenceExpr) find(f func(int, []string) (yaml.Node, bool)) (yaml.Node, EvaluationInfo, bool) {
	var step yaml.Node
	var ok bool

	info := DefaultInfo()
	for i := 0; i < len(e.Path); i++ {
		step, ok = f(i, e.Path)

		debug.Debug("  %d: %v %#v\n", i, ok, step)
		if !ok {
			info.Issue = fmt.Sprintf("'%s' not found", strings.Join(e.Path, "."))
			return nil, info, false
		}

		if !isLocallyResolved(step) {
			debug.Debug("  locally unresolved\n")
			return node(e), info, true
		}
	}

	if !isResolved(step) {
		debug.Debug("  unresolved\n")
		return node(e), info, true
	}

	debug.Debug("reference %v -> %+v\n", e.Path, step)
	return yaml.ReferencedNode(step), info, true
}
