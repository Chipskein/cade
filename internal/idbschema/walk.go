package idbschema

import "github.com/chipskein/cade/internal/v8value"

// Path notation: ".key" for object properties, "[]" for array elements,
// "{}" for map values and "<>" for set members.
const (
	rootPath   = "$"
	arrayStep  = "[]"
	mapStep    = "{}"
	setStep    = "<>"
	propertyOp = "."
)

type observeFunc func(path string, kind v8value.Kind)

// walkValue reports every (path, kind) under value. visiting guards against
// cycles created by V8 back-references.
func walkValue(path string, value *v8value.Value, depth int, visiting map[*v8value.Value]bool, observe observeFunc) {
	if value == nil || depth > maxWalkDepth || visiting[value] {
		return
	}
	if path == "" {
		path = rootPath
	}
	observe(path, value.Kind)
	visiting[value] = true
	defer delete(visiting, value)
	walkChildren(path, value, depth+1, visiting, observe)
}

func walkChildren(path string, value *v8value.Value, depth int, visiting map[*v8value.Value]bool, observe observeFunc) {
	for _, property := range value.Properties {
		walkValue(path+propertyOp+MaskKey(property.Key), property.Value, depth, visiting, observe)
	}
	step := arrayStep
	if value.Kind == v8value.KindSet {
		step = setStep
	}
	for _, item := range value.Items {
		walkValue(path+step, item, depth, visiting, observe)
	}
	for _, entry := range value.Entries {
		walkValue(path+mapStep, entry.Value, depth, visiting, observe)
	}
}
