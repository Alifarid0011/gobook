# Native Go memory sessions: design for evaluation

Ali Farid requested persistent data, Go concurrency, a cell dependency graph and Linux/Windows applications, and selected native compilation over an interpreter.

Proposal: a resident compiled worker, one per host, executes the notebook in a single main function. Generated backward goto labels plus blocking command gates between complete-statement cells preserve the Go lexical environment without hoisting types or interpreting expressions. At gate i, only cells 1..min(i+1,N) can be selected. Re-running an earlier cell invalidates later cells, which must run again in order. Cells share native values, handles and goroutines until stop, timeout, source/environment change requiring restart, or process exit. This supports normal Go modules, generics and CGO subject to the host toolchain. Source changes cannot migrate arbitrary live Go values; require an explicit restart and explain data loss.

Workers use a separate authenticated loopback control connection for bounded integer commands and random, streaming stdout/stderr completion markers. Capture enforces a per-command 1 MiB combined budget while still recognizing completion after truncation. Artifact checkpoints reuse bounded hashes. Exact module pins, provenance, process-group cleanup and host capability/Origin checks stay in force. A timeout kills the worker; a panic or os.Exit ends the session, without promising recovery of arbitrary memory.

Do not automatically parallelize arbitrary cells. A conservative static graph can explain reads/writes and warn about unknown call/alias effects; actual intra-cell concurrency uses normal goroutines and synchronization. A reproducible worker-pool notebook should verify ordered results.

A data/session explorer should show kernel status, last completed cell, source fingerprint, Go heap allocation and goroutine count, plus explicit Restart/Stop. Durable persistence remains explicit exported data/models and experiment bundles; saving arbitrary pointers, device handles or goroutine state is not promised.

Desktop deliverables need explicit packaging/runtime decisions. Initial portable native host binaries and browser app launchers are distinct from an embedded native webview and installer; do not label the former as complete native IDE installers.

Acceptance: no repeated prior side effect when running the next/repeated cell, preserved slice/map identity in RAM, earlier-cell invalidation, concurrent Go worker-pool example, bounded output with completion after truncation, timeout and crash cleanup, exact pins, and Windows build/runtime verification through CI.
