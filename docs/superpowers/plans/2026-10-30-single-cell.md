# Single-cell memory execution

User wants one selected cell to execute while earlier Go values and function values stay in RAM. Existing native worker already retains identity and closures, but the default cell button uses fresh prefix execution and native UI silently fills a missing prefix. Rendering also rebuilds every inline output.

Preserve the user's prior native compilation choice and accepted explicit restart after editing source. An asynchronous clarification asks whether live source redefinition is now essential; do not pretend the current compiled kernel has arbitrary Jupyter evaluation semantics. This change routes a selected cell through an automatically started native worker, sends exactly one command, and rejects an unavailable cell rather than replaying prior cells. Explicit Run all still executes all cells. Keep the existing native complete-statement/frontier constraints clearly visible. Function values use ordinary Go closures and can retain state and return values to later cells.

- [x] Add failing controller regressions: single-cell run starts resident worker, does not queue a missing prefix, leaves existing state/history intact.
- [x] Route cell Run and Shift Enter through the resident worker by default; clearly label fresh Run all and dependency errors.
- [x] Preserve unchanged inline output DOM while updating selected/changed outputs and removing invalidated ones; regression test node identity and artifact ownership changes.
- [x] Add a runnable data/function/closure example and native identity/no-repeat tests. Document native kernel limits without claiming full Jupyter parity.
- [x] Review and full native race/vet/frontend/portable checks. Delivery uses a separate preview tab and an atomic app update so the user's open notebook is preserved.
