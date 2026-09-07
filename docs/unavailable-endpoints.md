# Unavailable (disabled) endpoints

Some descriptor operations are **confirmed dead ends** on this account: a product or device
the account does not have, or a request the managed *child* device originates that a parent
CLI can never make. Each such op carries an `"unavailable": "<reason>"` field in the
descriptor. It is **disabled, not deleted** — the entry stays for auditability and is
re-enabled by clearing the field (e.g. after adding the device).

Effect on the CLI:

- `call <entity> <op>` **refuses** an unavailable op (even under `--dry-run`) and prints the reason.
- `entities` **hides** an entity whose every op is unavailable, and prints a one-line note listing them.
- `describe <entity>` still lists the ops, each marked `✗` with `[UNAVAILABLE: …]` — so the map is not lost.

Enforced by `TestDeadEndsDisabled` (descriptor guard).

## Currently disabled

Verified live 2026-09-07 against the logged-in app + CLI probes (a phone-child account with no
Gizmo Watch / pet collar / wearable):

| Entity | Ops | Why |
| --- | --- | --- |
| `messaging` | 19 | Family group-chat is **Gizmo-Watch-gated**. In the app, Chat/Call shows the Gizmo Watch upsell instead of any conversation. |
| `video_calling` | 3 | In-app WebRTC video calling is **Gizmo-Watch-gated**. In the app, a member's call button opens the **system phone dialer**, not an in-app video call. |
| `gizmo_activation` | 1 | Activates a **Gizmo Watch** during onboarding — no Gizmo on this account. |
| `wearable` | 5 | Pairs/onboards a child's **wearable watch (Gizmo)** — none on this account. |
| `pet_tracker` | 24 | A **pet collar tracker** (Jiobit/Fi) product — no such device on this account. |
| `tamper` | 15 | Inbound **child-device telemetry**: the managed child device posts the health of its own tamper protections. A parent CLI cannot originate these. |
| `installed_apps` | 1 | Inbound **child-device telemetry**: the child device reports its installed-apps inventory to the backend. Not a parent action. |

Total: **68 ops** across **7 entities**.

## Deliberately *not* disabled (reachable-but-restricted, different category)

- **Emergency** (`sos`, Safe Walk, `professional_monitoring` / 24-7 Assist, `roadside_assistance`): present and callable, but they dispatch to real emergency contacts / 911 — never triggered here for safety, not disabled.
- **Destructive** deletes (delete account/profile/device/subscription): guarded by `--confirm`, not disabled.
- **Runtime-state / two-sided** location ops (`location.checkIn`, live-location, etc.): reachable in principle but need a live GPS fix / the child device to respond; left enabled.
- **`app_block`, `flightdetection`**: mixed — they contain parent-callable ops (e.g. `app_block.blockApp`, confirmed live) alongside child-device senders, so the entity stays enabled.
