package nbt

import (
	"fmt"
	"io"
	"unicode/utf16"
	"unicode/utf8"
)

const maxModifiedUTF8Bytes = 65535

// writeModifiedUTF8 implements java.io.DataOutput.writeUTF semantics used by
// Java Edition NBT strings: a big-endian unsigned-short byte length followed
// by Modified UTF-8 encoded UTF-16 code units.
func writeModifiedUTF8(w io.Writer, value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("nbt: string is not valid UTF-8")
	}

	units := utf16.Encode([]rune(value))
	encoded := make([]byte, 0, len(value))
	for _, unit := range units {
		switch {
		case unit >= 0x0001 && unit <= 0x007f:
			encoded = append(encoded, byte(unit))
		case unit <= 0x07ff:
			encoded = append(encoded,
				byte(0xc0|unit>>6),
				byte(0x80|unit&0x3f),
			)
		default:
			encoded = append(encoded,
				byte(0xe0|unit>>12),
				byte(0x80|(unit>>6)&0x3f),
				byte(0x80|unit&0x3f),
			)
		}
		if len(encoded) > maxModifiedUTF8Bytes {
			return fmt.Errorf("nbt: modified UTF-8 string is %d bytes, maximum is %d", len(encoded), maxModifiedUTF8Bytes)
		}
	}

	var length [2]byte
	length[0] = byte(len(encoded) >> 8)
	length[1] = byte(len(encoded))
	if err := writeAll(w, length[:]); err != nil {
		return err
	}
	return writeAll(w, encoded)
}
