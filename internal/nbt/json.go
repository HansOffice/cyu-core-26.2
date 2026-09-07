package nbt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
)

// DecodeJSON decodes the typed JSON shape emitted by prismarine-nbt and the
// selected Minecraft 26.2 data generator. Numeric NBT types are explicit, list
// element types are carried once by the list payload, and 64-bit integers use
// the ProtoDef [high, low] signed-int32 pair representation.
func DecodeJSON(data []byte) (Value, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var node jsonValue
	if err := decoder.Decode(&node); err != nil {
		return nil, fmt.Errorf("nbt json: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("nbt json: multiple JSON values")
		}
		return nil, fmt.Errorf("nbt json: trailing data: %w", err)
	}

	value, err := decodeJSONNode(node)
	if err != nil {
		return nil, fmt.Errorf("nbt json: %w", err)
	}
	return value, nil
}

type jsonValue struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

type jsonList struct {
	Type  string            `json:"type"`
	Value []json.RawMessage `json:"value"`
}

func decodeJSONNode(node jsonValue) (Value, error) {
	if node.Type == "" {
		return nil, fmt.Errorf("missing type")
	}
	if node.Value == nil {
		return nil, fmt.Errorf("type %q is missing value", node.Type)
	}
	return decodeJSONPayload(node.Type, node.Value)
}

func decodeJSONPayload(typeName string, raw json.RawMessage) (Value, error) {
	switch typeName {
	case "byte":
		value, err := parseSignedInteger(raw, 8)
		return Byte(value), err
	case "short":
		value, err := parseSignedInteger(raw, 16)
		return Short(value), err
	case "int":
		value, err := parseSignedInteger(raw, 32)
		return Int(value), err
	case "long":
		value, err := parseLong(raw)
		return Long(value), err
	case "float":
		value, err := parseFloat(raw, 32)
		return Float(value), err
	case "double":
		value, err := parseFloat(raw, 64)
		return Double(value), err
	case "string":
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("string: %w", err)
		}
		return String(value), nil
	case "byteArray":
		return parseByteArray(raw)
	case "intArray":
		return parseIntArray(raw)
	case "longArray":
		return parseLongArray(raw)
	case "shortArray":
		return nil, fmt.Errorf("shortArray is not a Java Edition network NBT tag")
	case "list":
		return parseList(raw)
	case "compound":
		return parseCompound(raw)
	default:
		return nil, fmt.Errorf("unknown type %q", typeName)
	}
}

func parseCompound(raw json.RawMessage) (Value, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("compound: %w", err)
	}
	value := make(Compound, len(fields))
	for key, field := range fields {
		if bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
			continue
		}
		var node jsonValue
		decoder := json.NewDecoder(bytes.NewReader(field))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&node); err != nil {
			return nil, fmt.Errorf("compound key %q: %w", key, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			if err == nil {
				return nil, fmt.Errorf("compound key %q: multiple JSON values", key)
			}
			return nil, fmt.Errorf("compound key %q: trailing data: %w", key, err)
		}
		entry, err := decodeJSONNode(node)
		if err != nil {
			return nil, fmt.Errorf("compound key %q: %w", key, err)
		}
		value[key] = entry
	}
	return value, nil
}

func parseList(raw json.RawMessage) (Value, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var list jsonList
	if err := decoder.Decode(&list); err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("list: multiple JSON values")
		}
		return nil, fmt.Errorf("list: trailing data: %w", err)
	}

	elementType, err := prismarineType(list.Type)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	if elementType == End && len(list.Value) != 0 {
		return nil, fmt.Errorf("list: end element type requires an empty list")
	}
	values := make([]Value, len(list.Value))
	for i, item := range list.Value {
		entry, err := decodeJSONPayload(list.Type, item)
		if err != nil {
			return nil, fmt.Errorf("list[%d]: %w", i, err)
		}
		if entry.Type() != elementType {
			return nil, fmt.Errorf("list[%d]: decoded type %d does not match declared type %d", i, entry.Type(), elementType)
		}
		values[i] = entry
	}
	return List{ElementType: elementType, Values: values}, nil
}

func prismarineType(typeName string) (Type, error) {
	switch typeName {
	case "end":
		return End, nil
	case "byte":
		return ByteType, nil
	case "short":
		return ShortType, nil
	case "int":
		return IntType, nil
	case "long":
		return LongType, nil
	case "float":
		return FloatType, nil
	case "double":
		return DoubleType, nil
	case "byteArray":
		return ByteArrayType, nil
	case "string":
		return StringType, nil
	case "list":
		return ListType, nil
	case "compound":
		return CompoundType, nil
	case "intArray":
		return IntArrayType, nil
	case "longArray":
		return LongArrayType, nil
	case "shortArray":
		return End, fmt.Errorf("shortArray is not a Java Edition network NBT tag")
	default:
		return End, fmt.Errorf("unknown type %q", typeName)
	}
}

func parseByteArray(raw json.RawMessage) (Value, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("byteArray: %w", err)
	}
	value := make(ByteArray, len(items))
	for i, item := range items {
		integer, err := parseSignedInteger(item, 8)
		if err != nil {
			return nil, fmt.Errorf("byteArray[%d]: %w", i, err)
		}
		value[i] = byte(int8(integer))
	}
	return value, nil
}

func parseIntArray(raw json.RawMessage) (Value, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("intArray: %w", err)
	}
	value := make(IntArray, len(items))
	for i, item := range items {
		integer, err := parseSignedInteger(item, 32)
		if err != nil {
			return nil, fmt.Errorf("intArray[%d]: %w", i, err)
		}
		value[i] = int32(integer)
	}
	return value, nil
}

func parseLongArray(raw json.RawMessage) (Value, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("longArray: %w", err)
	}
	value := make(LongArray, len(items))
	for i, item := range items {
		integer, err := parseLong(item)
		if err != nil {
			return nil, fmt.Errorf("longArray[%d]: %w", i, err)
		}
		value[i] = integer
	}
	return value, nil
}

func parseLong(raw json.RawMessage) (int64, error) {
	var pair []json.RawMessage
	if err := json.Unmarshal(raw, &pair); err != nil {
		return 0, fmt.Errorf("long: expected [high, low] int32 pair: %w", err)
	}
	if len(pair) != 2 {
		return 0, fmt.Errorf("long: expected [high, low] int32 pair, got %d elements", len(pair))
	}
	high, err := parseSignedInteger(pair[0], 32)
	if err != nil {
		return 0, fmt.Errorf("long high: %w", err)
	}
	low, err := parseSignedInteger(pair[1], 32)
	if err != nil {
		return 0, fmt.Errorf("long low: %w", err)
	}
	return int64(int32(high))<<32 | int64(uint32(int32(low))), nil
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
