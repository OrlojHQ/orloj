## Summary

Describe what changed and why.

## Validation

List the commands you ran and key outcomes.

```bash
# Example
go test ./api ./controllers ./store -count=1
go run ./cmd/orlojctl validate -f examples/
```

## Checklist

- [ ] I ran relevant tests for the packages I changed.
- [ ] I updated docs/examples if behavior changed.
- [ ] I added a changelog entry under `[Unreleased]` when user-visible behavior changed.
- [ ] I kept this PR focused and avoided unrelated refactors.
- [ ] I included DCO sign-off (`git commit -s`).

## Risk and Rollback

Note operational risks and rollback steps if this change causes regressions.
