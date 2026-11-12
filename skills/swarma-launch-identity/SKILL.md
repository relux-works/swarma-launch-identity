---
name: swarma-launch-identity
description: Derive a public synthetic launch selector from stable IDs before an identity-less launch through an already admitted Curator route. Explicit identity launches use their existing route.
---

Use the manager's reviewed authoritative mapping for host_id, project_id and
profile_id. Preserve those explicit public IDs through directory moves and
profile updates. If an assignment is missing, stop; never derive one from a
path, hostname, process, profile version or environment identifier.

Run the installed command before curator run:

```sh
swarma-launch-identity derive --host-id VALUE --project-id VALUE --profile-id VALUE
```

Capture the exit status and complete raw stdout without shell command
substitution stripping the LF. Admit output only on exit 0, empty stderr and
exactly 70 bytes: `syn1:`, 64 lowercase hexadecimal digits, then one LF. Discard
any partial, trailing or failed frame. Refusals contain only `ERR CODE` and LF;
see the README for codes and evaluation order.

An existing A2A_AGENT, even empty, requires the separate explicit-identity
route. Never place the result into A2A_AGENT, remove its prefix, enroll it or
activate another identity's hooks. SWARMA_LAUNCH_IDENTITY must be absent or
already equal to the derived name.

Supply the validated public name without its frame LF via
SWARMA_LAUNCH_IDENTITY only on a qualified no-proxy Codex route through the
existing admitted Curator command. Before adoption, review and pin the
nonsecret key-name/selection configuration projection, exclude later overlays
and auth-token-name pointers selecting this key, and retain an inert-child
exact-value transport witness. A conflicting overlay can warn and overwrite;
the docking checker must fail that configuration. Unknown or changed routes
need qualification before real use. Do not inspect auth values or dump a
private configuration or environment.

The utility only derives and prints a public selector. It authenticates nobody,
grants no credential access and does not assign stable IDs, provision accounts,
write homes, start sessions, implement resume persistence or prove receiving
route safety. The manager separately owns namespace separation, stable mapping,
the same-UID trust limit, home persistence and caller integration acceptance.

## Caller template

A launching manager may copy `examples/review-seat-launch.zsh.tmpl` into its
own reviewed launch configuration. Replace every documented value placeholder
with a shell-quoted literal stable ID, matching expected name, pinned utility
path, seat and prompt path; replace the command placeholder with the admitted
Curator command path. Fill the argument placeholder with separately quoted
argv words. Keep the clean `env -i` tmux start and the explicit cleanup list.
The example captures
`derive | check-frame`, stops on any nonzero pipeline status, and exports
`SWARMA_LAUNCH_IDENTITY` only after frame validation succeeds.

The template is an example and does not authenticate a name, access a
credential, or qualify the selected Curator receiving route. A manager must
perform its own route and exact-value checks before real use; same-UID processes
remain within the existing trust boundary.
