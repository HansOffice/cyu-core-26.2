package nbt

import "io"

// WriteOptionalNetwork writes the optional anonymous-NBT form used by Java
// Edition packets: TAG_End (0) represents no value; otherwise the value is
// encoded exactly as WriteNetwork.
func WriteOptionalNetwork(w io.Writer, value Value) error {
	if value == nil {
		_, err := w.Write([]byte{byte(End)})
		return err
	}
	return WriteNetwork(w, value)
}
