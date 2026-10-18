# diagram-design (vendored)

The diagram-design skill by Cathryn Lavery, MIT licensed (see `LICENSE`
and `THIRD_PARTY_LICENSES.md`). Agent Office ships it with the app and
gives it to the AI for every step that has a diagram output.

- Upstream: https://github.com/cathrynlavery/diagram-design
- Version: 2.6.110, commit `550fd66edff058bc73056c3416b76d69cfe865ba`
- Copied: `.claude-plugin/plugin.json`, `LICENSE`,
  `THIRD_PARTY_LICENSES.md`, `skills/` (unchanged). The slash commands,
  docs and repository scripts are left out.

To update, copy the same paths from a newer upstream commit, update the
version and commit above, and run `go test ./...` (the engine tests run
the skill's `scripts/self_check.py` against a sample diagram).
