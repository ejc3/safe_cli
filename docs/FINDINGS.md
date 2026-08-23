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

## What still needs a dynamic capture

Two things static analysis cannot fully confirm:

1. That a headless OAuth2 login succeeds against Verizon SSO (MFA/OTP behavior).
2. That no individual endpoint independently demands an integrity token.

Because auth is a normal web OAuth2 flow (not app-only), this can be done by
driving the login in a browser and exchanging the code for tokens — no rooted
device or frida required. This is the step where the account owner's credentials
are needed.
