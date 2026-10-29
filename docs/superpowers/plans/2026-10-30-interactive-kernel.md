# Interactive kernel implementation

1. Pin interpreter and test editable persistent evaluation with standard-library imports, function values and error recovery.
2. Add child worker mode and transport integration without changing the native default API.
3. Add explicit engine selector and interactive source/output handling with controller regressions.
4. Run native and interactive integration, race, browser and portable checks. Document capabilities accurately and publish only after verification.

5. Add completion using official pinned gopls with labeled local fallback. Exercise stale replies, Unicode edit ranges, cancellation and source-based variable types. Bundle the language-server sidecar with license notices in portable packages.

Evidence: evaluator and real-worker editing tests, long control frames, source/snapshot pairing, 13 kernel controller checks and 7 completion UI regressions; full Linux native+interactive+gopls race suite passed. Independent review findings were fixed and regression-tested. Browser verification showed edited-cell output retaining a captured counter and typed fmt suggestions. Cross-platform release checks are required before publishing installers.
