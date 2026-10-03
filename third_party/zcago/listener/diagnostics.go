package listener

import "sync/atomic"

// Diagnostics contains counts and protocol header numbers only, never payloads,
// identifiers, URLs, keys or credentials. Counts are per Listener instance.
type Diagnostics struct {
	Frames, NonBinary, Unhandled, CipherKeys, GroupFrames, ReplayFrames          uint64
	DecodedGroups, EmittedGroups, Errors                                         uint64
	LastVersion, LastCommand, LastSubcommand                                     uint64
	LastUnhandledCommand, ReplayUsers                                            uint64
	ReplayCode                                                                   int64
	ReplayShape                                                                  uint64
	DirectFrames, DecodedDirect, EmittedDirect, Backpressure, CancelledEmissions uint64
	DirectErrors, GroupErrors, ReplayErrors                                      uint64
}
type diagnosticCounters struct {
	frames, nonBinary, unhandled, cipherKeys, groupFrames, replayFrames          atomic.Uint64
	decodedGroups, emittedGroups, errors                                         atomic.Uint64
	lastVersion, lastCommand, lastSubcommand                                     atomic.Uint64
	lastUnhandledCommand, replayUsers                                            atomic.Uint64
	replayCode                                                                   atomic.Int64
	replayShape                                                                  atomic.Uint64
	directFrames, decodedDirect, emittedDirect, backpressure, cancelledEmissions atomic.Uint64
	directErrors, groupErrors, replayErrors                                      atomic.Uint64
}

func (ln *listener) Diagnostics() Diagnostics {
	d := &ln.diagnostics
	return Diagnostics{
		Frames: d.frames.Load(), NonBinary: d.nonBinary.Load(), Unhandled: d.unhandled.Load(), CipherKeys: d.cipherKeys.Load(),
		GroupFrames: d.groupFrames.Load(), ReplayFrames: d.replayFrames.Load(), DecodedGroups: d.decodedGroups.Load(), EmittedGroups: d.emittedGroups.Load(), Errors: d.errors.Load(),
		LastVersion: d.lastVersion.Load(), LastCommand: d.lastCommand.Load(), LastSubcommand: d.lastSubcommand.Load(), LastUnhandledCommand: d.lastUnhandledCommand.Load(),
		ReplayUsers: d.replayUsers.Load(), ReplayCode: d.replayCode.Load(), ReplayShape: d.replayShape.Load(),
		DirectFrames: d.directFrames.Load(), DecodedDirect: d.decodedDirect.Load(), EmittedDirect: d.emittedDirect.Load(), Backpressure: d.backpressure.Load(), CancelledEmissions: d.cancelledEmissions.Load(),
		DirectErrors: d.directErrors.Load(), GroupErrors: d.groupErrors.Load(), ReplayErrors: d.replayErrors.Load(),
	}
}
