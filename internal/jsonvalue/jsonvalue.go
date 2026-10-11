// Package jsonvalue parses JSON into the value tree browser storages decode
// into (v8value.Value), so a JSON response from a request cache is mapped
// by the same schema paths as an IndexedDB record.
package jsonvalue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/chipskein/cade/internal/v8value"
)

// Parse reads one JSON document, keeping the order of object keys as the
// IndexedDB readers do.
//
//	value, err := jsonvalue.Parse([]byte(`[{"content":"oi"}]`))
func Parse(raw []byte) (*v8value.Value, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	value, err := readValue(decoder)
	if err != nil {
		return nil, fmt.Errorf("parse %d bytes of JSON: %w", len(raw), err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse %d bytes of JSON: data after the first value, expected one document", len(raw))
	}
	return value, nil
}

func readValue(decoder *json.Decoder) (*v8value.Value, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	return valueOf(decoder, token)
}

func valueOf(decoder *json.Decoder, token json.Token) (*v8value.Value, error) {
	switch typed := token.(type) {
	case json.Delim:
		return readContainer(decoder, typed)
	case string:
		return &v8value.Value{Kind: v8value.KindString, Text: typed}, nil
	case float64:
		return &v8value.Value{Kind: v8value.KindNumber, Number: typed}, nil
	case bool:
		return &v8value.Value{Kind: v8value.KindBool, Bool: typed}, nil
	default:
		return &v8value.Value{Kind: v8value.KindNull}, nil
	}
}

func readContainer(decoder *json.Decoder, open json.Delim) (*v8value.Value, error) {
	var value *v8value.Value
	var err error
	if open == '{' {
		value, err = readObject(decoder)
	} else {
		value, err = readArray(decoder)
	}
	if err != nil {
		return nil, err
	}
	// Consumes the closing delimiter, which More has already seen.
	_, err = decoder.Token()
	return value, err
}

func readObject(decoder *json.Decoder) (*v8value.Value, error) {
	object := &v8value.Value{Kind: v8value.KindObject}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		value, err := readValue(decoder)
		if err != nil {
			return nil, err
		}
		object.Properties = append(object.Properties, v8value.Property{Key: key.(string), Value: value})
	}
	return object, nil
}

func readArray(decoder *json.Decoder) (*v8value.Value, error) {
	array := &v8value.Value{Kind: v8value.KindArray}
	for decoder.More() {
		item, err := readValue(decoder)
		if err != nil {
			return nil, err
		}
		array.Items = append(array.Items, item)
	}
	return array, nil
}
