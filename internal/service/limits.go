package service

import "time"

// Local limits preserve existing codec, executable and synthetic contracts.
const (
	defaultArchiveCatalogueLimit     = 20
	membershipByteBits               = 8
	membershipBitRoundUp             = 7
	archiveExcerptRunes              = 2048
	stdioProxyTimeout                = 40 * time.Second
	maximumStdioToolNameBytes        = 256
	maximumStdioRequests             = 4096
	stdioRequestLifetime             = 24 * time.Hour
	maximumOwnerRequestBytes         = 16 << 10
	mobileLedgerArgumentCount        = 3
	mobileComparisonArgumentCount    = 4
	defaultOwnerRequestTimeout       = 10 * time.Second
	archiveAdministrationTimeout     = 140 * time.Second
	recallCommandTimeout             = 40 * time.Second
	mobileOfferCommandTimeout        = 205 * time.Second
	mobileCaptureCommandTimeout      = 445 * time.Second
	maximumProbeComparisonRecords    = 500
	probeArchivePageSize             = 50
	maximumProbeArchivePages         = 100
	maximumLedgerJSONDepth           = 4
	privateDiagnosticDirectoryMode   = 0700
	privateDiagnosticFileMode        = 0600
	recallReceiptDirectoryReadLimit  = 21
	recallReceiptReadLimit           = 513
	mobileSnapshotBudgetBytes        = 512 << 20
	accountArchiveCacheBudgetBytes   = 1 << 30
	accountArchiveLibraryBudgetBytes = 1 << 30
	serverHeaderTimeout              = 5 * time.Second
	mcpReadTimeout                   = 15 * time.Second
	mcpWriteTimeout                  = 45 * time.Second
	mcpIdleTimeout                   = 30 * time.Second
	ownerReadTimeout                 = 10 * time.Second
	ownerWriteTimeout                = 450 * time.Second
	collectorHeartbeatInterval       = 10 * time.Second
	maximumCollectorBackoff          = 40 * time.Second
	serviceErrorBuffer               = 8
	serviceRetentionInterval         = 5 * time.Minute
	telemetryQueueInterval           = 500 * time.Millisecond
	bearerTokenRandomBytes           = 32
)
