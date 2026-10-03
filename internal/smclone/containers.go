package smclone

import (
	"fmt"
	"strconv"

	"github.com/chipskein/cade/internal/v8value"
)

func registerContainerDecoders(decoders map[uint32]tagDecoder) {
	decoders[tagObjectObject] = decodeObject
	decoders[tagArrayObject] = decodeArray
	decoders[tagMapObject] = decodeMap
	decoders[tagSetObject] = decodeSet
	decoders[tagBackReferenceObject] = decodeBackReference
}

func decodeObject(d *decoder, _ uint32) (*v8value.Value, error) {
	object := d.remember(&v8value.Value{Kind: v8value.KindObject})
	properties, err := d.readProperties()
	object.Properties = properties
	return object, err
}

// decodeArray reads an array, whose pair data is its length; elements come
// as index-keyed properties, like any other property.
func decodeArray(d *decoder, length uint32) (*v8value.Value, error) {
	array := d.remember(&v8value.Value{Kind: v8value.KindArray})
	properties, err := d.readProperties()
	if err != nil {
		return nil, err
	}
	array.Properties = properties
	promoteIndexProperties(array, uint64(length))
	return array, nil
}

// promoteIndexProperties moves index keys into Items (nil for holes),
// capped because an array can declare length 2^32-1 with one element.
func promoteIndexProperties(array *v8value.Value, length uint64) {
	array.Items = make([]*v8value.Value, min(length, maxSparseItems))
	var named []v8value.Property
	for _, property := range array.Properties {
		index, err := strconv.ParseUint(property.Key, 10, 64)
		if err != nil || index >= uint64(len(array.Items)) {
			named = append(named, property)
			continue
		}
		array.Items[index] = property.Value
	}
	array.Properties = named
}

// readProperties reads key/value pairs up to SCTAG_END_OF_KEYS.
func (d *decoder) readProperties() ([]v8value.Property, error) {
	var properties []v8value.Property
	for {
		ended, err := d.consumeEndOfKeys()
		if err != nil || ended {
			return properties, err
		}
		property, err := d.readProperty()
		if err != nil {
			return nil, err
		}
		properties = append(properties, property)
	}
}

func (d *decoder) readProperty() (v8value.Property, error) {
	key, err := d.readValue()
	if err != nil {
		return v8value.Property{}, err
	}
	name, err := propertyKey(key)
	if err != nil {
		return v8value.Property{}, err
	}
	value, err := d.readValue()
	return v8value.Property{Key: name, Value: value}, err
}

// propertyKey renders a key: SpiderMonkey writes index keys as int32 and
// every other key as a string.
func propertyKey(key *v8value.Value) (string, error) {
	switch key.Kind {
	case v8value.KindString:
		return key.Text, nil
	case v8value.KindNumber:
		return strconv.FormatFloat(key.Number, 'f', propertyKeyFloatPrecision, 64), nil
	}
	return "", fmt.Errorf("property key of kind %s, expected string or number", key.Kind)
}

func (d *decoder) consumeEndOfKeys() (bool, error) {
	tag, err := d.peekTag()
	if err != nil || tag != tagEndOfKeys {
		return false, err
	}
	_, err = d.readWord()
	return true, err
}

func decodeMap(d *decoder, _ uint32) (*v8value.Value, error) {
	mapping := d.remember(&v8value.Value{Kind: v8value.KindMap})
	for {
		ended, err := d.consumeEndOfKeys()
		if err != nil || ended {
			return mapping, err
		}
		key, err := d.readValue()
		if err != nil {
			return nil, err
		}
		value, err := d.readValue()
		if err != nil {
			return nil, err
		}
		mapping.Entries = append(mapping.Entries, v8value.MapEntry{Key: key, Value: value})
	}
}

func decodeSet(d *decoder, _ uint32) (*v8value.Value, error) {
	set := d.remember(&v8value.Value{Kind: v8value.KindSet})
	for {
		ended, err := d.consumeEndOfKeys()
		if err != nil || ended {
			return set, err
		}
		member, err := d.readValue()
		if err != nil {
			return nil, err
		}
		set.Items = append(set.Items, member)
	}
}

// decodeBackReference returns an object already read; data is its index in
// creation order. A slot still reserved (a typed array whose buffer refers
// back to it) is rejected rather than returned as nil.
func decodeBackReference(d *decoder, index uint32) (*v8value.Value, error) {
	if uint64(index) >= uint64(len(d.objects)) || d.objects[index] == nil {
		return nil, fmt.Errorf("back-reference to object %d at offset %d, only %d objects complete", index, d.pos-wordSize, len(d.objects))
	}
	return d.objects[index], nil
}
