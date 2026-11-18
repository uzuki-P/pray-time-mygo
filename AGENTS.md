# Pray Time (MyGo)

The MyGo native UI port of the Kotlin app at `../pray-time`. Read the
justfile and README before changing the workflow; `just test` runs the unit
and headless view tests, and `just snapshots` renders the views to PNGs.

## Versioning

Always update the version with every change you ship.

- The version is `<major.minor from mygo.json>.<git commit count>`
  (`scripts/version`): every commit raises the last field by itself.
- A change users can see (a feature, new behaviour, a visible redesign)
  also bumps the minor version: run `just bump minor` and commit
  `mygo.json` with the change. A breaking change, such as a state file
  that older builds cannot read, bumps the major: `just bump major`.
  Fixes and internal changes need no bump; the commit count covers them.
- The build stamps the version into the app, which shows it at the
  bottom of Settings › General ("Pray Time 0.2.N"). Check it there after
  building.
- To publish a GitHub release of the pushed commit, run `just release`
  (tags `v<version>`; CI attaches the installers).
