package agentquic

import (
	"fmt"
	"math"
	"time"
)

func uint64FromInt(value int, field string) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("convert %s: negative value", field)
	}
	return uint64(value), nil
}

func uint64FromInt64(value int64, field string) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("convert %s: negative value", field)
	}
	return uint64(value), nil
}

func intFromUint64(value, maximum uint64, field string) (int, error) {
	if value > maximum || value > uint64(math.MaxInt) {
		return 0, fmt.Errorf("convert %s: value exceeds integer range", field)
	}
	return int(value), nil
}

func int64FromUint64(value uint64, field string) (int64, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("convert %s: value exceeds signed integer range", field)
	}
	return int64(value), nil
}

func unixMilliseconds(value time.Time, field string) (uint64, error) {
	milliseconds := value.UTC().UnixMilli()
	return uint64FromInt64(milliseconds, field)
}
