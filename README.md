# jomaa-everything

Home projects, kept in one repo.

| Folder | What it is |
|--------|------------|
| `jomaa-home-hub/` | Matter controller hub. Runs on the home network and controls Matter and Govee devices through one interface. |
| `jomaa-home-client/` | Phone apps for the hub: Android (`android/`) and iOS (`ios/`). |

## Checks

Run `python3 jomaa-home-hub/scripts/verify.py` before pushing. It runs the
formatter, module checks, `go vet`, strict golangci-lint, builds for x86, Pi
and Mac, race-detector tests, a vulnerability scan, and the Python tests. CI
runs the same script.

Turn on the pre-commit hook once per clone, so commits that break the hub's
format, lint or build are refused:

```bash
git config core.hooksPath .githooks
```
