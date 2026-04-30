package main

import "github.com/vmihailenco/msgpack/v5"

// decodeMsgpack is a tiny wrapper so main.go can decode the manifest
// [config] table without pulling the msgpack import into every source
// file that handles configuration.
func decodeMsgpack(data []byte, v any) error {
	return msgpack.Unmarshal(data, v)
}
