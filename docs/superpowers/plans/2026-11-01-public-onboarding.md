# Public GoBook Onboarding Implementation Plan

**Goal:** Publish an honest, practical entry point at https://alifarid0011.github.io/gobook/.
**Architecture:** Static HTML/CSS, a small platform selector, curated notebook downloads and native GitHub Pages deployment.
**Spec:** ../specs/2026-11-01-public-onboarding.md

## Constraints

No runtime, installation, active kernel, core API, or unrelated repository changes. No new dependencies. Preserve v0.8.1 download names and Go 1.25+ prerequisite. The site does not execute Go.

## Tasks

- [x] Write Node built-in tests for valid platform commands, unsafe/unsupported selections, and the runnable first-steps notebook; observe failure.
- [x] Add `site/app.js`, `site/index.html`, `site/styles.css`, and `site/examples/first-steps.gobook.json` with accessible, responsive local setup and useful defaults without JS.
- [x] Add `scripts/build-site.sh` and `.github/workflows/pages.yml`; link the public site from README.
- [x] Run Node tests, build the curated artifact, inspect in browser at desktop/mobile widths, and review the diff.
- [ ] Commit with requested author identity, publish on GitHub Pages, and verify its public behavior.

## Review focus

Wrong OS/CPU download; missing Go; confusing a static site with hosted execution; notebooks requiring Native mode; relative assets breaking under /gobook/.
