package nbt

import (
	"encoding/json"
	"fmt"
	"math"
)

// EncodeJSON emits the deterministic typed JSON representation consumed by
// DecodeJSON. List element types are written once, and long values use the
// prismarine-nbt/ProtoDef signed [high, low] int32 pair representation.
func EncodeJSON(value Value) ([]byte, error) {
	if value == nil {
		return []byte("null"), nil
	}
	node, err := encodeJSONNode(value)
	if err != nil {
		return nil, fmt.Errorf("nbt json: %w", err)
	}
	encoded, err := json.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("nbt json: marshal: %w", err)
	}
	return encoded, nil
}

type encodedJSONNode struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type encodedJSONList struct {
	Type  string `json:"type"`
	Value []any  `json:"value"`
}

func encodeJSONNode(value Value) (encodedJSONNode, error) {
	typeName, err := prismarineTypeName(value.Type())
	if err != nil {
		return encodedJSONNode{}, err
	}
	payload, err := encodeJSONPayload(value)
	if err != nil {
		return encodedJSONNode{}, err
	}
	return encodedJSONNode{Type: typeName, Value: payload}, nil
}

func encodeJSONPayload(value Value) (any, error) {
	switch value := value.(type) {
	case Byte:
		return int8(value), nil
	case Short:
		return int16(value), nil
	case Int:
		return int32(value), nil
	case Long:
		return longPair(int64(value)), nil
	case Float:
		v := float32(value)
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return nil, fmt.Errorf("float is not finite")
		}
		return v, nil
	case Double:
		v := float64(value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("double is not finite")
		}
		return v, nil
	case ByteArray:
		items := make([]int8, len(value))
		for i, entry := range value {
			items[i] = int8(entry)
		}
		return items, nil
	case String:
		return string(value), nil
	case List:
		typeName, err := prismarineTypeName(value.ElementType)
		if err != nil {
			return nil, fmt.Errorf("list element type: %w", err)
		}
		if value.ElementType == End && len(value.Values) != 0 {
			return nil, fmt.Errorf("end element type requires an empty list")
		}
		items := make([]any, len(value.Values))
		for i, entry := range value.Values {
			if entry == nil {
				return nil, fmt.Errorf("list element %d is nil", i)
			}
			if entry.Type() != value.ElementType {
				return nil, fmt.Errorf("list element %d type %d does not match declared type %d", i, entry.Type(), value.ElementType)
			}
			payload, err := encodeJSONPayload(entry)
			if err != nil {
				return nil, fmt.Errorf("list element %d: %w", i, err)
			}
			items[i] = payload
		}
		return encodedJSONList{Type: typeName, Value: items}, nil
	case Compound:
		fields := make(map[string]encodedJSONNode, len(value))
		for key, entry := range value {
			if entry == nil {
				return nil, fmt.Errorf("compound key %q is nil", key)
			}
			node, err := encodeJSONNode(entry)
			if err != nil {
				return nil, fmt.Errorf("compound key %q: %w", key, err)
			}
			fields[key] = node
		}
		return fields, nil
	case IntArray:
		return append([]int32(nil), value...), nil
	case LongArray:
		items := make([][2]int32, len(value))
		for i, entry := range value {
			items[i] = longPair(entry)
		}
		return items, nil
	default:
		return nil, fmt.Errorf("unsupported value implementation %T", value)
	}
}

func longPair(value int64) [2]int32 {
	return [2]int32{int32(value >> 32), int32(value)}
}

func prismarineTypeName(typeID Type) (string, error) {
	switch typeID {
	case End:
		return "end", nil
	case ByteType:
		return "byte", nil
	case ShortType:
		return "short", nil
	case IntType:
		return "int", nil
	case LongType:
		return "long", nil
	case FloatType:
		return "float", nil
	case DoubleType:
		return "double", nil
	case ByteArrayType:
		return "byteArray", nil
	case StringType:
		return "string", nil
	case ListType:
		return "list", nil
	case CompoundType:
		return "compound", nil
	case IntArrayType:
		return "intArray", nil
	case LongArrayType:
		return "longArray", nil
	default:
		return "", fmt.Errorf("invalid tag type %d", typeID)
	}
}
