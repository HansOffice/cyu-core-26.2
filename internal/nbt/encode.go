package nbt

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sort"
)

const defaultMaxDepth = 512

// WriteNetwork writes the anonymous NBT form used on the Minecraft network:
// one tag type byte followed immediately by that tag's payload. Unlike named
// root/file NBT, no root-name string is emitted.
func WriteNetwork(w io.Writer, value Value) error {
	if value == nil {
		return fmt.Errorf("nbt: network root is nil")
	}
	if !value.Type().validValueType() {
		return fmt.Errorf("nbt: invalid network root type %d", value.Type())
	}
	if err := writeByte(w, byte(value.Type())); err != nil {
		return err
	}
	return writePayload(w, value, 0)
}

func writePayload(w io.Writer, value Value, depth int) error {
	if depth > defaultMaxDepth {
		return fmt.Errorf("nbt: nesting exceeds %d levels", defaultMaxDepth)
	}
	if value == nil {
		return fmt.Errorf("nbt: nil value")
	}

	switch value := value.(type) {
	case Byte:
		return writeByte(w, byte(value))
	case Short:
		return writeUint16(w, uint16(value))
	case Int:
		return writeUint32(w, uint32(value))
	case Long:
		return writeUint64(w, uint64(value))
	case Float:
		return writeUint32(w, math.Float32bits(float32(value)))
	case Double:
		return writeUint64(w, math.Float64bits(float64(value)))
	case ByteArray:
		if err := writeLength(w, len(value)); err != nil {
			return err
		}
		return writeAll(w, value)
	case String:
		return writeModifiedUTF8(w, string(value))
	case List:
		return writeList(w, value, depth)
	case Compound:
		return writeCompound(w, value, depth)
	case IntArray:
		if err := writeLength(w, len(value)); err != nil {
			return err
		}
		for _, entry := range value {
			if err := writeUint32(w, uint32(entry)); err != nil {
				return err
			}
		}
		return nil
	case LongArray:
		if err := writeLength(w, len(value)); err != nil {
			return err
		}
		for _, entry := range value {
			if err := writeUint64(w, uint64(entry)); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("nbt: unsupported value implementation %T", value)
	}
}

func writeList(w io.Writer, list List, depth int) error {
	if len(list.Values) == 0 {
		if list.ElementType != End && !list.ElementType.validValueType() {
			return fmt.Errorf("nbt: invalid empty-list element type %d", list.ElementType)
		}
		if err := writeByte(w, byte(list.ElementType)); err != nil {
			return err
		}
		return writeLength(w, 0)
	}
	if !list.ElementType.validValueType() {
		return fmt.Errorf("nbt: invalid list element type %d", list.ElementType)
	}
	if err := writeByte(w, byte(list.ElementType)); err != nil {
		return err
	}
	if err := writeLength(w, len(list.Values)); err != nil {
		return err
	}
	for i, entry := range list.Values {
		if entry == nil {
			return fmt.Errorf("nbt: list element %d is nil", i)
		}
		if entry.Type() != list.ElementType {
			return fmt.Errorf("nbt: heterogeneous list at element %d: want type %d, got %d", i, list.ElementType, entry.Type())
		}
		if err := writePayload(w, entry, depth+1); err != nil {
			return fmt.Errorf("nbt: list element %d: %w", i, err)
		}
	}
	return nil
}

func writeCompound(w io.Writer, compound Compound, depth int) error {
	keys := make([]string, 0, len(compound))
	for key := range compound {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		entry := compound[key]
		if entry == nil {
			return fmt.Errorf("nbt: compound key %q has nil value", key)
		}
		if !entry.Type().validValueType() {
			return fmt.Errorf("nbt: compound key %q has invalid type %d", key, entry.Type())
		}
		if err := writeByte(w, byte(entry.Type())); err != nil {
			return err
		}
		if err := writeModifiedUTF8(w, key); err != nil {
			return fmt.Errorf("nbt: compound key %q: %w", key, err)
		}
		if err := writePayload(w, entry, depth+1); err != nil {
			return fmt.Errorf("nbt: compound key %q: %w", key, err)
		}
	}
	return writeByte(w, byte(End))
}

func writeLength(w io.Writer, length int) error {
	if uint64(length) > math.MaxInt32 {
		return fmt.Errorf("nbt: collection length %d exceeds int32", length)
	}
	return writeUint32(w, uint32(int32(length)))
}

func writeByte(w io.Writer, value byte) error {
	var buf [1]byte
	buf[0] = value
	return writeAll(w, buf[:])
}

func writeUint16(w io.Writer, value uint16) error {
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], value)
	return writeAll(w, buf[:])
}

func writeUint32(w io.Writer, value uint32) error {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], value)
	return writeAll(w, buf[:])
}

func writeUint64(w io.Writer, value uint64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], value)
	return writeAll(w, buf[:])
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
