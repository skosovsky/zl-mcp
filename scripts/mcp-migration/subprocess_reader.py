"""Transitional bounded process reader; replaced by the Go performance runner in H04."""
import os
import select
import time
import json

class BoundedJSONReader:
    """A partial line cannot block the watchdog; both output streams are bounded."""
    def __init__(self, stdout, stderr, limit=4 << 20):
        self.stdout, self.stderr, self.limit = stdout, stderr, limit
        self.pending = b''
        self.errors = bytearray()
        self.received = 0
        self.streams = [stdout, stderr]
        for stream in self.streams:
            os.set_blocking(stream.fileno(), False)

    def read(self, deadline):
        while True:
            if b'\n' in self.pending:
                line, self.pending = self.pending.split(b'\n', 1)
                return json.loads(line)
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise RuntimeError('STDIO smoke wall-clock budget exceeded')
            ready, _, _ = select.select(self.streams, [], [], remaining)
            if not ready:
                raise RuntimeError('STDIO smoke wall-clock budget exceeded')
            for stream in ready:
                chunk = os.read(stream.fileno(), 65536)
                if not chunk:
                    self.streams.remove(stream)
                    if stream is self.stdout:
                        raise RuntimeError('closed STDIO response')
                    continue
                self.received += len(chunk)
                if self.received > self.limit:
                    raise RuntimeError('STDIO output budget exceeded')
                if stream is self.stdout:
                    self.pending += chunk
                    if len(self.pending) > 1 << 20:
                        raise RuntimeError('oversized STDIO response')
                else:
                    self.errors.extend(chunk)

