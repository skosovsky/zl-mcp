package domain

// JournalReadyV1 is an optional control wakeup for an existing business journal.
// It is never a message/activity profile and must not enter business fanout.
const JournalReadyV1 = "zalo.subscription.journal.ready.v1"
