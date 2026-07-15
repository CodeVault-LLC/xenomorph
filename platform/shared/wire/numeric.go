package wire

import (
	"fmt"
	"math"
)

func uint64FromNonnegativeInt(value int, field string) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("%w: %s is negative", ErrLimit, field)
	}

	return uint64(value), nil
}

func intFromUint64(value uint64, field string) (int, error) {
	if value > uint64(math.MaxInt) {
		return 0, fmt.Errorf("%w: %s exceeds integer range", ErrLimit, field)
	}

	return int(value), nil
}

func int64FromUint64(value uint64, field string) (int64, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("%w: %s exceeds int64 range", ErrLimit, field)
	}

	return int64(value), nil
}

func uint16FromUint64(value uint64, field string) (uint16, error) {
	if value > math.MaxUint16 {
		return 0, fmt.Errorf("%w: %s exceeds uint16 range", ErrLimit, field)
	}

	return uint16(value), nil
}

func uint32FromUint64(value uint64, field string) (uint32, error) {
	if value > math.MaxUint32 {
		return 0, fmt.Errorf("%w: %s exceeds uint32 range", ErrLimit, field)
	}

	return uint32(value), nil
}
