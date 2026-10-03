package idbmap

import "github.com/chipskein/cade/internal/v8value"

// compiledCondition is a Condition ready to test items.
type compiledCondition struct {
	path    CompiledPath
	in      map[string]bool
	notIn   map[string]bool
	kindNot v8value.Kind
}

func compileConditions(conditions []Condition) ([]compiledCondition, error) {
	compiled := make([]compiledCondition, len(conditions))
	for i, condition := range conditions {
		path, err := CompilePath(condition.Path)
		if err != nil {
			return nil, err
		}
		kindNot, err := parseKind(condition.KindNot)
		if err != nil {
			return nil, err
		}
		compiled[i] = compiledCondition{path: path, in: stringSet(condition.In), notIn: stringSet(condition.NotIn), kindNot: kindNot}
	}
	return compiled, nil
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

// keeps reports whether item passes the condition. A missing value has
// kind undefined, as reading a missing property does in JavaScript.
func (c compiledCondition) keeps(item *v8value.Value) bool {
	value := c.path.First(item)
	if c.kindNot != noKind && kindOf(value) == c.kindNot {
		return false
	}
	text := scalarText(value)
	if len(c.in) > 0 && !c.in[text] {
		return false
	}
	return !c.notIn[text]
}

func kindOf(value *v8value.Value) v8value.Kind {
	if value == nil {
		return v8value.KindUndefined
	}
	return value.Kind
}

func keepsAll(conditions []compiledCondition, item *v8value.Value) bool {
	for _, condition := range conditions {
		if !condition.keeps(item) {
			return false
		}
	}
	return true
}
