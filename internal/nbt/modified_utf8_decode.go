package nbt

import (
	"fmt"
	"io"
	"unicode/utf16"
)

func readModifiedUTF8(r io.Reader) (string, error) {
	length, err := readUint16(r)
	if err != nil {
		return "", err
	}
	encoded := make([]byte, int(length))
	if _, err := io.ReadFull(r, encoded); err != nil {
		return "", err
	}

	units := make([]uint16, 0, len(encoded))
	for i := 0; i < len(encoded); {
		first := encoded[i]
		switch {
		case first >= 0x01 && first <= 0x7f:
			units = append(units, uint16(first))
			i++
		case first&0xe0 == 0xc0:
			if i+1 >= len(encoded) || encoded[i+1]&0xc0 != 0x80 {
				return "", fmt.Errorf("nbt: malformed modified UTF-8 at byte %d", i)
			}
			unit := uint16(first&0x1f)<<6 | uint16(encoded[i+1]&0x3f)
			units = append(units, unit)
			i += 2
		case first&0xf0 == 0xe0:
			if i+2 >= len(encoded) || encoded[i+1]&0xc0 != 0x80 || encoded[i+2]&0xc0 != 0x80 {
				return "", fmt.Errorf("nbt: malformed modified UTF-8 at byte %d", i)
			}
			unit := uint16(first&0x0f)<<12 | uint16(encoded[i+1]&0x3f)<<6 | uint16(encoded[i+2]&0x3f)
			units = append(units, unit)
			i += 3
		default:
			return "", fmt.Errorf("nbt: malformed modified UTF-8 leading byte 0x%02x at byte %d", first, i)
		}
	}

	for i := 0; i < len(units); i++ {
		unit := units[i]
		if unit >= 0xd800 && unit <= 0xdbff {
			if i+1 >= len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
				return "", fmt.Errorf("nbt: unpaired high surrogate at UTF-16 unit %d", i)
			}
			i++
			continue
		}
		if unit >= 0xdc00 && unit <= 0xdfff {
			return "", fmt.Errorf("nbt: unpaired low surrogate at UTF-16 unit %d", i)
		}
	}

	return string(utf16.Decode(units)), nil
}
