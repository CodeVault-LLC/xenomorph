//go:build windows

package filesystem

import (
	"os"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

func metadataWriteCapability() fileprotocol.CapabilityState {
	return fileprotocol.CapabilityUnavailable
}

func platformMetadataFields(_ os.FileInfo) map[string]fileprotocol.FieldValue {
	unavailable := fileprotocol.FieldValue{State: fileprotocol.CapabilityUnavailable}
	return map[string]fileprotocol.FieldValue{
		"owner": unavailable, "group": unavailable, "acl": unavailable,
		"birth_time": unavailable, "extended_attributes": unavailable,
	}
}

func (root *rootHandle) setMetadata(_ []string, delta fileprotocol.MetadataDelta) []fileprotocol.MetadataFieldResult {
	results := make([]fileprotocol.MetadataFieldResult, 0, 2)
	if delta.ModifiedAt != nil {
		results = append(results, fileprotocol.MetadataFieldResult{Field: "modified_at", State: fileprotocol.MetadataUnavailable, ErrorClass: "not_supported"})
	}
	if delta.POSIXMode != nil {
		results = append(results, fileprotocol.MetadataFieldResult{Field: "posix_mode", State: fileprotocol.MetadataUnavailable, ErrorClass: "not_supported"})
	}
	return results
}
