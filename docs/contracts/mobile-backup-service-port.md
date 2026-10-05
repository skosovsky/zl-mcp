# Mobile offer service port candidate

Status: internal service method; not exposed by MCP or the trusted CLI yet.

The service port executes only an existing prepared attempt through the current
collector JoinManager.API, which production RunInternal binds to sessionGuard.
It never restores another session or starts another listener. Follow caller and
service lifecycle cancellation before acquiring session ownership; a request
waiting for authentication does not dispatch. A missing current source returns a
fixed error without preparing/mutating an attempt. Hold the current port ownership
through execution so session replacement cannot race it. The guard handles
revocation/cancellation and late-success suppression; the prepared-offer runner
handles durable dispatch ownership and finalization. Return only a private offer
to a future trusted archive consumer, never serialize it as a tool result.

There is no public dispatch/import route, implicit startup phone request,
subscription change or corpus import at this stage. Internal staged download and
archive parsing are tested synthetically. Live dispatch authorization, independently
verified downloader hosts, real archive compatibility and selected account-bound
silent persistence remain open.

### First prepared page inside one scope

The internal first-page diagnostic composes offer, session-bound download,
file mapping/selection, immutable SQLite read, sender mapping and expiry while
holding the same current guard/port ownership. Validate scratch/page-size/clock
before any phone request. Capture the current numeric account and reject a change
before returning candidates. Clear the selected archive after page preparation;
clear the page on any late scope/error. This is not persistent import or an archive
cache/public pagination API. A new independent encrypted SQLite fixture supports
synthetic lifecycle acceptance without a real phone, cookie or private message.

### Bounded traversal inside the same scope

`walkPreparedMobilePages` uses the same selected archive and account guard for all
pages via [the streaming traversal contract](mobile-backup-page-walk.md). Reject
invalid page/page-count/clock/scratch/callback arguments before dispatch. No second
offer, download, login or listener is created to continue. The callback borrows a
page until return; after return its buffers are cleared. Recheck guard/account
before returning aggregate counts, and suppress the summary on late failure.
A synthetic service test reads two pages from one download, including an
expired-only final page. Its test-only callback converts a supported text row
and invokes the silent storage port: one historical message is retained, with
no Events/subscription/send ledger writes. Production import/checkpoint wiring
and global control/nontext handling remain open.
