package utils

import "github.com/klauspost/compress/zstd"

var (
	zstdEncoder, _ = zstd.NewWriter(nil)
	zstdDecoder, _ = zstd.NewReader(nil)
)

func Compress(data []byte) []byte {
	return zstdEncoder.EncodeAll(data, make([]byte, 0, len(data)))
}

func Decompress(data []byte) ([]byte, error) {
	return zstdDecoder.DecodeAll(data, nil)
}