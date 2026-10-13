package collector

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	joinManagerCallTimeout         = 5 * time.Second
	joinManagerexecuteCheckTimeout = 15 * time.Second
	joinManagerexecuteSaveTimeout  = 5 * time.Second
	joinManagerexecuteTimeout      = 45 * time.Second
	probePreloadTimeout            = 30 * time.Second
	refreshCollectorGroupsQTimeout = 30 * time.Second
	refreshContactProfilesTimeout  = 30 * time.Second
	refreshContactsTimeout         = 30 * time.Second
	refreshPreloadCatalogTimeout   = 30 * time.Second
	runSessionShutdownTimeout      = 5 * time.Second
)
