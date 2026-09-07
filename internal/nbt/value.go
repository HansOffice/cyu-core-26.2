package nbt

import "fmt"

// Value is a typed NBT value. Concrete values preserve the exact NBT numeric
// type; values are never inferred from Go's default numeric representation.
type Value interface {
	Type() Type
	nbtValue()
}

type Byte int8

type Short int16

type Int int32

type Long int64

type Float float32

type Double float64

type ByteArray []byte

type String string

type IntArray []int32

type LongArray []int64

type List struct {
	ElementType Type
	Values      []Value
}

type Compound map[string]Value

func (Byte) Type() Type      { return ByteType }
func (Short) Type() Type     { return ShortType }
func (Int) Type() Type       { return IntType }
func (Long) Type() Type      { return LongType }
func (Float) Type() Type     { return FloatType }
func (Double) Type() Type    { return DoubleType }
func (ByteArray) Type() Type { return ByteArrayType }
func (String) Type() Type    { return StringType }
func (List) Type() Type      { return ListType }
func (Compound) Type() Type  { return CompoundType }
func (IntArray) Type() Type  { return IntArrayType }
func (LongArray) Type() Type { return LongArrayType }

func (Byte) nbtValue()      {}
func (Short) nbtValue()     {}
func (Int) nbtValue()       {}
func (Long) nbtValue()      {}
func (Float) nbtValue()     {}
func (Double) nbtValue()    {}
func (ByteArray) nbtValue() {}
func (String) nbtValue()    {}
func (List) nbtValue()      {}
func (Compound) nbtValue()  {}
func (IntArray) nbtValue()  {}
func (LongArray) nbtValue() {}

// NewList constructs a homogeneous NBT list. An empty list uses TAG_End as
// its element type, which is the canonical representation CyuCore emits.
func NewList(values ...Value) (List, error) {
	if len(values) == 0 {
		return List{ElementType: End}, nil
	}
	if values[0] == nil {
		return List{}, fmt.Errorf("nbt: list element 0 is nil")
	}
	elementType := values[0].Type()
	if !elementType.validValueType() {
		return List{}, fmt.Errorf("nbt: invalid list element type %d", elementType)
	}
	cloned := make([]Value, len(values))
	for i, value := range values {
		if value == nil {
			return List{}, fmt.Errorf("nbt: list element %d is nil", i)
		}
		if value.Type() != elementType {
			return List{}, fmt.Errorf("nbt: heterogeneous list at element %d: want type %d, got %d", i, elementType, value.Type())
		}
		cloned[i] = Clone(value)
	}
	return List{ElementType: elementType, Values: cloned}, nil
}

// Clone returns a deep copy suitable for crossing immutable data boundaries.
func Clone(value Value) Value {
	switch value := value.(type) {
	case nil:
		return nil
	case Byte, Short, Int, Long, Float, Double, String:
		return value
	case ByteArray:
		return append(ByteArray(nil), value...)
	case IntArray:
		return append(IntArray(nil), value...)
	case LongArray:
		return append(LongArray(nil), value...)
	case List:
		cloned := List{ElementType: value.ElementType, Values: make([]Value, len(value.Values))}
		for i, entry := range value.Values {
			cloned.Values[i] = Clone(entry)
		}
		return cloned
	case Compound:
		cloned := make(Compound, len(value))
		for key, entry := range value {
			cloned[key] = Clone(entry)
		}
		return cloned
	default:
		panic(fmt.Sprintf("nbt: unsupported Value implementation %T", value))
	}
}
