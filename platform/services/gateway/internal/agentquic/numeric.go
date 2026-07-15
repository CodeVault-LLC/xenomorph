package agentquic

import (
	"fmt"
	"time"
)

func streamIDValue(value int64) (uint64, error) {
	if value < 0 {
		return 0, fmt.Errorf("invalid negative QUIC stream ID")
	}

	return uint64(value), nil
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

func durationMilliseconds(value time.Duration) (uint64, error) {
	milliseconds := value.Milliseconds()
	if milliseconds < 0 {
		return 0, fmt.Errorf("invalid negative duration")
	}

	return uint64(milliseconds), nil
}
