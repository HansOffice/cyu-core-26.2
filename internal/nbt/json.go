package nbt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
)

// DecodeJSON decodes CyuCore's canonical typed-JSON representation of one NBT
// value. The representation is intentionally explicit about numeric NBT types;
// generated registry data must not rely on JSON number inference.
func DecodeJSON(data []byte) (Value, error) {
	value, err := decodeJSONValue(data)
	if err != nil {
		return nil, fmt.Errorf("nbt json: %w", err)
	}
	return value, nil
}

type jsonValue struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

func decodeJSONValue(data []byte) (Value, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var node jsonValue
	if err := decoder.Decode(&node); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, fmt.Errorf("trailing data: %w", err)
	}
	if node.Type == "" {
		return nil, fmt.Errorf("missing type")
	}
	if node.Value == nil {
		return nil, fmt.Errorf("type %q is missing value", node.Type)
	}

	switch node.Type {
	case "byte":
		value, err := parseSignedInteger(node.Value, 8)
		return Byte(value), err
	case "short":
		value, err := parseSignedInteger(node.Value, 16)
		return Short(value), err
	case "int":
		value, err := parseSignedInteger(node.Value, 32)
		return Int(value), err
	case "long":
		value, err := parseSignedInteger(node.Value, 64)
		return Long(value), err
	case "float":
		value, err := parseFloat(node.Value, 32)
		return Float(value), err
	case "double":
		value, err := parseFloat(node.Value, 64)
		return Double(value), err
	case "string":
		var value string
		if err := json.Unmarshal(node.Value, &value); err != nil {
			return nil, fmt.Errorf("string: %w", err)
		}
		return String(value), nil
	case "byte_array":
		var raw []json.RawMessage
		if err := json.Unmarshal(node.Value, &raw); err != nil {
			return nil, fmt.Errorf("byte_array: %w", err)
		}
		value := make(ByteArray, len(raw))
		for i, item := range raw {
			integer, err := parseSignedInteger(item, 8)
			if err != nil {
				return nil, fmt.Errorf("byte_array[%d]: %w", i, err)
			}
			value[i] = byte(int8(integer))
		}
		return value, nil
	case "int_array":
		var raw []json.RawMessage
		if err := json.Unmarshal(node.Value, &raw); err != nil {
			return nil, fmt.Errorf("int_array: %w", err)
		}
		value := make(IntArray, len(raw))
		for i, item := range raw {
			integer, err := parseSignedInteger(item, 32)
			if err != nil {
				return nil, fmt.Errorf("int_array[%d]: %w", i, err)
			}
			value[i] = int32(integer)
		}
		return value, nil
	case "long_array":
		var raw []json.RawMessage
		if err := json.Unmarshal(node.Value, &raw); err != nil {
			return nil, fmt.Errorf("long_array: %w", err)
		}
		value := make(LongArray, len(raw))
		for i, item := range raw {
			integer, err := parseSignedInteger(item, 64)
			if err != nil {
				return nil, fmt.Errorf("long_array[%d]: %w", i, err)
			}
			value[i] = integer
		}
		return value, nil
	case "list":
		var raw []json.RawMessage
		if err := json.Unmarshal(node.Value, &raw); err != nil {
			return nil, fmt.Errorf("list: %w", err)
		}
		values := make([]Value, len(raw))
		for i, item := range raw {
			value, err := decodeJSONValue(item)
			if err != nil {
				return nil, fmt.Errorf("list[%d]: %w", i, err)
			}
			values[i] = value
		}
		list, err := NewList(values...)
		if err != nil {
			return nil, err
		}
		return list, nil
	case "compound":
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(node.Value, &raw); err != nil {
			return nil, fmt.Errorf("compound: %w", err)
		}
		value := make(Compound, len(raw))
		for key, item := range raw {
			entry, err := decodeJSONValue(item)
			if err != nil {
				return nil, fmt.Errorf("compound key %q: %w", key, err)
			}
			value[key] = entry
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unknown type %q", node.Type)
	}
}

func parseSignedInteger(raw []byte, bits int) (int64, error) {
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return 0, fmt.Errorf("integer: %w", err)
	}
	value, err := strconv.ParseInt(number.String(), 10, bits)
	if err != nil {
		return 0, fmt.Errorf("integer %q does not fit int%d", number.String(), bits)
	}
	return value, nil
}

func parseFloat(raw []byte, bits int) (float64, error) {
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return 0, fmt.Errorf("float: %w", err)
	}
	value, err := strconv.ParseFloat(number.String(), bits)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, fmt.Errorf("invalid float%d %q", bits, number.String())
	}
	return value, nil
}
