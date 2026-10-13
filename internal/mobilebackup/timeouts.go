package mobilebackup

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	finalizePreparedOfferFinishTimeout = 15 * time.Second
	readOwnDirectRecallSetTimeout      = 30 * time.Second
	readSQLitePageModeTimeout          = 30 * time.Second
	walkPreparedArchiveTimeout         = 120 * time.Second
)
