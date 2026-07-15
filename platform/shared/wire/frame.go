package wire

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	maxFrameLengthVarintBytes  = 5
	maxMessageTypeVarintBytes  = 3
	operationIDSize            = 16
	minimumFrameFieldsSize     = 4
	initialFrameHeaderCapacity = 32
	minimumFixedHeaderBytes    = 3
)

// MessageType is an append-only XBP message registry identifier.
type MessageType uint16

// FrameHeader contains routing and correlation fields common to XBP messages.
type FrameHeader struct {
	Type                MessageType
	SchemaRevision      uint8
	Flags               FrameFlag
	Sequence            uint64
	CorrelationSequence uint64
	OperationID         [operationIDSize]byte
}

// Frame is one fully decoded XBP application frame.
type Frame struct {
	Header FrameHeader
	Body   []byte
}

// FrameCodec encodes and decodes frames under one stream-kind byte limit.
type FrameCodec struct {
	maximum uint32
}

// NewFrameCodec creates a codec that rejects frames larger than maximum bytes.
func NewFrameCodec(maximum uint32) (FrameCodec, error) {
	if maximum < minimumFrameFieldsSize {
		return FrameCodec{}, fmt.Errorf("create XBP frame codec: %w: maximum %d", ErrLimit, maximum)
	}
	return FrameCodec{maximum: maximum}, nil
}

// WriteFrame writes one canonical frame without retaining caller body storage.
func (codec FrameCodec) WriteFrame(writer io.Writer, frame Frame) error {
	if writer == nil {
		return fmt.Errorf("write XBP frame: %w: nil writer", ErrEncoding)
	}
	header, err := encodeFrameHeader(frame.Header)
	if err != nil {
		return fmt.Errorf("write XBP frame: %w", err)
	}
	length := len(header) + len(frame.Body)
	lengthValue, err := uint64FromNonnegativeInt(length, "frame length")
	if err != nil {
		return fmt.Errorf("write XBP frame: %w", err)
	}
	if lengthValue > uint64(codec.maximum) {
		return fmt.Errorf("write XBP frame: %w: frame length %d exceeds %d", ErrLimit, length, codec.maximum)
	}
	var prefix [maxFrameLengthVarintBytes]byte
	prefixWidth := binary.PutUvarint(prefix[:], lengthValue)
	if _, err := writer.Write(prefix[:prefixWidth]); err != nil {
		return fmt.Errorf("write XBP frame length: %w", err)
	}
	if _, err := writer.Write(header); err != nil {
		return fmt.Errorf("write XBP frame header: %w", err)
	}
	if _, err := writer.Write(frame.Body); err != nil {
		return fmt.Errorf("write XBP frame body: %w", err)
	}
	return nil
}

// ReadFrame reads exactly one canonical bounded frame.
func (codec FrameCodec) ReadFrame(reader io.Reader) (Frame, error) {
	if reader == nil {
		return Frame{}, fmt.Errorf("read XBP frame: %w: nil reader", ErrEncoding)
	}
	buffered, ok := reader.(io.ByteReader)
	if !ok {
		buffered = singleByteReader{reader: reader}
	}
	length, err := readCanonicalUvarint(buffered, maxFrameLengthVarintBytes, uint64(codec.maximum))
	if err != nil {
		return Frame{}, fmt.Errorf("read XBP frame length: %w", err)
	}
	if length < minimumFrameFieldsSize {
		return Frame{}, fmt.Errorf("read XBP frame: %w: frame length %d", ErrEncoding, length)
	}
	encodedLength, err := intFromUint64(length, "frame length")
	if err != nil {
		return Frame{}, fmt.Errorf("read XBP frame: %w", err)
	}
	encoded := make([]byte, encodedLength)
	if _, err := io.ReadFull(readerFromByteReader(buffered), encoded); err != nil {
		return Frame{}, fmt.Errorf("read XBP frame payload: %w: %v", ErrEncoding, err)
	}
	header, bodyOffset, err := decodeFrameHeader(encoded)
	if err != nil {
		return Frame{}, fmt.Errorf("read XBP frame header: %w", err)
	}
	return Frame{Header: header, Body: append([]byte(nil), encoded[bodyOffset:]...)}, nil
}

// singleByteReader deliberately performs exact one-byte reads. A temporary
// bufio.Reader is unsafe here because it can read ahead into a later frame and
// then be discarded when ReadFrame returns.
type singleByteReader struct {
	reader io.Reader
}

func (reader singleByteReader) ReadByte() (byte, error) {
	var value [1]byte
	_, err := io.ReadFull(reader.reader, value[:])
	return value[0], err
}

func encodeFrameHeader(header FrameHeader) ([]byte, error) {
	if err := validateFrameHeader(header); err != nil {
		return nil, err
	}
	encoded := make([]byte, 0, initialFrameHeaderCapacity)
	encoded = binary.AppendUvarint(encoded, uint64(header.Type))
	encoded = append(encoded, header.SchemaRevision, byte(header.Flags))
	encoded = binary.AppendUvarint(encoded, header.Sequence)
	if header.Flags&FlagHasCorrelation != 0 {
		encoded = binary.AppendUvarint(encoded, header.CorrelationSequence)
	}
	if header.Flags&FlagHasOperationID != 0 {
		encoded = append(encoded, header.OperationID[:]...)
	}
	return encoded, nil
}

func decodeFrameHeader(encoded []byte) (FrameHeader, int, error) {
	var header FrameHeader
	typeValue, width, err := decodeCanonicalUvarint(encoded, maxMessageTypeVarintBytes)
	if err != nil || typeValue > uint64(^uint16(0)) {
		return FrameHeader{}, 0, fmt.Errorf("%w: message type", ErrEncoding)
	}
	header.Type = MessageType(typeValue)
	offset := width
	if len(encoded)-offset < minimumFixedHeaderBytes {
		return FrameHeader{}, 0, fmt.Errorf("%w: truncated fixed header", ErrEncoding)
	}
	header.SchemaRevision = encoded[offset]
	header.Flags = FrameFlag(encoded[offset+1])
	offset += 2
	header.Sequence, width, err = decodeCanonicalUvarint(encoded[offset:], maxUint64VarintBytes)
	if err != nil {
		return FrameHeader{}, 0, err
	}
	offset += width
	if header.Flags&FlagHasCorrelation != 0 {
		header.CorrelationSequence, width, err = decodeCanonicalUvarint(encoded[offset:], maxUint64VarintBytes)
		if err != nil {
			return FrameHeader{}, 0, err
		}
		offset += width
	}
	if header.Flags&FlagHasOperationID != 0 {
		if len(encoded)-offset < operationIDSize {
			return FrameHeader{}, 0, fmt.Errorf("%w: truncated operation ID", ErrEncoding)
		}
		copy(header.OperationID[:], encoded[offset:offset+operationIDSize])
		offset += operationIDSize
	}
	if err := validateFrameHeader(header); err != nil {
		return FrameHeader{}, 0, err
	}
	return header, offset, nil
}

func validateFrameHeader(header FrameHeader) error {
	if err := validateFrameIdentity(header); err != nil {
		return err
	}
	if err := validateFrameCorrelation(header); err != nil {
		return err
	}
	return validateFrameOperation(header)
}

func validateFrameIdentity(header FrameHeader) error {
	if header.Type == 0 || header.SchemaRevision == 0 || header.Sequence == 0 {
		return fmt.Errorf("%w: zero type, revision, or sequence", ErrEncoding)
	}
	if header.Flags&^knownFrameFlags != 0 {
		return fmt.Errorf("%w: reserved frame flag", ErrEncoding)
	}
	return nil
}

func validateFrameCorrelation(header FrameHeader) error {
	if header.Flags&FlagIsResponse != 0 && header.Flags&FlagHasCorrelation == 0 {
		return fmt.Errorf("%w: response lacks correlation", ErrEncoding)
	}
	if header.Flags&FlagHasCorrelation != 0 && header.CorrelationSequence == 0 {
		return fmt.Errorf("%w: zero correlation sequence", ErrEncoding)
	}
	if header.Flags&FlagHasCorrelation == 0 && header.CorrelationSequence != 0 {
		return fmt.Errorf("%w: undeclared correlation sequence", ErrEncoding)
	}
	return nil
}

func validateFrameOperation(header FrameHeader) error {
	if header.Flags&FlagHasOperationID != 0 && isZero16(header.OperationID) {
		return fmt.Errorf("%w: zero operation ID", ErrEncoding)
	}
	if header.Flags&FlagHasOperationID == 0 && !isZero16(header.OperationID) {
		return fmt.Errorf("%w: undeclared operation ID", ErrEncoding)
	}
	return nil
}

func readCanonicalUvarint(reader io.ByteReader, maximumBytes int, maximum uint64) (uint64, error) {
	encoded := make([]byte, 0, maximumBytes)
	for index := 0; index < maximumBytes; index++ {
		value, err := reader.ReadByte()
		if err != nil {
			return 0, fmt.Errorf("%w: truncated varint: %v", ErrEncoding, err)
		}
		encoded = append(encoded, value)
		if value&0x80 != 0 {
			continue
		}
		decoded, width, err := decodeCanonicalUvarint(encoded, maximumBytes)
		if err != nil {
			return 0, err
		}
		if width != len(encoded) || decoded > maximum {
			return 0, fmt.Errorf("%w: varint value exceeds limit", ErrLimit)
		}
		return decoded, nil
	}
	return 0, fmt.Errorf("%w: varint overflow", ErrEncoding)
}

func readerFromByteReader(reader io.ByteReader) io.Reader {
	if typed, ok := reader.(io.Reader); ok {
		return typed
	}
	return byteReaderAdapter{reader: reader}
}

type byteReaderAdapter struct {
	reader io.ByteReader
}

func (adapter byteReaderAdapter) Read(destination []byte) (int, error) {
	for index := range destination {
		value, err := adapter.reader.ReadByte()
		if err != nil {
			return index, err
		}
		destination[index] = value
	}
	return len(destination), nil
}

func isZero16(value [16]byte) bool {
	return value == [16]byte{}
}
