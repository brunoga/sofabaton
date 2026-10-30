# Oracle vectors

`oracle.py` runs the upstream Python implementation's pure builders and
parsers and writes their outputs to `testdata/oracle/*.json`. The Go tests
check that our code produces the same bytes, or document why it does not.

It runs upstream code, so it uses its own virtualenv with only upstream's
declared dependencies, and it never talks to a hub:

```sh
git clone https://github.com/m3tac0de/home-assistant-sofabaton-x1s /tmp/upstream
git -C /tmp/upstream checkout 5fd5887d11272a553da99383f327c53cb8b4190e
make oracle UPSTREAM=/tmp/upstream
```

The vectors are committed; regenerate them only when moving to a new
upstream commit (update `PINNED_COMMIT` in `oracle.py` and the capture
corpus together).
