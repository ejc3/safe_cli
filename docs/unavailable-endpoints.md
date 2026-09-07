# Unavailable (disabled) endpoints

Some descriptor operations are **confirmed dead ends** on this account: a product or device
the account does not have, or a request the managed *child* device originates that a parent
CLI can never make. Each such op carries an `"unavailable": "<reason>"` field in the
descriptor. It is **disabled, not deleted** — the entry stays for auditability and is
re-enabled by clearing the field (e.g. after adding the device).

The `unavailable` reason is **advisory**: it was observed on one account's device inventory,
but the descriptor ships embedded in the binary, so the CLI cannot know the caller's own
account. It therefore steers agents off a likely dead end without hard-blocking a different
account that does have the device.

Effect on the CLI:

- `call <entity> <op>` **refuses by default** and prints the reason — but `--force` sends it
  anyway (for an account that *does* have the device), and `--dry-run` is never blocked (it only
  prints the request, makes no call).
- `entities` **hides** an entity whose every op is unavailable, and prints a one-line note listing them.
- `describe <entity>` still lists the ops, each marked `✗` with `[UNAVAILABLE: …]` — so the map is not lost.

Enforced by `TestDeadEndsDisabled` (descriptor guard) and `TestUnavailableDeadEndsHiddenAndRefused`
(CLI behaviour, incl. the `--force`/`--dry-run` bypass).

## Currently disabled

Verified live 2026-09-07 against the logged-in app + CLI probes (a phone-child account with no
Gizmo Watch / pet collar / wearable):

| Entity | Ops | Why |
| --- | --- | --- |
| `messaging` | 19 | Family group-chat is **Gizmo-Watch-gated** (verified live: Chat/Call shows the Gizmo upsell). Fully hidden. |
| `video_calling` | 3 | In-app WebRTC video calling is **Gizmo-Watch-gated** (verified live: a member's call opens the system dialer). Fully hidden. |
| `installed_apps` | 1 | Inbound **child-device telemetry** (the child reports its app inventory). Fully hidden. |
| `tamper` | 14 | Child-device tamper-status reports. The parent-facing `putTamperInstructions` (same route as `dashboard.putTamperInstructions`) stays available, so the entity is still listed. |
| `pet_tracker` | 22 | Pet-collar-specific ops (live tracking, wifi, firmware). The route-shared/general ops — `getPurchaseLink` (buy one) and `getAllAvailableEmergencyContacts` — stay available. |
| `wearable` | 3 | Gizmo-wearable-specific ops (`confirmWatchPairing`, `watchAuth`, `notifyGuardianFromDependantWatch`). The general `resendInvite` and the shared-route `onboardWearableWatch` stay available. |

Total: **62 ops** disabled across 6 entities (3 fully hidden; `tamper`/`pet_tracker`/`wearable` keep their route-shared or parent-facing ops).

**Invariant:** an op is never disabled if its `(method, path)` route is also served by an available op — otherwise the CLI would block functionality reachable via a sibling. Enforced by `TestNoUnavailableSharesRouteWithAvailable`. This is what un-disabled the earlier over-reach (`gizmo_activation.validateGizmoActivation` = `pairing.validateGizmoActivation`, the pet_tracker read aliases, `wearable.resendInvite`).

## Deliberately *not* disabled (reachable-but-restricted, different category)

- **Emergency** (`sos`, Safe Walk, `professional_monitoring` / 24-7 Assist, `roadside_assistance`): present and callable, but they dispatch to real emergency contacts / 911 — never triggered here for safety, not disabled.
- **Destructive** deletes (delete account/profile/device/subscription): guarded by `--confirm`, not disabled.
- **Runtime-state / two-sided** location ops (`location.checkIn`, live-location, etc.): reachable in principle but need a live GPS fix / the child device to respond; left enabled.
- **`app_block`, `flightdetection`**: mixed — they contain parent-callable ops (e.g. `app_block.blockApp`, confirmed live) alongside child-device senders, so the entity stays enabled.
