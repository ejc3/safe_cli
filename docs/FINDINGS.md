# SafePath feasibility — findings

_Static analysis of Verizon Family (Smith Micro SafePath), app
`com.verizon.familybase.parent` **v8.101.30** (build 810100030). 2026-08-23._

This resolves the "single deciding question" from the original feasibility notes:
**does the SafePath backend enforce device attestation or per-request signing?**

## TL;DR — VIABLE

A scripted, bearer-token client for the SafePath parental-control API looks
**buildable and durable**. Auth is standard OAuth2/OIDC; the attestation
machinery in the app is confined to an optional FIDO login path, not a
per-request gate on the REST API.

## How the APK was obtained

The dev box's egress proxy blocks APK mirrors and there was no phone available to
`adb pull`. With the account owner's explicit authorization, the signed APK was
downloaded through their own Browserbase cloud browser (the apkcombo variant link
is IP-locked to the browser session, so it must be fetched through that browser),
then analyzed locally. Nothing about the app was executed; this is static
analysis of software the owner licenses and uses. The APK itself is **not**
committed (git-ignored).

## The deciding question

| Signal | Finding | Bearing on viability |
|---|---|---|
| **Device attestation** | Play Integrity classes present, but under `com.noknok.android.client.extension.PlayIntegrity` / `com.fido.uaf…UafEngine$PlayIntegrityProcessor` — the **Nok Nok FIDO/UAF** SDK. Used as a nonce/attestor in the *optional biometric login*, not on API calls. | **Not a blocker.** |
| **Firebase App Check** | None (0 references). | Not gated by App Check. |
| **Attestation HTTP header** | None (`X-Integrity`/`X-AppCheck`/`X-Attestation` = 0). The `X-*` headers are mundane device/crashlytics/AWS-S3 metadata. | **No per-request attestation.** |
| **Auth** | OAuth2/OIDC authorization-code + PKCE → bearer `access_token`/`refresh_token`/`id_token` (356 `/token`, 126 `Authorization`). A direct username/password→OTP→token path exists too. | **Replayable → viable.** |
| **Per-request signing** | A few `HmacSHA256`/`SignRequest`/`x-signature`/`computeSignature` refs; `X-Amz-Signature` is AWS SigV4 for direct S3 image uploads. | **Caveat, not a blocker** (any in-app-keyed signature is replayable). |
| **TLS cert pinning** | Present (`CertificatePinner`, `network_security_config`). | **Capture-only** — unpin to MITM; never affects viability. |

The crude first-pass "80 attestation hits" was inflated by the broad `attest`
substring (48 of the 80). The real Play-Integrity API references number ~20 and
are all inside the Nok Nok FIDO code.

## The backend

- **Base URL:** `https://api.prd.vsf.aws.vz-connect.com` (VSF = Verizon Safe
  Family; AWS-hosted; platform codename **frisco**).
- **Auth:** `client_id=6ebckm2cmaijai6kfb7251ar9a`,
  redirect `vsfapp://com.verizon.familybase.parent/signin`, `scope=openid profile`,
  `code_challenge_method=S256`. Verizon SSO is the identity provider.
- **Parental-control API:** `/frisco/parental-control/v5…v6` and `/frisco/v5…v8`
  and `/vsf/…`.
- **Full catalog:** `docs/discovered-endpoints.txt` — **183 paths** across
  parental-control, app-limits, screen-time, schedules, web/app activity,
  location, notifications, call-and-text, family-line, driving-insights,
  account-management, and auth.

## The dynamic capture (completed)

Static analysis could not confirm the exact login contract, whether any endpoint
gates on an integrity token, or the signing key. All three were resolved by a full
dynamic capture — see `docs/PROCESS.md` §7–§10 — which also established that login is a
**hybrid, multi-stage flow**, not a single OAuth exchange. (An early hypothesis of a
pure browser OAuth code-exchange was disproved: the `authorize` step is only one stage
and the device-auth calls are signed.)

## Confirmed by dynamic capture (2026-08-23)

Booted the app on an arm64 Cuttlefish (Android 17) and MITM'd its real sign-in
(mitmproxy + frida unpinning; see `docs/PROCESS.md` §7). Results:

- **VIABLE holds — no device attestation observed.** No Play Integrity / attestation
  challenge appeared on any captured call: the device-OTP send, `otp/validate`, the
  token exchange, or the authenticated API calls that followed.
- **Auth path prefix is `/auth/frisco/…`** for the device-auth calls (not `/frisco/…`),
  a cause of the earlier blind `400`s.
- **Device-auth requests are signed.** `x-signature` = HMAC-SHA256 (hex) over request
  **metadata** — `AppVersion + SourceApp + x-transaction-id + method + x-timestamp +
  x-appuuid` (NOT the body) — keyed by an app-embedded secret recovered from the APK
  (`BuildConfig.HMAC_SIGNING_SECRET`). Per-request signing, **not** attestation, and
  replicable. The token exchange and authenticated API calls are *not* signed.
- **Login is a hybrid, multi-stage flow** (see `docs/PROCESS.md` §9): it **begins** with
  the signed device-OTP API (`POST …/user/auth/otp` → `otp/validate` → a
  `login_recom_token`), then hands off to OAuth authorize + the hosted My Verizon web
  login + account 2FA, and finally a token exchange returns the real `access`/`refresh`/
  `id` tokens.
- **API auth:** `Authorization: <id_token>` (raw JWT, no `Bearer`) for config/content
  endpoints; AWS SigV4 (Cognito temp creds) for the parental-control operations.

The signing algorithm and full token model are recovered and verified — see
`docs/PROCESS.md` §9–§10.
