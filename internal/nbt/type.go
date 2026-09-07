package nbt

// Type is the one-byte tag identifier used by the Java Edition NBT format.
type Type byte

const (
	End Type = iota
	ByteType
	ShortType
	IntType
	LongType
	FloatType
	DoubleType
	ByteArrayType
	StringType
	ListType
	CompoundType
	IntArrayType
	LongArrayType
)

func (t Type) validValueType() bool {
	return t >= ByteType && t <= LongArrayType
}
