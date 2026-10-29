# Research workspace implementation plan

Goal: make GoBook useful on small screens and ship a reproducible, inspectable tabular experiment workflow.

Architecture: retain the standard-library server, native Go compiler, and fresh-process execution. Separate frontend work, kernel reproducibility, tabular model serialization, and research protocol into disjoint files. No persistent REPL or GPU-training claim.

Global constraints: Go >=1.25; no added application dependencies; keep host/origin/capability guards; notebook code runs with user privileges; loaded files never auto-execute; keep v1 notebooks valid; no invented benchmark outcomes or universal language ranking.

Tasks:
- Frontend (root): readable touch controls, code/results views, statistics templates, accessible figure expansion and result downloads; verify desktop/tablet/mobile.
- Kernel: exact dependency pins; bounded input JSON/CSV files; capture environment and module lock provenance; export JSON/CSV artifacts. Verify invalid pins, traversal, bounds, and round-trip model files.
- Tabular package: transparent one-predictor OLS regression, held-out evaluation, strict CSV, JSON model save/load and explicit close; test malformed/nonfinite input and serialization parity. A small model workflow, not sklearn parity.
- Experiment export (root): download/import a versioned bundle containing notebook, data, settings, run metadata/results and figures; importing never executes and marks historical results.
- Research: English protocol/manuscript skeleton under docs/research, named alifarid provisionally; real references, no fabricated results or claim of established novelty; define identical-backend and end-to-end baselines, statistics, and threats to validity.
- Integrate: run full race/vet tests, browser workflows, independent final review, publish own repo changes and verify CI. Submit no journal paper without final author review.

Review focus: data loss on template/import; stale results from edited code; auth on exported execution; nonfinite/numerically degenerate model inputs; misleading performance attribution.
