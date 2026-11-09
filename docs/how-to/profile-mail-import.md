<!-- prose:plain -->
# Profile a mail import

Run this test from `backend/` to read what an import costs. It uses test mail, the real HTTP reader
and parser, and a sink that keeps no result.

```sh
go test ./internal/modules/capture/gmail -run '^$' \
  -bench BenchmarkRawImportsShareMemoryBudget -benchtime=1x \
  -memprofile=/tmp/mail-import.pprof -outputdir=/tmp
go tool pprof -top /tmp/mail-import.pprof
```

The test prints the bytes it used and a sample of `HeapAlloc`. A sample can miss work between
reads. Use it with the live worker profiles in [worker profiles](../reference/worker-diagnostics.md).
The test does not write to the database or measure total import time.
