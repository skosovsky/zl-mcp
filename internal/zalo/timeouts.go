package zalo

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	receiveMobileBackupOfferTimeout = 180 * time.Second
)
