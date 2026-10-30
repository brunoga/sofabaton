## Summary

<!-- What does this PR do and why? Link to a related issue if applicable. -->

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Refactor / internal improvement
- [ ] Documentation update

## Testing

<!-- How did you verify this works? New tests? Which hub and firmware for live tests? -->

## Checklist

- [ ] `go test -race ./...` passes
- [ ] `golangci-lint run` passes
- [ ] New decoders have a fuzz target and round-trip the captures
- [ ] Wire output matches the oracle vectors (or the difference is explained)
- [ ] Live tests run on a hub, if the change writes to one (state hub model)
- [ ] `docs/` updated with any new protocol fact and its confidence tag
