package nbt

import "io"

// WriteEndSentinelOptionalNetwork writes the nullable anonymous-NBT wire form
// where TAG_End (0) represents no value; otherwise the value is encoded exactly
// as WriteNetwork. This is deliberately named after its sentinel semantics: it
// is not the boolean-prefixed ByteBufCodecs.optional(TAG) form used by protocol
// 776 RegistrySynchronization.PackedRegistryEntry.
func WriteEndSentinelOptionalNetwork(w io.Writer, value Value) error {
	if value == nil {
		_, err := w.Write([]byte{byte(End)})
		return err
	}
	return WriteNetwork(w, value)
}
