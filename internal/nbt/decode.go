package nbt

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// ReadNetwork reads the anonymous NBT form used on the Minecraft network: one
// tag type byte followed immediately by that tag's payload. TAG_End is not a
// valid standalone value; optional packet fields must consume their own
// presence marker before calling this function.
func ReadNetwork(r io.Reader) (Value, error) {
	if r == nil {
		return nil, fmt.Errorf("nbt: nil reader")
	}
	typeByte, err := readByte(r)
	if err != nil {
		return nil, fmt.Errorf("nbt: read root type: %w", err)
	}
	typeID := Type(typeByte)
	if !typeID.validValueType() {
		return nil, fmt.Errorf("nbt: invalid network root type %d", typeID)
	}
	value, err := readPayload(r, typeID, 0)
	if err != nil {
		return nil, fmt.Errorf("nbt: root: %w", err)
	}
	return value, nil
}

func readPayload(r io.Reader, typeID Type, depth int) (Value, error) {
	if depth > defaultMaxDepth {
		return nil, fmt.Errorf("nesting exceeds %d levels", defaultMaxDepth)
	}

	switch typeID {
	case ByteType:
		value, err := readByte(r)
		return Byte(int8(value)), err
	case ShortType:
		value, err := readUint16(r)
		return Short(int16(value)), err
	case IntType:
		value, err := readUint32(r)
		return Int(int32(value)), err
	case LongType:
		value, err := readUint64(r)
		return Long(int64(value)), err
	case FloatType:
		value, err := readUint32(r)
		return Float(math.Float32frombits(value)), err
	case DoubleType:
		value, err := readUint64(r)
		return Double(math.Float64frombits(value)), err
	case ByteArrayType:
		length, err := readLength(r)
		if err != nil {
			return nil, err
		}
		value := make(ByteArray, length)
		if _, err := io.ReadFull(r, value); err != nil {
			return nil, err
		}
		return value, nil
	case StringType:
		value, err := readModifiedUTF8(r)
		return String(value), err
	case ListType:
		return readList(r, depth)
	case CompoundType:
		return readCompound(r, depth)
	case IntArrayType:
		length, err := readLength(r)
		if err != nil {
			return nil, err
		}
		value := make(IntArray, length)
		for i := range value {
			entry, err := readUint32(r)
			if err != nil {
				return nil, fmt.Errorf("int array element %d: %w", i, err)
			}
			value[i] = int32(entry)
		}
		return value, nil
	case LongArrayType:
		length, err := readLength(r)
		if err != nil {
			return nil, err
		}
		value := make(LongArray, length)
		for i := range value {
			entry, err := readUint64(r)
			if err != nil {
				return nil, fmt.Errorf("long array element %d: %w", i, err)
			}
			value[i] = int64(entry)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported tag type %d", typeID)
	}
}

func readList(r io.Reader, depth int) (Value, error) {
	elementByte, err := readByte(r)
	if err != nil {
		return nil, fmt.Errorf("list element type: %w", err)
	}
	elementType := Type(elementByte)
	length, err := readLength(r)
	if err != nil {
		return nil, fmt.Errorf("list length: %w", err)
	}
	if length == 0 {
		if elementType != End && !elementType.validValueType() {
			return nil, fmt.Errorf("invalid empty-list element type %d", elementType)
		}
		return List{ElementType: elementType}, nil
	}
	if !elementType.validValueType() {
		return nil, fmt.Errorf("invalid list element type %d", elementType)
	}
	values := make([]Value, length)
	for i := range values {
		value, err := readPayload(r, elementType, depth+1)
		if err != nil {
			return nil, fmt.Errorf("list element %d: %w", i, err)
		}
		values[i] = value
	}
	return List{ElementType: elementType, Values: values}, nil
}

func readCompound(r io.Reader, depth int) (Value, error) {
	value := make(Compound)
	for {
		typeByte, err := readByte(r)
		if err != nil {
			return nil, fmt.Errorf("compound entry type: %w", err)
		}
		typeID := Type(typeByte)
		if typeID == End {
			return value, nil
		}
		if !typeID.validValueType() {
			return nil, fmt.Errorf("invalid compound entry type %d", typeID)
		}
		name, err := readModifiedUTF8(r)
		if err != nil {
			return nil, fmt.Errorf("compound entry name: %w", err)
		}
		if _, exists := value[name]; exists {
			return nil, fmt.Errorf("duplicate compound key %q", name)
		}
		entry, err := readPayload(r, typeID, depth+1)
		if err != nil {
			return nil, fmt.Errorf("compound key %q: %w", name, err)
		}
		value[name] = entry
	}
}

func readLength(r io.Reader) (int, error) {
	value, err := readUint32(r)
	if err != nil {
		return 0, err
	}
	length := int32(value)
	if length < 0 {
		return 0, fmt.Errorf("negative collection length %d", length)
	}
	return int(length), nil
}

func readByte(r io.Reader) (byte, error) {
	var buf [1]byte
	_, err := io.ReadFull(r, buf[:])
	return buf[0], err
}

func readUint16(r io.Reader) (uint16, error) {
	var buf [2]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

func readUint32(r io.Reader) (uint32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(buf[:]), nil
}

func readUint64(r io.Reader) (uint64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(buf[:]), nil
}
