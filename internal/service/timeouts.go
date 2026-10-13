package service

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	collectSessionRestoreTimeout                                    = 30 * time.Second
	membershipPortcaptureAccountArchiveWithDownloaderTimeout        = 420 * time.Second
	membershipPortinspectAccountArchiveFromTimeout                  = 120 * time.Second
	membershipPortprobeMobileArchiveComparisonWithDownloaderTimeout = 420 * time.Second
	membershipPortprobeMobileOfferTimeout                           = 185 * time.Second
	membershipPortrecallControlTimeout                              = 30 * time.Second
	runApproveJoinCommandApproveTimeout                             = 5 * time.Second
	runApproveJoinCommandQTimeout                                   = 5 * time.Second
	runLoginCommandLoginTimeout                                     = 3 * time.Minute
	runPreloadProbeCommandProbeTimeout                              = 35 * time.Second
	serviceLooprunSampleTimeout                                     = 500 * time.Millisecond
	serviceLooprunShutdownTimeout                                   = 5 * time.Second
)
