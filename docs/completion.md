# Go code completion

Type an identifier or a package selector such as `fmt.Pr`, or press **Ctrl+Space**
in a code cell. Suggestions include a name and signature/type where available.
Use Up/Down to select, Enter or Tab to insert, and Escape to dismiss. Shift+Enter
still runs the cell. Completion replaces the selected identifier range, including
the suffix when the cursor is inside a word; it does not append duplicate text.

Portable release archives include the official **gopls v0.20.0** language server
as a sidecar executable. The installed app finds it beside `gobook`. For a source
installation, install it separately:

```sh
go install golang.org/x/tools/gopls@v0.20.0
```

Ensure its directory is on PATH, or pass
`gobook -gopls /absolute/path/to/gopls`. On Windows, use the `.exe` path.

The suggestion footer identifies **gopls · typed suggestions** versus **Local
suggestions**. Without gopls, the editor still offers bundled standard-library
exports with reflection-based signatures, Go keywords, builtins and lexical
notebook identifiers. Lexical identifiers are suggestions, not proof of scope
or type validity. No remote completion service or AI account is used.

Gopls sees generated source containing the imports and cells preceding the
selected cell. It does not inspect values inside either running kernel. Out-of-
order interpreter state, arbitrary mixed declarations/statements and live type
redefinitions can differ from its static source model. External module completion
needs the relevant dependencies in the local Go cache; completion does not
silently download dependencies. Running them in Native mode uses the normal
versioned module workflow.

Requests are debounced and serialized through one lazily started language server.
Old replies cannot insert text after cursor, selection, source, focus or editor
lifetime changes. Escape also cancels queued display. A ten-second deadline
closes blocked language-server transport, and ordinary notebook execution does
not depend on completion succeeding. The process is stopped with the GoBook host.

This release adds completion, not a full language-server IDE. Workspace edits,
automatic import insertion, rename, hover, diagnostics and navigation remain
separate work. GoBook does not claim full Jupyter compatibility.
