package wire

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"unicode/utf8"
)

const (
	maxUint64VarintBytes = 10
	bitsPerByte          = 8
	uint32ByteWidth      = 4
	uint64ByteWidth      = 8
	varintPayloadBits    = 7
	varintWidthRounding  = 6
	maximumPresenceBits  = 64
	signedIntegerSignBit = 63
)

// Builder incrementally constructs a canonical bounded message body.
type Builder struct {
	data []byte
	err  error
}

// NewBuilder creates a message-body builder with a bounded initial capacity.
func NewBuilder(capacity int) *Builder {
	if capacity < 0 {
		capacity = 0
	}

	return &Builder{data: make([]byte, 0, capacity)}
}

// Bytes returns a detached canonical body or the first encoding error.
func (b *Builder) Bytes() ([]byte, error) {
	if b == nil {
		return nil, fmt.Errorf("%w: nil builder", ErrEncoding)
	}

	if b.err != nil {
		return nil, b.err
	}

	return append([]byte(nil), b.data...), nil
}

// Uint appends a canonical unsigned LEB128 value.
func (b *Builder) Uint(value uint64) {
	if b == nil || b.err != nil {
		return
	}

	b.data = binary.AppendUvarint(b.data, value)
}

// BoundedUint appends a canonical unsigned value only when it is inside the
// reviewed field bound.
func (b *Builder) BoundedUint(value, maximum uint64) {
	if b == nil || b.err != nil {
		return
	}

	if value > maximum {
		b.err = fmt.Errorf("%w: unsigned value %d exceeds %d", ErrLimit, value, maximum)
		return
	}

	b.Uint(value)
}

// SInt appends a ZigZag-transformed canonical signed integer.
func (b *Builder) SInt(value int64) {
	encoded := uint64(value<<1) ^ uint64(value>>signedIntegerSignBit) // #nosec G115 -- ZigZag encoding intentionally reinterprets signed bits.
	b.Uint(encoded)
}

// BoundedSInt appends a ZigZag signed value only when its absolute value is
// inside the reviewed field bound.
func (b *Builder) BoundedSInt(value, maximumAbsolute int64) {
	if b == nil || b.err != nil {
		return
	}

	if maximumAbsolute < 0 || value < -maximumAbsolute || value > maximumAbsolute {
		b.err = fmt.Errorf("%w: signed value outside limit", ErrLimit)
		return
	}

	b.SInt(value)
}

// Bool appends the exact one-byte XBP boolean representation.
func (b *Builder) Bool(value bool) {
	if value {
		b.appendByte(1)
		return
	}

	b.appendByte(0)
}

// Fixed32 appends a little-endian fixed-width unsigned value.
func (b *Builder) Fixed32(value uint32) {
	if b == nil || b.err != nil {
		return
	}

	b.data = binary.LittleEndian.AppendUint32(b.data, value)
}

// Fixed64 appends a little-endian fixed-width unsigned value.
func (b *Builder) Fixed64(value uint64) {
	if b == nil || b.err != nil {
		return
	}

	b.data = binary.LittleEndian.AppendUint64(b.data, value)
}

// Fixed16 appends an opaque 16-byte identifier.
func (b *Builder) Fixed16(value [16]byte) {
	if b == nil || b.err != nil {
		return
	}

	b.data = append(b.data, value[:]...)
}

// Opaque32 appends an opaque 32-byte digest or identifier.
func (b *Builder) Opaque32(value [32]byte) {
	if b == nil || b.err != nil {
		return
	}

	b.data = append(b.data, value[:]...)
}

// BytesField appends bounded bytes with their canonical length prefix.
func (b *Builder) BytesField(value []byte, maximum int) {
	if b == nil || b.err != nil {
		return
	}

	if maximum < 0 || len(value) > maximum {
		b.err = fmt.Errorf("%w: byte field length %d exceeds %d", ErrLimit, len(value), maximum)
		return
	}

	length, err := uint64FromNonnegativeInt(len(value), "byte field length")
	if err != nil {
		b.err = err
		return
	}

	b.Uint(length)
	b.data = append(b.data, value...)
}

// String appends a bounded UTF-8 string that contains no NUL byte.
func (b *Builder) String(value string, maximum int) {
	if b == nil || b.err != nil {
		return
	}

	if !utf8.ValidString(value) || containsNUL([]byte(value)) {
		b.err = fmt.Errorf("%w: string is not valid XBP text", ErrEncoding)
		return
	}

	b.BytesField([]byte(value), maximum)
}

// Presence appends the shortest little-bit-order optional presence map.
func (b *Builder) Presence(bits uint64, allowedBits uint8) {
	if b == nil || b.err != nil {
		return
	}

	if allowedBits > maximumPresenceBits || hasBitsAbove(bits, allowedBits) {
		b.err = fmt.Errorf("%w: optional presence bit is not assigned", ErrEncoding)
		return
	}

	length := presenceLength(bits)

	encodedLength, err := uint64FromNonnegativeInt(length, "presence length")
	if err != nil {
		b.err = err
		return
	}

	b.Uint(encodedLength)

	for index := 0; index < length; index++ {
		b.appendByte(byte(bits >> (index * bitsPerByte))) // #nosec G115 -- shifting selects the low byte by construction.
	}
}

// StringList appends a count-bounded and aggregate-byte-bounded string list.
func (b *Builder) StringList(values []string, countMaximum, itemMaximum, aggregateMaximum int) {
	if b == nil || b.err != nil {
		return
	}

	if len(values) > countMaximum {
		b.err = fmt.Errorf("%w: list count %d exceeds %d", ErrLimit, len(values), countMaximum)
		return
	}

	start := len(b.data)
	b.Uint(uint64(len(values)))

	for _, value := range values {
		b.String(value, itemMaximum)
	}

	if b.err == nil && len(b.data)-start > aggregateMaximum {
		b.err = fmt.Errorf("%w: encoded list exceeds %d bytes", ErrLimit, aggregateMaximum)
	}
}

func (b *Builder) appendByte(value byte) {
	if b == nil || b.err != nil {
		return
	}

	b.data = append(b.data, value)
}

// Parser incrementally consumes one bounded message body.
type Parser struct {
	data   []byte
	offset int
	err    error
}

// NewParser creates a parser over one frame body.
func NewParser(data []byte) *Parser {
	return &Parser{data: data}
}

// Done requires the parser to have consumed the body exactly.
func (p *Parser) Done() error {
	if p == nil {
		return fmt.Errorf("%w: nil parser", ErrEncoding)
	}

	if p.err != nil {
		return p.err
	}

	if p.offset != len(p.data) {
		return fmt.Errorf("%w: %d trailing bytes", ErrEncoding, len(p.data)-p.offset)
	}

	return nil
}

// Uint reads one canonical unsigned LEB128 value.
func (p *Parser) Uint(maximum uint64) uint64 {
	if p == nil || p.err != nil {
		return 0
	}

	value, width, err := decodeCanonicalUvarint(p.data[p.offset:], maxUint64VarintBytes)
	if err != nil {
		p.err = err
		return 0
	}

	if value > maximum {
		p.err = fmt.Errorf("%w: unsigned value %d exceeds %d", ErrLimit, value, maximum)
		return 0
	}

	p.offset += width

	return value
}

// SInt reads a canonical ZigZag integer with a symmetric absolute bound.
func (p *Parser) SInt(maximumAbsolute int64) int64 {
	if maximumAbsolute < 0 {
		p.fail(fmt.Errorf("%w: negative signed integer limit", ErrLimit))
		return 0
	}

	maximumEncoded := uint64(maximumAbsolute) << 1
	value := p.Uint(maximumEncoded + 1)

	if p.err != nil {
		return 0
	}

	decoded, err := int64FromUint64(value>>1, "signed integer magnitude")
	if err != nil {
		p.fail(err)
		return 0
	}

	if value&1 != 0 {
		decoded = ^decoded
	}

	if decoded < -maximumAbsolute || decoded > maximumAbsolute {
		p.err = fmt.Errorf("%w: signed value outside limit", ErrLimit)
		return 0
	}

	return decoded
}

// Bool reads one exact XBP boolean.
func (p *Parser) Bool() bool {
	value := p.byte()

	if p.err != nil {
		return false
	}

	switch value {
	case 0:
		return false
	case 1:
		return true
	default:
		p.err = fmt.Errorf("%w: boolean byte %d", ErrEncoding, value)
		return false
	}
}

// Fixed32 reads a little-endian fixed-width unsigned value.
func (p *Parser) Fixed32() uint32 {
	value := p.take(uint32ByteWidth)

	if p.err != nil {
		return 0
	}

	return binary.LittleEndian.Uint32(value)
}

// Fixed64 reads a little-endian fixed-width unsigned value.
func (p *Parser) Fixed64() uint64 {
	value := p.take(uint64ByteWidth)

	if p.err != nil {
		return 0
	}

	return binary.LittleEndian.Uint64(value)
}

// Fixed16 reads an opaque 16-byte identifier.
func (p *Parser) Fixed16() [16]byte {
	var result [16]byte

	value := p.take(len(result))
	if p.err == nil {
		copy(result[:], value)
	}

	return result
}

// Opaque32 reads an opaque 32-byte digest or identifier.
func (p *Parser) Opaque32() [32]byte {
	var result [32]byte

	value := p.take(len(result))
	if p.err == nil {
		copy(result[:], value)
	}

	return result
}

// BytesField reads a bounded length-prefixed byte field into detached storage.
func (p *Parser) BytesField(maximum int) []byte {
	if maximum < 0 {
		p.fail(fmt.Errorf("%w: negative byte field limit", ErrLimit))
		return nil
	}

	maximumLength, err := uint64FromNonnegativeInt(maximum, "byte field limit")
	if err != nil {
		p.fail(err)
		return nil
	}

	length := p.Uint(maximumLength)

	if p.err != nil {
		return nil
	}

	lengthValue, err := intFromUint64(length, "byte field length")
	if err != nil {
		p.fail(err)
		return nil
	}

	value := p.take(lengthValue)

	if p.err != nil {
		return nil
	}

	return append([]byte(nil), value...)
}

// String reads a bounded UTF-8 string that contains no NUL byte.
func (p *Parser) String(maximum int) string {
	value := p.BytesField(maximum)

	if p.err != nil {
		return ""
	}

	if !utf8.Valid(value) || containsNUL(value) {
		p.err = fmt.Errorf("%w: string is not valid XBP text", ErrEncoding)
		return ""
	}

	return string(value)
}

// Presence reads a canonical optional presence map for the assigned bit count.
func (p *Parser) Presence(allowedBits uint8) uint64 {
	maximumLength := int((uint16(allowedBits) + varintPayloadBits) / bitsPerByte)

	maximumLengthValue, err := uint64FromNonnegativeInt(maximumLength, "maximum presence length")
	if err != nil {
		p.fail(err)
		return 0
	}

	length := p.Uint(maximumLengthValue)

	if p.err != nil {
		return 0
	}

	encodedLength, err := intFromUint64(length, "presence length")
	if err != nil {
		p.fail(err)
		return 0
	}

	encoded := p.take(encodedLength)

	if p.err != nil {
		return 0
	}

	if len(encoded) > 0 && encoded[len(encoded)-1] == 0 {
		p.err = fmt.Errorf("%w: overlong optional presence map", ErrEncoding)
		return 0
	}

	var bits uint64
	for index, value := range encoded {
		bits |= uint64(value) << (index * bitsPerByte)
	}

	if hasBitsAbove(bits, allowedBits) {
		p.err = fmt.Errorf("%w: optional presence bit is not assigned", ErrEncoding)
		return 0
	}

	return bits
}

// StringList reads a count-bounded and aggregate-byte-bounded string list.
func (p *Parser) StringList(countMaximum, itemMaximum, aggregateMaximum int) []string {
	if countMaximum < 0 || itemMaximum < 0 || aggregateMaximum < 0 {
		p.fail(fmt.Errorf("%w: negative list bound", ErrLimit))
		return nil
	}

	start := p.offset
	maximumCount, _ := uint64FromNonnegativeInt(countMaximum, "list count")
	count := p.Uint(maximumCount)

	if p.err != nil {
		return nil
	}

	capacity, err := intFromUint64(count, "list count")
	if err != nil {
		p.fail(err)
		return nil
	}

	values := make([]string, 0, capacity)
	for range count {
		values = append(values, p.String(itemMaximum))
	}

	if p.err == nil && p.offset-start > aggregateMaximum {
		p.err = fmt.Errorf("%w: encoded list exceeds %d bytes", ErrLimit, aggregateMaximum)
		return nil
	}

	return values
}

func (p *Parser) byte() byte {
	value := p.take(1)

	if p.err != nil {
		return 0
	}

	return value[0]
}

func (p *Parser) take(length int) []byte {
	if p == nil || p.err != nil {
		return nil
	}

	if length < 0 || length > len(p.data)-p.offset {
		p.err = fmt.Errorf("%w: truncated value", ErrEncoding)
		return nil
	}

	value := p.data[p.offset : p.offset+length]
	p.offset += length

	return value
}

func (p *Parser) fail(err error) {
	if p != nil && p.err == nil {
		p.err = err
	}
}

func decodeCanonicalUvarint(data []byte, maximumBytes int) (uint64, int, error) {
	if maximumBytes <= 0 || maximumBytes > maxUint64VarintBytes {
		return 0, 0, fmt.Errorf("%w: invalid varint byte limit", ErrEncoding)
	}

	for index := 0; index < maximumBytes; index++ {
		if index >= len(data) {
			return 0, 0, fmt.Errorf("%w: truncated varint", ErrEncoding)
		}

		if data[index]&0x80 != 0 {
			continue
		}

		width := index + 1
		value, decodedWidth := binary.Uvarint(data[:width])

		if decodedWidth <= 0 || canonicalUvarintWidth(value) != width {
			return 0, 0, fmt.Errorf("%w: noncanonical varint", ErrEncoding)
		}

		return value, width, nil
	}

	return 0, 0, fmt.Errorf("%w: varint overflow", ErrEncoding)
}

func canonicalUvarintWidth(value uint64) int {
	if value == 0 {
		return 1
	}

	return (bits.Len64(value) + varintWidthRounding) / varintPayloadBits
}

func presenceLength(bits uint64) int {
	length := 0
	for bits != 0 {
		length++
		bits >>= bitsPerByte
	}

	return length
}

func hasBitsAbove(bits uint64, allowedBits uint8) bool {
	if allowedBits >= maximumPresenceBits {
		return false
	}

	return bits>>allowedBits != 0
}

func containsNUL(value []byte) bool {
	for _, current := range value {
		if current == 0 {
			return true
		}
	}

	return false
}
