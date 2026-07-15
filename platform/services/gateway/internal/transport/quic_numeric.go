package transport

import (
	"fmt"
	"math"
)

func boundedIntFromUint64(value, maximum uint64, field string) (int, error) {
	if value > maximum || value > uint64(math.MaxInt) {
		return 0, fmt.Errorf("convert %s: value exceeds integer range", field)
	}

	return int(value), nil
}

func boundedInt32FromUint64(value, maximum uint64, field string) (int32, error) {
	if value > maximum || value > math.MaxInt32 {
		return 0, fmt.Errorf("convert %s: value exceeds int32 range", field)
	}

	return int32(value), nil
}

func uint64FromNonnegativeInt(value int, field string) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("convert %s: negative value", field)
	}

	return uint64(value), nil
}

func uint64FromNonnegativeInt64(value int64, field string) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("convert %s: negative value", field)
	}

	return uint64(value), nil
}

func int64FromBoundedUint64(value uint64, field string) (int64, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("convert %s: value exceeds int64 range", field)
	}

	return int64(value), nil
}
