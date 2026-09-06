package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
)

var (
	cachedRegistryPackets  [][]byte
	cachedUpdateTagsPacket []byte
)

const rawRegistryDataB64 = 	"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"

const rawUpdateTagsB64 = 	"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" +
	"s1wAAA=="

func init() {
	cachedRegistryPackets = unpackRegistryPackets(rawRegistryDataB64)
	cachedUpdateTagsPacket = unpackSinglePacket(rawUpdateTagsB64)
}

func decompressB64(b64Str string) []byte {
	compressed, err := base64.StdEncoding.DecodeString(b64Str)
	if err != nil {
		return nil
	}
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil
	}
	return data
}

func unpackRegistryPackets(b64Str string) [][]byte {
	raw := decompressB64(b64Str)
	if raw == nil {
		return nil
	}
	buf := bytes.NewReader(raw)
	count, err := readVarInt(buf)
	if err != nil {
		return nil
	}
	packets := make([][]byte, count)
	for i := 0; i < count; i++ {
		length, err := readVarInt(buf)
		if err != nil {
			return nil
		}
		p := make([]byte, length)
		if _, err := io.ReadFull(buf, p); err != nil {
			return nil
		}
		packets[i] = p
	}
	return packets
}

func unpackSinglePacket(b64Str string) []byte {
	return decompressB64(b64Str)
}
