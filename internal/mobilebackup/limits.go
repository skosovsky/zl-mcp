package mobilebackup

import "time"

// Local limits preserve existing codec, executable and synthetic contracts.
const (
	maximumArchivePageRecords        = 50
	archiveLibraryScanEntries        = 32
	archiveEncryptionKeyBytes        = 32
	archiveKeyReadLimit              = archiveEncryptionKeyBytes + 1
	archiveTimestampUpperMS          = 253402300800000
	archiveDownloadTimeout           = 120 * time.Second
	fileTableScalarBytes             = 4
	maximumArchiveControls           = 5000
	maximumRejectedIDDiagnosticBytes = 64
	maximumNativeIdentityDigits      = 20
	retainedMetadataSizeBytes        = 4
	privateArchiveFileMode           = 0600
	retainedInventoryBatch           = 32
	snapshotFramingSlackBytes        = 64
	snapshotMetadataSizeBytes        = 4
	snapshotDirectoryExtras          = 4
	snapshotKeyDirectoryScanLimit    = 64
	nativeInformationalRowType       = 20
	sqliteReservedHeaderBytes        = 20
	sqliteJournalHeaderProbeBytes    = 20
)
