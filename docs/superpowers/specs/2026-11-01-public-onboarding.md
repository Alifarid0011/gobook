# Public GoBook onboarding

The user approved a public GitHub Pages site with simple local setup. Sheetframe keeps real browser execution; GoBook's public site must explain that its compiler and kernel run on the user's computer. Keep the installed application and active kernels untouched.

Use a small static site in `site/`, existing workspace imagery, and curated downloadable notebook files. Reuse the v0.8.1 release assets. Offer Linux/Windows amd64/arm64 downloads, source-based macOS setup, a first-run notebook requiring only fmt, a link to the real sheetframe playground, and visible engine/platform limitations. No hosted execution service or new dependencies.

A platform selector makes install commands and download links match the selected target. Invalid platform/architecture values must fail closed. The initial HTML must remain useful without JavaScript.

Build a curated public artifact in `dist/site`; do not publish the full docs/research tree. A Pages workflow builds/tests the static artifact and deploys from main. Verify platform choices, notebook schema/cell execution, local navigation and mobile layout, then verify the real deployment.
