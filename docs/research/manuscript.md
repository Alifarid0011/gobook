# Reproducible Go Research Workspaces: A Protocol for Evaluating Process Lifecycles and Tabular Data Pipelines

**Working protocol and manuscript scaffold — no experimental results**  
Author: **Ali Farid (علی فرید)**  
Version date: 2026-10-29

## Protocol abstract

This document proposes an evaluation of GoBook as an inspectable workspace for small computational experiments. The study will ask whether explicit process lifecycles, pinned dependencies, recorded data splits, and exportable experiment artifacts support repeatable tabular workflows, and what execution and memory costs accompany these choices. A one-predictor ordinary least-squares task will provide a narrow correctness reference. Prospective comparisons will separate compilation, process startup, data preparation, fitting, serialization, and prediction. Optional inference comparisons will hold the native backend and model fixed where technically possible. No measurements, performance advantage, established novelty, or universal language ranking are reported. The protocol must be implemented, frozen and independently reproduced before a results manuscript can be prepared.

## 1. Motivation and research questions

An interactive result is difficult to inspect when its data, execution order, environment and fitted model cannot be recovered together. GoBook is being developed around standard Go compilation and fresh-process execution. The research opportunity is to evaluate a documented combination of provenance and lifecycle choices, rather than to infer that a programming language alone causes a performance advantage.

The prospective questions are:

1. **RQ1, reproducibility:** Can an independent operator recover the same split, model predictions and evaluation metrics from an exported experiment in a documented clean environment?
2. **RQ2, lifecycle costs:** How much time and memory are attributable to compilation, startup, data preparation, model operations and artifact serialization under explicitly defined cache conditions?
3. **RQ3, attribution:** When the same numerical backend is available through two language bindings, what differences remain in application orchestration and marshaling, and how do these differ from end-to-end stack comparisons?

RQ1 is primary. RQ2 is secondary. RQ3 is an optional extension requiring additional adapters; its absence must be reported, not replaced by a different-backend claim about language speed. The current contribution is an evaluation protocol and research artifact under development. A systematic novelty review and completed evidence are prerequisites to any stronger contribution claim.

## 2. Related work and positioning

Go notebooks and Go machine learning are established areas. GoNB provides a Jupyter Go kernel and uses the standard compiler, so compiled Go notebook execution is not claimed as new here. [GoNB repository](https://github.com/janpfeifer/gonb)

GoMLX provides Go machine-learning abstractions with multiple computation backends. Hugot provides ONNX transformer pipelines in Go with pluggable backends. These projects demonstrate relevant prior art; this protocol does not claim to introduce machine learning or transformer inference to Go. Their changing capabilities must be pinned to specific revisions in an eventual comparison. [GoMLX repository](https://github.com/gomlx/gomlx), [Hugot repository](https://github.com/knights-analytics/hugot)

The scikit-learn documentation explains that transformations learned using held-out data can leak information and bias evaluation; it recommends splitting before fitting preprocessing and using pipelines appropriately. This study adopts that principle across implementations. [scikit-learn common pitfalls](https://scikit-learn.org/stable/common_pitfalls)

Go supplies benchmark and profiling tools, including allocation reporting and CPU/heap profiles. Benchmark timing boundaries matter because setup can be excluded from a loop. Profiles can perturb measurements and will therefore be collected in separate diagnostic runs. [Go testing](https://pkg.go.dev/testing), [Go diagnostics](https://go.dev/doc/diagnostics)

Artifact availability and evaluation should be distinguished from scientific correctness. ACM SIGIR describes artifact evaluation and result-reproduction recognition under the ACM framework; this project has received no badge or external evaluation. FAIR principles motivate explicit metadata, provenance and usage licenses. These are design references, not certifications. [ACM SIGIR artifact badging](https://sigir.org/general-information/acm-sigir-artifact-badging/), [FAIR principles](https://www.gofair.foundation/fair-principles)

This is an initial source-backed positioning exercise, not a systematic literature review. Before submission, search additional notebook systems, executable papers, workflow/provenance systems and resource-aware inference methods; record search dates, queries, screening criteria and excluded near neighbors.

## 3. System and claim boundaries

The implementation plan retains the standard-library server, native compiler and fresh-process execution. Exact dependency pins, model JSON and experiment export are implemented in the v0.2.0 candidate; this protocol does not yet designate a frozen evaluated research revision. Persistent workers are future work. The narrow tabular artifact supports strict CSV input, one-predictor OLS, held-out evaluation and JSON model round trips. Exported experiment bundles carry source, data, settings and historical execution records. This feature inventory does not establish independent reproducibility; the evaluated revision and verification evidence must be frozen before the planned study.

Process termination can bound a session lifetime, but it does not prove a bound on peak memory. File-size limits do not establish whole-process isolation or protect against arbitrary user-code allocations. Explicit model closure and process exit must be examined separately. User code runs with the user's privileges; a research bundle is not a security sandbox. No GPU training capability or parity with a general machine-learning framework is asserted.

## 4. Prospective experimental design

Freeze a signed-off protocol revision before confirmatory measurement. Record deviations with rationale and date. The following values are planned, not yet preregistered or executed.

### 4.1 Data and correctness

Use deterministic synthetic CSV data first: row counts 1,000, 10,000 and 100,000; one float64 predictor, one float64 target, fixed generator revision, generator seeds 101–110, and an explicit archived split-index file. A size outside a verified application limit is unsupported and must be reported as such. Use an 80/20 train/test split created once outside the timed model code; record index hashes. Fit any learned transformation only on training rows, and persist its parameters. The initial task uses no learned preprocessing. Do not select hyperparameters or tolerances using the held-out targets.

Archive exact CSV bytes instead of relying on cross-language pseudorandom generators to produce identical data. Include exact-affine, noisy-affine, constant-predictor, malformed, missing-value and nonfinite fixtures. Degenerate input should yield a documented rejection or explicitly defined behavior in every adapter. Constant-target R-squared is reported as undefined rather than silently assigned an arbitrary score. Add a separately licensed real dataset only through a documented protocol amendment before its confirmatory analysis.

Use an independent high-precision reference for small, well-conditioned affine fixtures. Planned float64 acceptance is `abs(actual-reference) <= 1e-10 + 1e-8*abs(reference)` per coefficient and prediction. The accepted input domain and conditioning thresholds must be defined in the executable fixtures before freezing. Both prediction and metric checks are required: held-out MAE is the mean absolute residual, MSE is the mean squared residual, and R-squared is one minus residual sum of squares divided by the sum of squared deviations from the held-out target mean. Each defined metric must satisfy the same absolute-plus-relative tolerance against the reference. Ill-conditioned fixtures are diagnostic and must not be silently discarded. Save/reload predictions in the same implementation must satisfy the same bound. Parse rejection, split indices, row counts and SHA-256 hashes require exact agreement. A failure prevents performance comparisons from being interpreted as equivalent work. These thresholds are study decisions, not guarantees supplied by cited libraries.

### 4.2 Comparison matrix

| Track | Implementations | Controlled factors | Interpretation |
| --- | --- | --- | --- |
| A: artifact repeatability | GoBook original run, clean local replay, independent operator replay | Source revision, input bytes, split, dependency locks, commands | Reproduction success and provenance coverage |
| B: tabular end-to-end | GoBook OLS, compiled Go CLI using the same Go OLS routine, Python script using a pinned scikit-learn linear regression | Data/split, float64, intercept, explicit threading, metric definitions | Stack/workflow costs; solvers may differ |
| C: same-backend inference, optional | Go and Python bindings to one pinned native runtime | Exact model bytes, runtime/provider, graph options, tokenizer/input tensors, precision, thread count, batch size | Binding/orchestration differences; backend equivalence must be demonstrated |

Track C is not implemented by this protocol. Select a redistributable model and write a revision-specific adapter plan before running it. If backend equivalence cannot be established, report the comparison as end-to-end and remove language-causality wording. GoMLX/Hugot are prior art and possible future integrations, not measured baselines by inclusion in this document.

### 4.3 Timing and resource boundaries

Measure the following independently with a monotonic clock: dependency acquisition; clean compilation with an isolated empty build cache; cached compilation with a prepopulated cache; process start to ready; parse/preprocess; fit; serialize; load; first prediction; repeated prediction; complete cell submission to result. Never mix dependency download with steady execution or describe a fresh process as an empty OS page cache. Record filesystem/cache preparation commands and whether page cache is uncontrolled.

For warm prediction, use 10 untimed calls followed by 100 timed calls per process; retain per-call timings. Use 30 fresh processes per implementation/size/seed, with order randomized in paired blocks by a recorded scheduling seed. Calls inside a process are correlated and are not 100 independent experimental replicates. Cold start is measured once per fresh process before warmup. Compilation has its own 30-repetition schedule per cache condition. Complete-workflow runs include compilation/startup costs where the actual workflow incurs them.

Collect wall time, CPU time, throughput, process peak RSS, allocations where supported, artifact bytes, error/timeout counts, prediction error and reproduction success. Go heap bytes and allocations describe the Go runtime; they are not directly comparable to whole-process RSS or native accelerator memory. Record units, platform-specific RSS semantics, sampling frequency and unsupported metrics. Optional accelerator runs require explicit synchronization and separate host/device memory reporting. Do not run profilers during primary timing samples.

On one dedicated host first, record CPU model, cores, RAM, OS/kernel, power policy, affinity, background load, Go/Python/compiler versions, CGO flags, native library versions, container digest if used, thread limits, `GOMAXPROCS`, `GOGC`, `GOMEMLIMIT`, and relevant backend settings. Run a second host/architecture as external validation when available; report limited generalization otherwise.

### 4.4 Analysis plan

The primary outcome is the fraction of independent artifact replays passing every prespecified correctness/provenance check; publish numerator, denominator and failure reasons. Initially recruit one independent operator and perform three clean replays per available host. This small sample is descriptive, not a population estimate of usability.

For runtime and RSS, publish raw data, medians, interquartile ranges and 95% confidence intervals. Estimate paired implementation ratios using a hierarchical bootstrap that resamples dataset seeds and process blocks, never individual warm calls as independent runs; use 10,000 resamples and a fixed recorded analysis seed. State the resampling hierarchy in the analysis script. Keep bootstrap intervals descriptive with only ten data seeds. For Go microbenchmarks use a pinned benchstat revision and archive its output; its documentation recommends repeated samples and provides median comparisons. [benchstat documentation](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat)

Do not cherry-pick a single fastest trial, remove statistical outliers automatically, or aggregate incompatible hardware into one language ranking. Invalid samples include clock failures, verified infrastructure interruption, or protocol violation; keep them in raw logs with reasons and report sensitivity with and without exclusions. Report timeouts and out-of-memory failures separately, without imputing successful timings. Label any exploratory endpoint or post hoc hypothesis. Do not claim practical superiority based only on statistical significance.

## 5. Open artifact and provenance plan

Archive a source commit, protocol revision, build commands, toolchain versions, exact dependency locks and native libraries, source/model/data hashes, data generator, split indices, model schema, fitted parameters, raw logs, analysis scripts, figures and a step-by-step clean replay guide. Keep historical outputs explicitly tied to their source/data hashes and distinguish imported records from newly executed results. Redact secrets and personal paths from environment metadata before public release.

Maintain an artifact manifest containing `name`, `kind`, `upstream_url`, `revision`, `sha256`, `license_identifier`, `license_url`, `copyright_notice`, `modifications`, and `redistribution_status`. Review software, dataset, model weights and tokenizers separately; their terms may differ. A missing or incompatible license blocks inclusion in the all-open research bundle. The goal is a fully inspectable redistributable artifact, not an unsupported legal assertion about upstream rights. Select an archival repository and persistent identifier after the bundle and licenses are verified.

## 6. Separate future algorithmic study

One future question is whether a bounded in-flight pipeline can enforce a specified memory envelope while batching variable-sized records. A candidate design would admit work using a byte budget and release permits only after downstream consumers release buffers. This is a hypothesis, not an implemented algorithm, novelty claim or performance result. A real bound would need to account for queue ownership, retained buffers, tokenizer expansion, model workspaces, native allocations and failure paths. Existing batching and resource-aware scheduling literature must be reviewed first. Evaluate any future implementation against fixed batching and size-bucket batching using identical models and input order, with memory/latency/throughput tradeoffs and starvation checks. Keep this study separate from the current OLS workspace evaluation.

## 7. Results and limitations

**Results pending.** No measurement table, speedup, memory saving, reproduction rate or statistical test is reported. Test passes from software development, if available elsewhere, are not substituted for study observations.

This protocol covers a small one-predictor regression task and cannot establish general ML-framework completeness, large-model training performance or superiority across programming languages. Different numerical solvers, shared native libraries, filesystem caches, thread pools, host noise and hardware can confound comparisons. Fresh-process execution may improve state visibility while adding compilation and startup cost; the direction and magnitude remain empirical questions. Deterministic output does not establish scientific validity or freedom from leakage. Independent artifact replay is planned, not completed.

## 8. Author review and submission gate

Confirm author contact details; review the selected venue’s assistance-disclosure policy and prepare any required accurate contribution/assistance disclosure; complete the novelty review; freeze runnable methods; verify licenses; collect and analyze all planned runs; reproduce independently; and obtain author approval of the results and wording. Venue selection and formatting follow that work. No journal submission or arXiv upload has occurred.
