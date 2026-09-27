# patlint

Pattern linter. It matches source by structure, reports findings, and can rewrite the match.

```bash
patlint grep '(token "interface{}")' ./pkg
patlint rewrite '(token "interface{}")' 'any' ./pkg
patlint run .patlint -- .
```

Rules are `.rft` files. A directory pack loads `$dir/*.rft` and `$dir/.patlint/*.rft` (not recursive). `patlint run` with no pack uses the git repository root. `patlint run --fix` applies non-overlapping rewrites.

