# safe_cli subcommand tree — consolidated design

## 1. Overview

Today every one of the 459 descriptor operations is reached through one generic dispatcher, `safe_cli call <entity> <op> [--service-id X] [--data '<json>'] [--query k=v]`. That interface makes the caller do the backend's job: hand-write a nested JSON body (`pauseInternet` wants `{"timeZone":..,"profiles":[{"profileId":..,"devices":[{"serviceId":..,"deviceId":..,"pauseSchedule":..}]}]}`), paste in profileId/deviceId values that `members` already printed, pick the right op out of 19 `location` ops or 13 `content_filter` ops, and format microsecond ISO timestamps by hand. The baseline probe failed on exactly these points (D1-D6). The replacement is an area-first tree, `safe_cli <area> <verb> [--child <service-id>] --flags`, where each verb is one parent intent, every flag is a typed semantic value with a stated default, the CLI resolves internal ids itself, and a wrong body is impossible because there is no body flag. `call` stays as the power-user escape hatch and nothing is removed from it.

### Grammar: area-first (decided)

`safe_cli <area> <verb> [--child <service-id>] [--flags]`. Examples: `safe_cli pause-internet pause --child 9002 --for 30m`, `safe_cli filter show --child 9002`, `safe_cli location where`.

Why area-first and not bare subject-first (`safe_cli alex pause-internet 30m`): the consumer is an LLM agent working from `--help`. `safe_cli --help` cannot list `alex` because member names are account data, not grammar; the first token would be ambiguous with area names; and account-level ops (`account show`, `location where`) have no subject at all. GAM's subject-first only works through an explicit `user` keyword, which is still weaker than area-first for discovery. Area-first makes every step discoverable: `safe_cli --help` lists areas, `safe_cli <area> --help` lists verbs (primary verbs first), `safe_cli <area> <verb> --help` lists every flag including `--child`, with type, default and effect. The target is the explicit `--child` flag, omitted for account/self ops. No positional-after-verb form is proposed; nothing in the surveys produced a case where it beat the flag for clarity.

### The agent-consumer balance

The consumer happily makes several round trips: `members` first, then the action. So the design does not chase one-shot magic. Composable primitives (`members`, per-area `list`/`show`) are the lookups; friendly verbs do only the resolution that is cheap and obvious (service-id -> profileId/deviceId from one account read; a category id -> its name/categoryId from one categories read). What matters most is discoverability and predictability: complete `--help` at each level, clean `--json`, and a tree regular enough that an agent constructs a correct call from help text alone.

## 2. Target resolution (decided)

- `--child <service-id>` takes the SERVICE-ID that `safe_cli members` prints (the `x-fp-identifier-target-serviceid` value). Finding it is a separate lookup the agent chains: `members` (optionally `members --find alex`, section 8). No fuzzy name matching or disambiguation lives inside verbs.
- From the service-id the CLI resolves, internally and per invocation, everything an op's body/query/path needs: `profileId` (`userProfileId`), `deviceId`, `pairingStatus`, and the account `timezone`, all from one `account.getAccountDetails` read (the same call `members` makes today; cached for the process). The user never types profileId/deviceId. Examples that need it: `pause_internet.pauseInternet` (profileId, serviceId, deviceId in the body), `calls_and_texts.deleteContactFromTheList` (profileId, deviceId as query), `app_block.getBlockedApps` (`{deviceId}` path placeholder), `location.fetchHistory` (`profileId` query), `restricted_usage.addLimit` (profileId/deviceId in body).
- Guardian-side ids are resolved the same way from the token: `postSchedule.notifyMember` and `postScheduleAlert.events[].profileId` take the logged-in guardian's profileId (`$self.profileId`), `updateLocationSharingSetting.events[].profileId` likewise.
- Enrichment lookups, only where a body needs fields the user cannot reasonably know: `filter block --category 10003` reads `content_filter.getCategories` once to fill `name`, `categoryId`, `categoryShortName` for `updateSubcategory`; `places set --name Home` reads `geofence.getGeofenceSettings` to fill the server-assigned `geofenceId`/`eventId`. These are declared per op (section 4), never implicit.
- `--child` is omitted for account/self ops: `account show`, `account set`, `location where` (whole family), `alerts settings show` (account-wide), `member invite`, `watch list`, `roadside *`, `plan *`, `dashboard show`. Their target header is the caller's own service id from the token, as `members` does today.
- Advanced overrides stay: `--service-id` (raw header), `--profile-id`, `--device-id` bypass resolution; they are global and not re-listed per verb.
- Device-scoped verbs (`target: device`) check `pairingStatus` and refuse an UNPAIRED target with an actionable error (D3): `member 9002 is UNPAIRED: pause-internet pause has no effect until the child's phone is paired (safe_cli pairing show --child 9002). Pass --allow-unpaired to send anyway.` `members` moves PAIRING to the column right after ROLE and adds `paired: true|false` to its JSON rows.

## 3. Conventions

Verb vocabulary (reused everywhere; one verb = one parent intent):

| Verb | Meaning |
|---|---|
| `list` | read a collection |
| `show` | read one thing / current state |
| `add` | create an entry (returns its id) |
| `set` | write settings, or update an entry by `--<thing>-id`; for singleton settings (`screen-time set`) the CLI reads the current id and creates or updates |
| `remove` | delete an entry (`--confirm` when descriptor `destructive`) |
| `enable` / `disable` | flip a boolean feature; takes a positional feature name only when the area has more than one (`website enable safe-search`, `calls enable trusted-only`) |

Domain verbs, allowed only where they are the parent's own word: `pause`/`resume` (pause-internet), `block`/`allow` (filter, apps, website), `where`/`history`/`track` (location), `approve`/`decline` (time requests, buddy requests, invites), `invite` (member, family-line), `find-my` (watch — pings a Gizmo watch's find-my; NOT the child's location, which is `location where`), `log`/`top` (calls), `request`/`cancel`/`status` (roadside, pick-me-up), `start`/`stop`/`extend`/`escalate`/`mark-safe` (safe-walk), `provision`/`deprovision` (family-line), `link`/`unlink` (watch), `mark-read` (alerts), `send`/`validate` (pin), `usage`/`insights`/`categories`/`requests` (screen-time reads).

Area naming: task-oriented kebab-case nouns a parent would say (`pause-internet`, `filter`, `screen-time`, `downtime`, `app-limits`, `calls`, `places`). Several entities merge into one area when that reads better (`device` = account devices + pairing device ops + device_settings + security_threat + vpn_status). Two levels is the norm; a third level (`<area> <sub-resource> <verb>`) is allowed only for a real sub-resource with three or more verbs: `device settings show|set`, `alerts settings show|set`, `roadside vehicle add|set|remove|makes|models`, `watch media list|backups|remove-backup`.

Flag naming: kebab-case, typed, one semantic meaning each. `--child` is the target. Ids of things inside an area are `--<thing>-id` (`--schedule-id`, `--limit-id`, `--entry-id`, `--event-id`, `--trip-id`). Counts are `--minutes`, `--mph`, `--radius` (meters). Times: `--start 22:00`/`--end 06:00` (24h HH:MM), `--days mon,tue|weekdays|weekends|all`, `--date today|yesterday|YYYY-MM-DD`, time ranges `--last 7d|24h|90m` / `--since` / `--until` (accept `YYYY-MM-DD`, RFC3339, or a duration back from now; expanded to whatever the op wants: `yyyy-MM-dd'T'HH:mm:ss.SSSSSS'Z'`, epoch-ms, or yyyy-MM-dd) with the raw value still accepted as advanced input (D4). `--timezone` defaults to the account timezone from `getAccountDetails`, converted to the form the op wants (IANA, or the short code `EST`/`PST` that `pauseInternet`, `postSchedule` and `postScreenTimeData` require) and the help says so. Every flag's help states type, allowed values, default and effect (D5): kong `default:` and `enum:` tags render them, and the generator appends `(default: X)` where a default is computed (`(default: account timezone)`). Every `target: device` verb's own `--help` also carries its PAIRED prerequisite line and names `--allow-unpaired`, so an agent calling it cold is warned before the refusal rather than only by this section.

Global flags (inherited, not re-listed): `--json`, `--plain`, `--dry-run` (prints the exact request the verb would send, including the resolved ids; never sends), `--confirm` (required by descriptor `destructive` ops and by live-emergency verbs), `--service-id`/`--profile-id`/`--device-id` (advanced overrides), `--allow-unpaired`, `--force` (send an op marked `unavailable`). `--dry-run` works before any auth beyond the account read; when resolution itself would fail, `--dry-run` prints the unresolved placeholders instead.

Output: table by default, `--json` returns the backend response with a small `_meta` (resolved target ids, op name) so an agent can chain. Errors name the verb, the target, and the next command to run.

## 4. How it is generated

The descriptor stays the single source of truth. Each op gains an optional `cli` block; the kong tree is generated from it.

```json
"pauseInternet": {
  "method": "POST", "path": "/frisco/parental-control/v5/device/pause", "...": "...",
  "cli": {
    "area": "pause-internet", "verb": "pause", "priority": "core",
    "target": "device",
    "summary": "Pause the child's internet now, for a fixed time or until you resume it.",
    "prereq": ["The child's phone must be PAIRED (safe_cli members shows PAIRING)."],
    "auth": "id_token",
    "body_template": "{\"timeZone\":\"$tz\",\"profiles\":[{\"profileId\":$child.profileId,\"devices\":[{\"serviceId\":$child.serviceId,\"deviceId\":$child.deviceId,\"pauseSchedule\":$for?,\"untilIUnpause\":$indefinite,\"callOnlyMode\":$callOnly}]}]}",
    "flags": [
      {"name": "for", "type": "enum", "enum": ["30m","1h","2h","4h","until-morning"], "default": "30m",
       "maps_to": "body:$for", "transform": "pause_schedule",
       "help": "How long to pause. until-morning lifts the pause at the child's next morning. Ignored with --indefinite."},
      {"name": "indefinite", "type": "bool", "default": false, "maps_to": "body:$indefinite", "nulls": ["for"],
       "help": "Pause until you run `pause-internet resume` (sends untilIUnpause=true and OMITS pauseSchedule)."},
      {"name": "call-only", "type": "bool", "default": false, "maps_to": "body:$callOnly",
       "help": "Leave phone calls working while data is paused."},
      {"name": "timezone", "type": "tz", "default": "$account.timezone", "maps_to": "body:$tz", "transform": "tz_short",
       "help": "Short zone code (EST, PST) the pause timing is computed in. Default: the account timezone."}
    ],
    "resolve": ["$child.profileId", "$child.serviceId", "$child.deviceId", "$account.timezone"],
    "output": {"table": ["status", "timeLeft"]}
  }
}
```

Fields:

- `area`, `verb`, optional `group` (the third level), `priority` (`core|common|long-tail`), `summary` (one line for `<area> --help`; core verbs are listed first, D2), `prereq[]` (rendered as "Prerequisite:" lines in the verb's help, D6).
- `target`: `account` | `self` | `child` | `device`. `child`/`device` add `--child` (required); `device` adds the pairing check (D3). `account`/`self` use the caller's service id.
- `auth`: `id_token` (default) | `spc_token` (the verb mints `family_line.getSpcToken` with `{"tokenIssued":$now.epochMs,"grantType":"token"}` first and sends the op with that `Authorization`; `--dry-run` prints both requests). This automates family_line's two-step flow (D6) once the SPC-gated routes are wire-verified; until then the help says "Prerequisite: Family Line SPC token (minted automatically; not yet wire-verified)".
- `flags[]`: `name`, `type` (`string|int|float|bool|enum|duration|date|datetime|tz|list`), `enum[]`, `default` (literal or `$account.timezone`/`$now`), `required`, `repeatable`, `excludes[]`, `maps_to` (`body:$var` into `body_template`, `query:<name>`, `header:<name>`, `path:<placeholder>`), `transform` (a fixed registry in Go: `pause_schedule`, `tz_short`, `iso_micro`, `epoch_ms`, `day3_lower`, `day3_title`, `weekday_ints`, `bool01`, `allow_block_ab`), `help`. `excludes[]` may only name flags that have no `default`: a defaulted flag is always populated, so an exclusion against it could not tell "only `--indefinite`" from "`--indefinite --for 30m`" and would either reject every indefinite pause or none; precedence over a defaulted flag is expressed with `nulls[]` instead (the nulled flag's `"$var?"` property is omitted and its help says it is ignored). A verb-level `at_least_one: [flags]` requires at least one of the named optional flags (`account set`).
- `body_template`: the descriptor's `body_example` with `$vars`. A property written as `$var?` is **omitted from the body entirely** when its variable is unset or nulled by another flag's `nulls[]` — this is how `--indefinite` sends `untilIUnpause=true` with no `pauseSchedule`, matching the wire-verified indefinite body (the app sends the field as absent, not empty); `excludes[]` only rejects flag combinations and carries no omission semantics on its own. Every retained field must be **classified**: mapped (a flag's `$var`), resolved (a `resolve` entry), or a declared constant in `constants` (that is how `MAPPVersion`, `editSource`, `productType` stay baked — explicitly, by name). Any `body_example` field that is none of the three is **rejected** by the descriptor test, because examples carry request-specific sample data, not only protocol constants: `geofence.createDeviceGeofenceSettings`' sample `geofenceId` must be dropped (create must omit the server-assigned id), and `invite.sendInvite`'s unrelated pet, contact and Wi-Fi fields must never ship from `member invite`. The test asserts every `$var` is a flag's `maps_to` or a `resolve` entry, every constant is declared, and no unclassified field remains.
- `query`: an explicit map of query-parameter values — constants and flag-mapped variables — because the descriptor's name-only `query[]` cannot carry a value and several zero-flag verbs need one that today lives only in prose or op metadata: `filter set` needs `{"categorySupported": "v6"}`, `places list` needs `{"strategy": "NotNull"}`, `calls log` maps `{"startDate": "$since", "endDate": "$until"}`. Validated like the body template: every name in the op's `query[]`/`required_query` must be a flag's `maps_to: query:<name>` target, a `query` constant, or explicitly marked optional; a required name with no source fails the descriptor test. Values already baked into the op's path string (`?activityType=call,text&timezone=preferred`) stay there and are not re-declared.
- `resolve[]`: the variables the runtime must fill: `$child.serviceId|profileId|deviceId|pairing`, `$self.serviceId|profileId`, `$account.id|timezone`, `$now.epochMs`, `$uuid`, and `$lookup:<entity>.<op>:<key>=<flag>:<field>` for enrichment (e.g. `$lookup:content_filter.getCategories:id=category:name`).
- `variants`: `{ "account": "notifications.getReportSettings2", "child": "dashboard.getReportSettings" }` for the few verbs whose op depends on whether `--child` is given (`alerts settings show|set`, `emergency-contacts list`).
- `alias_of`: on duplicate entries that hit the same route (`content_filter.createAppLimit` -> `schedules.createAppLimit`, `contacts.addContactToTheList` -> `calls_and_texts.addContactToTheList`, `pairing.getReportSettings` -> `notifications.getReportSettings`) so they generate nothing and `describe` points at the canonical verb.
- `call_only: true` with `reason` for ops deliberately left on `call` (device-originated telemetry, SDK plumbing). A descriptor test asserts every available op has either a `cli` block, an `alias_of`, or `call_only`, so nothing is forgotten silently.
- `live_emergency: true` requires `--confirm` and prints a warning (safe-walk escalate/start, pro-monitoring help, roadside request).

Generation: `go:generate` runs `internal/descriptor/gen` which emits `cmd/safe_cli/zz_generated_tree.go`: one kong struct per area (with `help:` from the area summary in a new top-level `areas` map in the descriptor), one struct per verb with typed fields and tags (`name`, `help`, `default`, `enum`, `required`), and a `Run(rc)` that calls the shared engine `invoke(rc, opRef, values)`. The engine does: load tokens -> resolve target (cached account read) -> pairing guard -> enrichment lookups -> build body/query/headers/path from the template -> `--dry-run` or send -> render `output`. `make lint` gains a drift check (regenerate and `git diff --exit-code`), the same way `go mod tidy` is checked. `call`, `raw`, `members`, `entities`, `describe`, `auth` are unchanged; `describe <entity>` adds a CLI column showing `pause-internet pause` next to `pauseInternet`.

## 5. The command tree

Format: `verb <- entity.op [priority]`. Long tail grouped at the end of each area as `call: ...`. Ops marked unavailable in the descriptor (62) are not generated (section 8).

**members** (top-level, exists) `members [--find <text>] [--role child|guardian]` <- account.getAccountDetails [core]. ROLE renders `child`/`guardian` (the API's DEPENDENT/GUARDIAN; `dependent` accepted as an alias) so nothing has to be inferred, and `--json` rows add `is_child`. PAIRING column promoted to right after ROLE (D3). Rows are sorted deterministically — guardians first, then children with PAIRED before UNPAIRED, then by name — and `members --help` states that order, so "the first child" is well-defined and actionable.

**pause-internet** [pause_internet] — `status` <- getDevices [core]; `pause` <- pauseInternet [core]; `resume` <- unPauseInternet [core].

**filter** [content_filter] — `show` <- getFilterContent [core]; `set --preset none|young-child|child|teen` <- createGroupPolicy [core]; `block --category <id>` <- updateSubcategory enabled=true [core]; `allow --category <id>` <- updateSubcategory enabled=false [core]; `categories [--search-engines]` <- getCategories [common] (help: "every category and its id, for block/allow — not what is blocked; see show"); `presets` <- getAgeGroupMetaData [common]. call: getSubCategoriesDetail, getParentalControls, setCFCategories (bulk form of block/allow). Objectionable-alert settings live in `alerts settings`.

**apps** [app_block, app_management] — `list` <- app_block.getBlockedApps ({deviceId} resolved) [core]; `block --app <id>` <- blockApp enabled=true [core]; `allow --app <id>` <- blockApp enabled=false [core]; `status` <- app_management.getAppStatus [common]; `set --app <id> --enabled=bool` <- updateAppStatus [common]. call (device-originated): sendAccessibilityStatus, sendBlockStatus, installed_apps.sendInstalledApps (unavailable).

**app-limits** [schedules; content_filter aliases] — `list` <- getAppTimeLimits [core]; `add --app <id> --minutes N [--days] [--all-day]` <- createAppLimit [core]; `set --limit-id` <- updateAppLimit [common]; `remove --limit-id` <- deleteAppLimit [common]; `approve|decline --limit-id [--minutes]` <- acceptDeclineAskTimeRequest [common]. call: postAppTimeLimit (child-side ask).

**screen-time** [schedules; web_and_apps alias] — `show` <- getScreenTimeData [core]; `set --mon 60 ... --weekdays 60 --weekends 120` <- postScreenTimeData / putScreenTimeData (upsert) [core]; `remove` <- deleteScreenTimeData [common]; `usage --date` <- getScreenTime (profile-id/device-id resolved) [common]; `insights` <- schedules.getInsights [common]; `categories --date` <- schedules.getCategories [common]; `requests` <- getAskTimeRequests [common]; `approve|decline --request-id --minutes --day` <- actionOnScreenTimeData [common]. call: askForMoreScreenTime, putAppUsageStats, updateSchedulerRunStatus.

**downtime** [schedules] — `list` <- getSchedules [core]; `add --type bedtime|school|custom --name --start --end --days [--mode block|alert]` <- postSchedule [core]; `set --schedule-id ...` <- putSchedule [common]; `remove --schedule-id` <- deleteSchedule [common].

**usage-limits** [restricted_usage] — `list` <- getAllTheLimits [core]; `set --type data|text|call --threshold N [--alert-only]` <- addLimit [core]; `remove --type` <- resetLimit [common].

**chores** [todo] — `list` <- invoke(getTodos) [core]; `add --message --start --minutes [--repeat weekly --days] [--reward]` <- createTodo [core]; `set --todo-id` <- updateTodo [common]; `remove --todo-id` <- deleteTodo [common].

**location** [location, real_time_tracking] — `where [--child] [--refresh] [--last-known]` <- getDashboardDetails [core] (`location --help` opens with "where is my child right now → `where`"; `watch find-my` only pings a Gizmo watch and is not a location query); `history --child --since|--last` <- fetchHistory [core]; `track --child` <- manageLiveLocationRequest [common]. call: getHistoryStatus, checkIn, checkInSeen, postLocationTamper, real_time_tracking.invoke, real_time_tracking.getHistoryEvents.

**places** [geofence, location] — `list` <- getSavedLocations [core]; `show --name|--event-id` <- getGeofenceSettings [common]; `add --name --lat --lon [--radius 150] [--alert arrive|leave|both] [--address...]` <- createDeviceGeofenceSettings [core]; `set --name ...` <- updateDeviceGeofenceSettings (geofenceId enriched) [common]; `remove --name|--event-id` <- location.deleteGeofenceSettings [common]. call: postGeofenceViolationEvent, sendGeoFenceConfirmation.

**location-alerts** [schedule_alert; schedules aliases] — `list` <- getScheduledAlerts [common]; `add --at "9:00 PM" [--days|--every-day]` <- postScheduleAlert [common]; `set --alert-id` <- updateScheduledAlert [common]; `remove --alert-id` <- deleteScheduledAlert [common].

**location-sharing** [location] — `show [--child]` <- getLocationSharingSettings / getWithWhomIamSharingLocation [common]; `enable|disable --child` <- updateLocationSharingSetting [core]; `set --default auto|manual` <- updateLocationSharingSettingConfig [common]. call: getLocationSharingConfigEvent.

**pick-me-up** [location] — `request --child --lat --lon|--address` <- pickMeUp [common]; `respond --child --accept|--decline` <- putPickMeUp [common]; `status --child` <- getPickMeUpStatus [common]. call: getAvailableParentForPickMeUp.

**flight-detection** [flightdetection] — `show`, `enable`, `disable` <- getFlightDetectionStatus / updateFlightDetectionSetting [common]. call: updateFlightDetectionStatus.

**steps** [step_counter] — `show --period daily|weekly` <- getChartStepsTracking [common]; `set --goal N` <- setStepGoals [common].

**calls** [calls_and_texts; contacts aliases] — `list [--list all|blocked|trusted|watchlist]` <- getAllTrustBlockWatchContactList [core]; `add --number --list blocked|trusted|watchlist [--name]` <- addContactToTheList [core]; `remove --number --list` <- deleteContactFromTheList [core]; `enable|disable block-unlisted` <- contacts.putPrivateRestrictedCall [common]; `enable|disable trusted-only` <- contacts.updateTrustedContacts [common]; `show` (trusted-only state) <- contacts.getTrustedContacts [common]; `log --last 7d [--number]` <- getCallAndTextActivityListV7 / getCallAndTextSpecificContactActivityListV7 [core]; `top --last 7d` <- getTopContactListByActivityV7 [common]; `summary --last 7d` <- getCallAndTextProfileSummaryListV7 [common]; `name --number --name` <- addCustomNameToAddressBook [common]. call: addNamesToAddressBook, getSchedulesRequest.

**contacts** [contacts] (Gizmo watch contact book) — `list` <- getGizmoContacts [core]; `add --number --name [--relationship]` <- addCallingContact [core]; `set --number ...` <- updateContact [common]; `remove --number` <- removeContact [common]; `available` <- getFamilyMembersAndBuddies [common]; `validate --mdn` <- validateGizmoMdn [common]; `permissions --video-calling=bool ...` <- updatePermissions [common]; `approve|decline --number` <- acceptDeclineBuddyRequest [common]. call: bulkUpdateContacts, bulkDeleteContacts, getGizmoContactImage, getInAppBanner.

**account** [account, user_setting, legal] — `show` <- getAccountDetails [core]; `set [--family-name] [--timezone]` (at least one — `at_least_one`) <- updateFamilyNameOrTimeZone (reads current so the untouched field is resent, not nulled) [common]; `leave --confirm` <- deleteMyself [long-tail]; `enable|disable keep-signed-in` <- user_setting.updateUserSettings [common]; `request-export`, `export-link` <- legal.* [long-tail]. call: selfRemoveProfile.

**member** [account, profile, invite, dashboard, contacts.sendInvite, pairing] — `set --child --name|--dob|--timezone|--avatar-url` <- updateProfile [core]; `remove --child --confirm` <- deleteProfile [common]; `invite --name --dob --mdn [--device-os]` <- invite.sendInvite [core]; `add-line --line <id>` <- dashboard.sendInvitation [common]; `lines` <- dashboard.queryEligibleLines / invite.getAccountLines [common]; `add-wifi-device --name --nickname` <- invite.createWifiDevice [common]; `resend-invite --child` <- profile.reSendInvite [common]; `accept-invite|decline-invite` <- contacts.acceptDeclineFamilyInvite [long-tail]. call: updateProfileName (alias of `set --name`), updateProfileImage, getProfileImage, attestDob, profile.getProfileAvatars, onboarding.getProfileAvatars, pairing.createProfile, pairing.createDependentProfile, pairing.updateProfileImage, dashboard.createProfile, contacts.sendInvite, invite.sendStandaloneInvite, invite.getAccountLines2, pairing.reSendInvite.

**device** [account, pairing, device_settings, security_threat, vpn_status] — `status --child [--refresh]` <- pairing.getDeviceStatus [core]; `set --child --name` <- account.updateDeviceName [common]; `add --child --mdn --type` <- account.addDevice [common]; `remove --child --confirm` <- account.deleteDevice [common]; `threats --child` <- security_threat.getThreats [common]; `monitoring --child` <- vpn_status.getWebAppVisibility [common]; `check-upgrade`, `upgrade` <- pairing.checkUpgrade / upgrade [common]; `power-off --child --confirm` <- pairing.powerOffGizmoDevice [common]; `settings show|set --child ...` <- device_settings.getDeviceSettings / postDeviceSettings [common]; `logs list|collect|url` <- device_settings.* [long-tail]. call: account.addOnboardingDevice, pairing.getDeviceStatus2, getDeviceShadowDetails, pairing.getDeviceSettings/postDeviceSettings/getDeviceLogs/... (aliases).

**pairing** [pairing] — `show --child` <- getConsent [common]; `consent --child --grant` <- setConsent [common]; `set --child --status --parental-consent` <- updatePairing [common]; `otp-status --child` <- getOtpStatus [common]; `resend-otp --child` <- resendOtp [common]. call: getIdTokenUsingOtp, getWebAppVisibility, getDefaultDOHLocation.

**watch** [pairing gizmo ops, wearable, gizmo_activation, invite] — `list` <- getGizmoDevices [common]; `find-my --child` <- pairing.findGizmo [common] (pings the Gizmo watch's find-my; its help says: not the child's location — use `location where`); `lookup --mdn` <- getMdnLookupResponse [common]; `check-eligibility --mdn` <- gizmoDevicesCheckEligibility [common]; `import-list`, `import` <- gizmoImportDevices / gizmoImportInitiate [common]; `validate --imei --iccid | --mdn` <- validateGizmoActivation / validateGizmoMdn [common]; `link --code --mdn` <- linkGizmoAccount [core]; `unlink --confirm` <- unlinkGizmoAccount [common]; `onboard --name --dob --mdn --model` <- wearable.onboardWearableWatch [common]; `retry-pairing --child` <- invite.retryPairing [common]; `replace-device --child --imei --iccid --mdn` <- invite.replaceDevice [common]; `resend-invite --child` <- wearable.resendInvite [common]; `media list|backups|remove-backup --media-ids --confirm` <- getMediaList / getMediaBackupList / deleteMediaBackupEntries [common]. call: gizmoDevicesGetLists, gizmoImport*NotSignedInUser, gizmoImportEligibility, getMediaBackupStorageStatus, getInteractionData, gizmo_activation.validateGizmoActivation, invite.setRelationships, wearable.confirmWatchPairing/watchAuth/notifyGuardianFromDependantWatch (unavailable).

**alerts** [notifications, dashboard, content_filter objectionable] — `list --child [--per-page]` <- getNotifications [core]; `count --child` <- getNotificationCount [core]; `show --child --event-id` <- getObjectionableWeb [common]; `mark-read --child` <- markAllRead [common]; `settings show [--child]` <- getReportSettings2 (account) / dashboard.getReportSettings (child) [common]; `settings set [--child] --objectionable-alerts=bool --new-contact-alert=bool --daily-summary-alert=bool ...` <- updateReportSettings (account) / dashboard.postReportSettings (child) [common]. call: getNotificationFeed, getFilters, notifications.getReportSettings, content_filter.get/postObjectionableSettings (aliases), dashboard.getReportSetting/postReportSetting, pairing.getNotificationFeed/getObjectionableWeb/getReportSettings.

**tamper** [dashboard, tamper] — `show --child` <- dashboard.getViewBanner [common]; `detail --child --feature` <- getViewBannerDetails [common]; `set --child --template` <- putTamperInstructions [common]; `restore --child` <- dashboard.putRestrictionsBackOn [long-tail]. call: all 14 tamper.* telemetry posts (unavailable, device-originated).

**pin** [accessibility_pin] — `show --child [--new]` <- retrievePin [core]; `send --child` <- sendPin [common]; `validate --child --pin` <- validatePin [common]. call: age-capture trio, getDeviceShadowDetails.

**website** [website, web_and_apps] — `list --child` <- web_and_apps.getWebsites2 [core]; `block --child --url a.com [--url ...]` <- postWebsites status=b [core]; `allow --child --url` <- postWebsites status=a [core]; `remove --child --entry-id` <- deleteWebsite [common]; `enable|disable safe-search --child` <- enableSafeSearch / disableSafeSearch [common]. call: postWebsite (single-entry alias).

**activity** [web_and_apps, most_used_apps, app_management, activity_tracking, profile] — `apps --child [--date] [--count 10]` <- most_used_apps.getTopApps [core]; `websites --child --date` <- web_and_apps.getWebsite [core]; `categories --child --date` <- web_and_apps.getCategories [common]; `insights --child` <- web_and_apps.getInsights [common]; `app-usage --child` <- app_management.getAppUsages [common]; `timeline --child [--date]` <- activity_tracking.getActivity / getDailyActivities [common]. call: getWebsites (legacy), getWebsiteDetails, getAppUsageDetails, app_management.getInteractionData, profile.getTopApps (alias).

**safe-walk** [sos, safety_alerts] — `status --child` <- getWatchMeSoSSessionInfo [core]; `alerts` <- safety_alerts.getSafetyAlerts [core]; `history --child --session-id` <- getWatchMeSoSHistory [common]; `insights --child` <- watchMeSosInsightSummaryList [common]; `enroll --child --pin` <- postWmsOnboardProfile [common]; `set --child --pin` <- manageWmsPin [common]; `mark-safe --child --session-id` <- markSafeWatchMeSoSRequest [common]; `extend --child` <- extendSoSSession [long-tail]; `escalate --child --session-id --confirm` (live emergency) <- escalateToSoSRequest [long-tail]; `start --child --lat --lon --confirm` (live) <- submitWatchMeRequest [long-tail]. call: watchMeSosAlerts, updateSafeWalkProfile.

**pro-monitoring** [professional_monitoring] — `show`, `enroll`, `set`, `address`, `validate-address`, `set-address`, `deactivate --confirm`, `reactivate --confirm` [common]; `help --confirm` (live emergency) [long-tail]. call: none.

**medical-id** [medical_id] — `add`, `set` <- postMedicalId / putMedicalId [common].

**emergency-contacts** [emergency_contacts] — `list [--child]` (variant: profile / account) [core]; `add --child --contact <profileId-of-member>` <- addEmergencyContactsToProfile [core]; `set --child --contact ...` (replace) <- updateEmergencyContacts [common]; `remove --child --contact-id --confirm` <- deleteEmergencyContact [common].

**school-calendar** [calendar_sync] — `schools --zip`, `suggest`, `show`, `events --child [--holidays]`, `select --school-id`, `set --subscription`, `remove --school-id --confirm` [core/common]. call: getCalendarSelection, updateCalendarSelection, completeResyncOperation, getGoogleCalendarTokens.

**family-line** [family_line] — `status --child` <- getProvisioningStatus (id_token, wire-verified) [common]; `list` <- getFamilyLines; `eligible` <- getEligibleLines [common]; `provision --child` <- provisionUser [core]; `deprovision --child --confirm` [common]; `invite --child` <- sendFamilyLineInvite [core]; `remove-member --child --confirm` [common]; `address --child` <- getAddress; `set-address --child --street-number ... --postal-code` <- saveAddress [core]; `validate-address` [common]; `set --child --dnd=bool` <- updateFamilyLineSettings [common]. All except status/trace carry `auth: spc_token` and the help prerequisite line (D6). call: termsAndConditionsAccepted, acceptFamilyLineInvite, swapDeviceProvisioning, logoutFamilyLine, getSpcToken, updateAppUuid, sendCallLog, traceSdkResponse.

**permissions** [feature_permissions, invite] — `show --child` <- getParentalControlFeaturePermissions [common]; `list` <- getManagedUserProfiles [common]; `set --child --location-management=bool --parental-controls=bool --elevated-access=bool` <- updatePermissions [common]; `set-role --child --role --relationship` <- changeUserProfileRole [common]. call: updateUserProfileAccess, invite.getFeaturePermissions, invite.updateFeaturePermissions.

**plan** [account, subscription, dashboard] — `show` <- dashboard.getSubscriptions [common]; `set --plan` <- account.updatePlan (choosePlan for first subscription) [long-tail]; `cancel --confirm` <- subscription.cancelSubscription [long-tail].

**dashboard** [dashboard] — `show` <- dashboardFlow [common]. call: getEligibility, getParentingTips, acknowledgeLuciqPush.

**driving** [driving_insights, p2] — `trips --child`, `trip --trip-id`, `summary`, `crashes [--since]`, `crash --crash-id`, `settings`, `enable`, `set --crash=bool --trip-end=bool --passenger=bool`, `speed-limit --mph`, `reclassify --trip-id --mode`, `resolve-crash --crash-id --dismiss|--false-alarm` [core/common]. call: updateCrashNotifications, getAuthCode.

**roadside** [roadside_assistance, p2] — `request ... --confirm` (live dispatch), `cancel --rescue-id`, `status`, `history`, `access`, `set-access --enable --consent`, `vehicle add|set|remove|makes|models` [common]. call: getTowLocations, validateTowLocation, dismissRsaNotificationBanner, setUpRsaIntroBottomSheet, dismissWhatsNew.

**setup** [setup_wizard] — `tasks --child`, `complete --child --task-id` [common]. **age-verify** [age_verification] — `submit --child --id-data`, `keys` [long-tail]. **offers** [cross_sell] — `list`, `promotions`, `trials`, `dismiss` [common]. **services** [services_hub] — `list`, `status` [common]. **whats-new** `show` [long-tail]. **reviews**, **config**, **realtime** [pubnub], **auth** identity ops beyond the shipped `auth login|refresh|logout` — `call` only.

Not generated (descriptor `unavailable`, 62 ops): pet_tracker (22), messaging (19), tamper telemetry (14), video_calling (3), wearable device-side (3), installed_apps (1). Reachable via `call --force`; the generator picks them up automatically when `unavailable` is cleared.

## 6. Flagship areas

### pause-internet (D1, D3)

```
safe_cli members --find alex                  # NAME  ROLE       PAIRING   SERVICE-ID ...
safe_cli pause-internet status --child 9002   # status Paused/Unpaused, timeLeft, valid pause timings
safe_cli pause-internet pause --child 9002 --for 1h --call-only
safe_cli pause-internet resume --child 9002
```

Ops: `status` <- `pause_internet.getDevices`; `pause` <- `pause_internet.pauseInternet` (body built from the template in section 4: `pauseSchedule` from `--for` via `pause_schedule` (`30m`->`30_minutes`, `1h`->`1_hour`, `until-morning`->`Until_tomorrow_morning`), `untilIUnpause` from `--indefinite` (which omits `pauseSchedule`, as the app does), `callOnlyMode` from `--call-only`, `timeZone` short code from the account timezone, profileId/serviceId/deviceId resolved); `resume` <- `pause_internet.unPauseInternet` (DELETE, no body; a "Device already unpaused" 500 is reported as "already resumed").

Rendered `safe_cli pause-internet pause --help` — what an agent actually sees; the enum, defaults and prerequisite are explicit rather than implied by examples:

```
Usage: safe_cli pause-internet pause --child=SERVICE-ID [flags]

Pause the child's internet now, for a fixed time or until you resume it.

Prerequisite: the child's phone must be PAIRED (safe_cli members shows PAIRING).
              An UNPAIRED target is refused; pass --allow-unpaired to send anyway.

Flags:
      --child=SERVICE-ID   The child, by the SERVICE-ID that `safe_cli members` prints. Required.
      --for=30m            How long to pause: 30m|1h|2h|4h|until-morning (default: 30m).
                           until-morning lifts the pause at the child's next morning. Ignored with --indefinite.
      --indefinite         Pause until you run `pause-internet resume`.
      --call-only          Leave phone calls working while data is paused.
      --timezone=TZ        Short zone code (EST, PST) the timing is computed in (default: account timezone).
```

D1 before/after:

```
before: safe_cli call pause_internet pauseInternet --service-id 9002 \
          --data '{"pauseSchedule":"30_minutes","timeZone":"EST"}'     # flat body, server 500
after:  safe_cli pause-internet pause --child 9002 --for 30m
```

There are no body flags; the only inputs are `--for`, `--indefinite`, `--call-only`, `--timezone`, so the flat shape cannot be produced. D3: `pause`/`resume` are `target: device`; on an UNPAIRED member they refuse with the message in section 2 unless `--allow-unpaired`.

### filter (with apps, website) (D2)

```
safe_cli filter show --child 9002                       # the one answer to "what is blocked"
safe_cli filter set --child 9002 --preset teen
safe_cli filter categories --child 9002                 # ids to use with block/allow
safe_cli filter block --child 9002 --category 10003
safe_cli apps block --child 9002 --app 101
safe_cli website block --child 9002 --url example.com
```

Ops: `filter show` <- `content_filter.getFilterContent` (app-name: VSF header auto-sent); `filter set` <- `createGroupPolicy` (`--preset none|young-child|child|teen` -> groupId 1..4; help says a preset replaces current subcategory filters); `filter block|allow` <- `updateSubcategory` with `enabled` true/false, `name`/`categoryId`/`categoryShortName` enriched from `getCategories` by the given id; `filter categories` <- `getCategories` (`--search-engines` sets include-safesearch=true); `filter presets` <- `getAgeGroupMetaData`. `apps list` <- `app_block.getBlockedApps` (deviceId placeholder resolved); `apps block|allow` <- `blockApp`, same enrichment. `website list` <- `web_and_apps.getWebsites2`; `website block|allow` <- `postWebsites` (status b/a, repeatable `--url`); `website remove --entry-id` <- `deleteWebsite` (profileDomainId from `website list`); `website enable|disable safe-search`.

D2 before/after:

```
before: safe_cli describe content_filter   # 13 ops; agent guessed
after:  safe_cli filter show --child 9002
```

`safe_cli filter --help` lists `show` first with "See what is currently blocked for this child"; the reads that are not the answer (getSubCategoriesDetail, getParentalControls) are not verbs.

### location (with places) (D2, D4)

```
safe_cli location where                          # whole family, last known
safe_cli location where --child 9002 --refresh   # ask the phone for a fresh fix
safe_cli location history --child 9002 --last 7d
safe_cli places add --child 9002 --name School --lat 37.42 --lon -122.08 --radius 200 --alert both
```

Ops: `where` <- `location.getDashboardDetails` (account target; `--refresh` -> onDemand=true, `--last-known` -> onlyLastKnownLoc=true; `source`, `locationEnabled`, `locPermission` describe the caller's phone and are baked; `--child` filters rows client-side); `history` <- `fetchHistory` (profileId resolved; `--last`/`--since` -> startTime, format pinned by the transform once confirmed on the wire); `track` <- `manageLiveLocationRequest` (serviceIds/profileId/accountId resolved; device fields baked). `places list` <- `geofence.getSavedLocations`; `places add` <- `createDeviceGeofenceSettings` (`--alert arrive|leave|both` -> geofenceType onEntry/onExit/all; deviceId/profileId/accountId resolved; `eventId` = `$uuid`, `eventDateTime` = `$now`); `places set` <- `updateDeviceGeofenceSettings` (geofenceId/eventId enriched from `getGeofenceSettings` by name); `places remove` <- `location.deleteGeofenceSettings` (eventId enriched, eventType=savedLocation baked).

D2 before/after:

```
before: safe_cli call location getDashboardDetails --service-id <own> --query onDemand=true   # one of 19, low confidence
after:  safe_cli location where
```

### screen-time (with downtime, app-limits)

```
safe_cli screen-time show --child 9002
safe_cli screen-time set --child 9002 --weekdays 60 --weekends 120
safe_cli downtime add --child 9002 --type bedtime --name Bedtime --start 21:30 --end 06:30 --days all
safe_cli app-limits add --child 9002 --app 10061 --minutes 45 --days weekdays
safe_cli screen-time requests --child 9002
safe_cli screen-time approve --child 9002 --request-id <id> --minutes 30 --day monday
```

Ops: `screen-time show` <- `schedules.getScreenTimeData`; `set` <- `postScreenTimeData` or `putScreenTimeData` (upsert: reads the existing `screenTimeLimitId`; `--mon..--sun`, `--weekdays`, `--weekends`, `--all` expand to `weeklyLimits`; timeZone short code defaulted); `remove` <- `deleteScreenTimeData`; `usage` <- `getScreenTime` (profile-id/device-id resolved, `--date` default today); `requests` <- `getAskTimeRequests`; `approve|decline` <- `actionOnScreenTimeData`. `downtime add` <- `postSchedule` (`--type bedtime|school|custom` -> nhr/shr/cus; `--mode block|alert` -> blockContent/alertOn, default block; days `day3_title`; `notifyMember` = `$self.profileId`); `set` <- `putSchedule`; `remove` <- `deleteSchedule` (schedule-id query). `app-limits add` <- `createAppLimit` (days `day3_lower`, `--all-day` -> blockAllDay); `approve|decline` <- `acceptDeclineAskTimeRequest`.

### calls (with contacts)

```
safe_cli calls list --child 9002 --list blocked
safe_cli calls add --child 9002 --number 5551230000 --name Spam --list blocked
safe_cli calls enable trusted-only --child 9002
safe_cli calls log --child 9002 --last 7d
```

Ops: `list` <- `calls_and_texts.getAllTrustBlockWatchContactList` (profileId/deviceId query resolved; `--list` filters client-side); `add` <- `addContactToTheList` (contactType from `--list`, profileId/deviceId resolved); `remove` <- `deleteContactFromTheList`; `enable|disable block-unlisted` <- `contacts.putPrivateRestrictedCall`; `enable|disable trusted-only` <- `contacts.updateTrustedContacts` (settingId 8000 baked, settingValue 1/0); `log` <- `getCallAndTextActivityListV7` (`--number` switches to `getCallAndTextSpecificContactActivityListV7`); `top` <- `getTopContactListByActivityV7`; `summary` <- `getCallAndTextProfileSummaryListV7`; `name` <- `addCustomNameToAddressBook` (x-fp-identifier-deviceid resolved). `contacts list|add|set|remove` <- `getGizmoContacts` / `addCallingContact` / `updateContact` / `removeContact` (Gizmo watch contact book; help says so).

D4 before/after:

```
before: safe_cli call calls_and_texts getCallAndTextActivityListV7 --service-id 9002 \
          -q startDate=2026-09-01T00:00:00.000000Z -q endDate=2026-09-08T00:00:00.000000Z
after:  safe_cli calls log --child 9002 --last 7d
```

`--last 7d` (default) or `--since 2026-09-01 --until 2026-09-08` expand to `yyyy-MM-dd'T'HH:mm:ss.SSSSSS'Z'` via `iso_micro`; the raw form is still accepted by `--since`/`--until`, and `--dry-run` shows the expansion.

### account and member (with members)

```
safe_cli members
safe_cli account show
safe_cli account set --family-name "Rivera Family" --timezone America/Los_Angeles
safe_cli member set --child 9002 --name "Alex R."
safe_cli member invite --name Sam --dob 2013-04-02 --mdn 5551234567 --device-os iOS
```

Ops: `members` <- `account.getAccountDetails` (unchanged op; PAIRING column promoted, `--find`); `account show` <- `getAccountDetails` (raw); `account set` <- `updateFamilyNameOrTimeZone` (reads current familyName/timezone first so the untouched field is resent, x-pending-activation=false baked); `member set` <- `updateProfile` (PATCH with only the flags given); `member remove` <- `deleteProfile` (destructive, `--confirm`); `member invite` <- `invite.sendInvite` (v6; `--mdn/--device-os/--device-model` fill `devices[0]`, providerId VZW baked); `member add-line` <- `dashboard.sendInvitation`; `member lines` <- `dashboard.queryEligibleLines`; `member resend-invite` <- `profile.reSendInvite`.

## 7. Reconciliation notes

- Area names: `gizmo` + `watch` -> `watch` (a parent says watch). `invites` + `invite` -> verbs on `member` (`invite`, `add-line`, `add-wifi-device`) and `family-line invite`. `call-log` -> `calls log|top|summary`. `security`, `monitoring`, `device_settings`, pairing's device ops, account's device ops -> one `device` area. `protection` + `tamper` -> `tamper` (show/detail/set). `reports`, notification settings from four entities -> `alerts settings`. `settings`, `privacy` -> verbs on `account`. `media`, `notifications` (pairing cluster) -> `watch media` and `alerts`. `location-sharing` and `pick-me-up` stay separate areas (distinct tasks; keeps `location` to three verbs). `schedules`' four scheduled-alert ops are aliases of `schedule_alert` and generate nothing.
- Verbs: `preset` -> `filter set --preset`; `rename` -> `member set --name` / `device set --name`; `clear` -> `remove`; `update` -> `set`; `respond` -> `approve|decline`; `safe-search-on/off` -> `enable|disable safe-search`; `block-private`/`trusted-only` -> `enable|disable block-unlisted|trusted-only`; `where` kept as the single "where is my kid" verb and `getDashboardDetails` is its only op. A `calls block` shorthand was rejected: it would be asymmetric with `remove --list` and `add --list` is unambiguous.
- Duplicate ops across entities (same route): schedules vs content_filter app-limit ops, contacts vs calls_and_texts list ops, pairing vs device_settings/notifications, most_used_apps vs profile top-apps, web_and_apps vs schedules insights. One canonical op generates the verb; the rest carry `alias_of`.
- Same-verb, op-by-target: `alerts settings show|set` and `emergency-contacts list` pick the account-wide or per-child op by whether `--child` is present (`variants`). Used nowhere else.
- Stay on `call` by design: device-originated telemetry (tamper.*, app_block.sendBlockStatus/sendAccessibilityStatus, schedules.putAppUsageStats/updateSchedulerRunStatus, flightdetection.updateFlightDetectionStatus, location.postGeofenceViolationEvent/postLocationTamper), SDK/auth plumbing (identity.*, pubnub.*, config.*, driving_insights.getAuthCode, calendar_sync.getGoogleCalendarTokens, age_verification keys), child-side asks (askForMoreScreenTime, postAppTimeLimit), and the 62 unavailable ops. `multipart` ops (2) remain refused by `call` and get no verb.
- Safety: `safe-walk escalate|start`, `pro-monitoring help`, `roadside request` fire live emergency/dispatch flows on a real account; they require `--confirm` and print a warning (`live_emergency`).
- `members` keeps its top-level place (the documented start point); `member` is the per-member action area. No `member list` duplicate.

## 8. Open decisions

Status (2026-09-08): the owner delegated these to best judgment; **every recommendation below is adopted as the decision** and the implementation proceeds on it.

1. **Generate everything vs curate core+common and leave the long tail on `call`.** Recommendation: curate. Phase 1 ships `cli` blocks for the ~60 core and ~150 common ops (about half the available surface); the rest is `call_only` with a reason or `alias_of`. A descriptor test fails if an available op has none of the three, so the long tail is a deliberate list, not an omission. Generating all 397 available ops would put SDK plumbing next to `pause` in `--help` and hurt D2.
2. **`members --find <text>`.** Recommendation: yes, a case-insensitive substring filter on name plus `--role dependent|guardian`; it is the one lookup convenience and costs nothing. `--child` stays service-id only. Optional variant: also accept an exact, unique member name in `--child` (the account read is already made), still no fuzzy matching; default is to not do this until asked.
3. **How much internal resolution.** Recommendation: exactly one cached `getAccountDetails` per invocation for serviceId -> profileId/deviceId/pairing/timezone (plus the guardian's own ids from the token), and explicit `$lookup` enrichment only where a body needs fields a user cannot know (filter/apps block, places set/remove, screen-time upsert, account set). No chained writes, no name resolution. Everything else the agent chains (`filter categories` then `filter block`). `--profile-id`/`--device-id` remain as overrides.
4. **Generation mechanism.** Recommendation: `go:generate` codegen from the descriptor into `cmd/safe_cli/zz_generated_tree.go` (kong structs + Run stubs into one engine), with a `make lint` drift check. Kong is struct-tag driven; runtime construction would lose typed flags, enums, defaults and per-verb help, which is the whole point.
5. **Unavailable ops.** Recommendation: not generated; `call --force` only. Clearing `unavailable` in the descriptor makes the generator emit them on the next `go generate`, so a Gizmo/pet account gains them without code changes.
6. **Confirm gating and the third level.** Recommendation: add `live_emergency` (requires `--confirm` and warns) alongside `destructive`; allow the `<area> <sub-resource> <verb>` form only for `device settings`, `alerts settings`, `roadside vehicle`, `watch media`, and keep everything else at two levels.

## 9. Pressure test (fresh Sonnet via `claude -p`)

A fresh Sonnet agent with no prior context was given the same five parent tasks twice: first with only today's generic CLI (it could run the binary), then with only this design's help text (no binary). Its confidence and what it had to guess:

| Task | Today's `call` | This design |
|---|---|---|
| List members, identify children | high | high |
| Pause the first child 30 min, then resume | medium — produced a **malformed body** (the flat shape the server 500s on) | high — `pause-internet pause --child <svc> --for 30m`; no body is possible |
| What is blocked | medium — guessed among 13 content_filter ops | high — `filter show`, "the one answer" |
| Calls/texts, last 7 days | medium — hand-built microsecond ISO timestamps | high — `calls log --child <svc> --last 7d` |
| Where is the child now | **low** — guessed among 19 location ops | medium — one naming collision, fixed below |

Tweaks applied from the design probe's remaining confusions: `device find` (← `pairing.findGizmo`, a Gizmo watch find-my ping) became `watch find-my` so `find` no longer competes with `location where`, and `location --help` opens with the where-is-my-child answer; `members` renders ROLE as `child`/`guardian` rather than the API's DEPENDENT and adds `is_child` to JSON; `members` output order is deterministic and stated (guardians, then PAIRED children, then UNPAIRED, then by name); each `target: device` verb's `--help` carries its PAIRED prerequisite and `--allow-unpaired`; the `--for` enum and defaults render in the verb's help (section 6 shows it); `filter categories`' help says it is the id catalog, not what is blocked. The two probe harnesses are reusable for re-testing once the tree is generated, so the same five tasks become a regression check on the real `--help`.
