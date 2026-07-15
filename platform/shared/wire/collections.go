package wire

import (
	"fmt"
	"math"
)

// BrowserObservation is client-authored endpoint inventory carried by XBP.
// Its values are observations and never identity or authorization evidence.
type BrowserObservation struct {
	Name             string
	BinaryPath       string
	ProfileDirectory string
}

// ApplicationUsage is a client-authored count for one registered application category.
type ApplicationUsage struct {
	Category uint16
	Count    uint32
}

func appendBrowserObservations(builder *Builder, values []BrowserObservation) {
	const (
		maximumBrowsers = 32
		maximumName     = 80
		maximumPath     = 4096
		aggregateLimit  = 64 << 10
	)

	if builder == nil || builder.err != nil {
		return
	}

	if len(values) > maximumBrowsers {
		builder.err = fmt.Errorf("%w: browser count %d exceeds %d", ErrLimit, len(values), maximumBrowsers)
		return
	}

	start := len(builder.data)
	builder.Uint(uint64(len(values)))

	for _, value := range values {
		builder.String(value.Name, maximumName)
		builder.String(value.BinaryPath, maximumPath)
		builder.String(value.ProfileDirectory, maximumPath)
	}

	if builder.err == nil && len(builder.data)-start > aggregateLimit {
		builder.err = fmt.Errorf("%w: browser observations exceed aggregate limit", ErrLimit)
	}
}

func parseBrowserObservations(parser *Parser) []BrowserObservation {
	const (
		maximumBrowsers = 32
		maximumName     = 80
		maximumPath     = 4096
		aggregateLimit  = 64 << 10
	)

	start := parser.offset
	count := parser.Uint(maximumBrowsers)

	if parser.err != nil {
		return nil
	}

	capacity, err := intFromUint64(count, "browser count")
	if err != nil {
		parser.fail(err)
		return nil
	}

	values := make([]BrowserObservation, 0, capacity)
	for range count {
		values = append(values, BrowserObservation{
			Name:             parser.String(maximumName),
			BinaryPath:       parser.String(maximumPath),
			ProfileDirectory: parser.String(maximumPath),
		})
	}

	if parser.err == nil && parser.offset-start > aggregateLimit {
		parser.err = fmt.Errorf("%w: browser observations exceed aggregate limit", ErrLimit)
		return nil
	}

	return values
}

func appendApplicationUsage(builder *Builder, values []ApplicationUsage) {
	const (
		maximumCategories = 64
		maximumCategory   = 63
	)

	if builder == nil || builder.err != nil {
		return
	}

	if len(values) > maximumCategories {
		builder.err = fmt.Errorf("%w: application category count exceeds %d", ErrLimit, maximumCategories)
		return
	}

	builder.Uint(uint64(len(values)))

	for _, value := range values {
		if value.Category > maximumCategory {
			builder.err = fmt.Errorf("%w: application category %d", ErrEncoding, value.Category)
			return
		}

		builder.Uint(uint64(value.Category))
		builder.Uint(uint64(value.Count))
	}
}

func parseApplicationUsage(parser *Parser) []ApplicationUsage {
	const (
		maximumCategories = 64
		maximumCategory   = 63
	)

	count := parser.Uint(maximumCategories)

	if parser.err != nil {
		return nil
	}

	capacity, err := intFromUint64(count, "application usage count")
	if err != nil {
		parser.fail(err)
		return nil
	}

	values := make([]ApplicationUsage, 0, capacity)

	for range count {
		category, categoryErr := uint16FromUint64(parser.Uint(maximumCategory), "application category")
		usageCount, countErr := uint32FromUint64(parser.Uint(math.MaxUint32), "application usage count")

		if categoryErr != nil || countErr != nil {
			parser.fail(fmt.Errorf("%w: invalid application usage", ErrLimit))
			return nil
		}

		values = append(values, ApplicationUsage{
			Category: category,
			Count:    usageCount,
		})
	}

	return values
}
