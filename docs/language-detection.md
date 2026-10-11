# Source-based language detection

`Scanner.Scan` runs specialized detectors, adds generic coverage for unclaimed
source, computes source weights, attaches context, and then finalizes topology.
Language weights are computed **before** displayed summaries are deduplicated;
therefore the weighting phase performs its own repository-relative path deduplication.

## Language versus file kind

`internal/scanner/language.go` owns source-extension classification. It covers
Go, Java, Python, JavaScript, TypeScript, C/C++, C#, Rust, Ruby, PHP, Kotlin,
Swift and the existing Ada, COBOL, JCL, SystemVerilog, VHDL, Fortran, PL/I, RPG,
IBM CL and ABAP extensions. File kind (`source`, `asset`, `binary`) remains a
separate safety/context classification. Textual config, JSON and documentation
are not programming-language votes merely because their file kind is `source`.

Ambiguous extensions such as `.h`, `.vh`, `.v` and `.f` do not guess a language.
C/C++ companion headers remain available as context but implementations cast
the language votes. C implementations in a C++ project are reported as C.

Vue single-file components are normal Node source files. A bounded 64 KiB prefix
is inspected for script tags: `lang="ts"`/`typescript` votes TypeScript,
JavaScript or omitted `lang` votes JavaScript, template-only components use the
explicit Vue category, and unsupported script languages do not guess JS/TS.
The source is never modified. A script tag beyond the inspection limit cannot
establish its JS/TS language from that scan.

## Weights and deterministic results

Accepted source paths are recorded separately from the bounded `TopFiles`
summary lists, using internal, non-serialized `Module.LanguageSourceFiles`.
Each repository-relative path casts at most one vote, including overlapping
module records and workspace parents/children. C# and C++ accepted records obey
their existing scan budgets; detector aggregate counters are not added a second
time. Generic coverage still excludes companion/context files from weights.

Node modules inspect all safe source directories, not only `src`, `app`, `lib`,
`server`, `test` and `tests`. Those conventional roles remain; other directories
use `Additional Source`, without inventing framework-specific module contracts.
Nested `package.json` roots own their subtrees, so a workspace parent cannot
claim its children's sources. Even an otherwise empty `package.json` can define
a module when actual Node source or `tsconfig.json` is present.

Actual JS/TS source votes take precedence over `package.json` TypeScript hints or
`tsconfig.json`. A module can contain both languages; its single `Language`
field is the dominant accepted source language, while topology `Languages`
contains both. Manifest/config hints are retained for source-less module context
but do not cast topology language votes.

`PrimaryLanguage` is the language with the most unique accepted sources. Ties
use lexicographic language-name order, as before. `Languages` is alphabetically
sorted and includes only languages with positive source weights during a real
scan. With no recognized source, it and `PrimaryLanguage` report `Unknown`;
manifest-backed modules and stack information are still retained.

For the Go backend + TypeScript frontend + `scripts/build.py` example:

- `Languages`: Go, Python, TypeScript
- `PrimaryLanguage`: Go (2 Go and 2 TypeScript files, alphabetical tie break)
- `Stack`: Go Modules, Node.js
- `Structure`: Multi-Module; generic coverage does not invent a structural module

Top-level deployment/operations areas and shared static web resources keep their
existing language-neutral context policy. Ordinary source-language build/helper
scripts outside those areas can establish a language even when represented as
supplemental project context rather than a new structural module.

## Bounded traversal and exclusions

The shared recursive source walk is deterministic, does not follow symlinks, and
obeys the total entry/file budgets and configured directory caps. Node retains
its source-file-per-directory cap; generic coverage retains its entry-based cap.
Specialized detectors keep their own acceptance/budget rules. Thus weights reflect
the accepted scan, not files outside those bounds. Display summary caps do not
reduce source weights.

Dependency, cache and output directories remain excluded, including
`node_modules`, `.venv`, `venv`, `vendor`, `third-party`, `third_party`, `dist`,
`build`, `target`, `bin`, `out`, `coverage`, `.next`, `.nuxt`, `.svelte-kit`
and `cmake-build-*`. Marker and stack discovery also avoid these areas. C# sources
under `obj` retain their language-specific output exclusion; that name alone does
not suppress unrelated Ruby/Python/C++ source directories. Sensitive
file omission remains unchanged.

## Verification

Regressions in `internal/scanner/language_test.go`, `scanner_test.go` and
`node_test.go` cover source-only/manifest-only projects, all JS/TS extensions,
Vue script signals, mixed projects, unconventional directories, source summary
caps, workspace ownership, duplicate module records, deterministic ties and
ignored dependencies/outputs.

```sh
go test ./internal/scanner ./internal/model
go build ./...
go vet ./...
```
