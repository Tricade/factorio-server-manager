## Summary

- Describe the user-visible outcome and the smallest coherent set of changes.

## Failure mode or motivation

Explain the reproduced failure or the concrete limitation being addressed. Link the issue with `Fixes #N` when merge should close it.

## Safety and compatibility

- Describe how process lifecycle, active-profile state and profile-data locking remain consistent.
- Describe persistent-storage activation and rollback behavior when saves, mods, config or releases change.
- State authentication, path/input bounds, credential handling and outbound-traffic impact.
- State deployment, API, environment-variable and migration compatibility.

## Verification

- List the exact local tests, race checks, validators, container checks and manual scenarios run.
- Identify checks that were not run and explain why.

## Documentation and release

- [ ] `CHANGELOG.md` is updated for observable behavior, or this change has no user-visible effect.
- [ ] Relevant README, Docker, Unraid, environment, API and screenshot documentation is updated, or no such documentation changed.
- [ ] Generated frontend output, dependencies, credentials, logs and local data are not committed.
