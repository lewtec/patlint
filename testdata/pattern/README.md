# Pattern fixtures (`rft grep` / `rft rewrite`)

Unified fixture form: [`skills/refactree/references/testing.md`](../../skills/refactree/references/testing.md).
`kind = "grep"` or `kind = "rewrite"`; `scenario/` + `test.toml`; rewrite also has `expected/`.

Engine + harness: `pkg/pattern` (`go test ./pkg/pattern/`).

**Pattern language:** Lisp sexpr only. See [`SPEC.md`](../../SPEC.md) **Pattern algebra**.

```text
rft grep '(token "interface{}")' testdata/pattern/go_interface_to_any/scenario
rft rewrite '(token "interface{}")' 'any'
rft test testdata/pattern
```

Replacement is sexp emit: atom / `(slot)` / `(ref …)` / `(seq …)`. Same as `(rewrite MATCH EMIT)`.

---

## Layout

See the skill. Case dirs use `scenario/` + `test.toml` (`kind = "grep"` / `"rewrite"`).

## Golden set (kept as-is)

| Directory | Mode | Notes vs locks |
|-----------|------|----------------|
| `go_failed_to_prefix` | rewrite | whole-match emit via `(ref …)` + literal args |
| `go_interface_to_any` | rewrite | Aligns: grammar tokens |
| `go_strings_splitn` | grep | Aligns: capture F = `strings.SplitN`, lit `2` |
| `go_listen_and_serve` | grep | Aligns: capture F = `http.ListenAndServe` |
| `go_unify_if_return` | rewrite | unify a/b: max-shaped if/return; reject mismatched return |
| `go_unify_if_return_alt` | rewrite | Group alt collapses if/return vs if/else/return → `max` |
| `go_error_not_last_result` | grep | error not last in results; Kleene multi over leading slots + named arm |
| `go_discarded_error_blank` | grep | `_` before `:=`/`=` and a call; optional `[T]` type args |

## Example patterns (locked style)

```text
(token "interface{}")
(seq (capture F (ref "go:strings::SplitN")) "(" (capture S any) "," (capture SEP any) "," "2" ")")
(seq (unify a any) ">" (unify b any) ":" (unify a any) "?" (unify b any))
(ref "go:testing::T")
```

## Checks

```bash
go test ./pkg/pattern/
go run ./cmd/rft test testdata/pattern
go run ./cmd/rft grep '(token "interface{}")' testdata/pattern/go_interface_to_any/scenario
```
