# Process: how the SafePath protocol was (and is being) reverse-engineered

This is the end-to-end runbook behind `safe_cli` — how we established that a
durable client is viable, recovered the backend contract, and how we drive a real
device to capture the parts static analysis can't reach. It is written so the work
is reproducible and auditable.

> **Scope & authorization.** Everything here is interoperability research on the
> account owner's *own* Verizon Family account and their *own* licensed copy of the
> app, comparable to the community clients for Google Family Link, Microsoft Family
> Safety, and Nintendo parental controls. No credentials, tokens, or app binaries
> are committed to this repo.

## 0. The environment

The work runs on an AWS **Graviton (ARM64)** dev box that is itself a **Firecracker**
microVM. Relevant properties, all verified rather than assumed:

| Property | Result | Consequence |
|---|---|---|
| `/dev/kvm` + `KVM_CREATE_VM` | works (nested KVM) | can run a nested Android VM |
| `binder` kernel module | **absent** (`7.0.14-fcvm` kernel) | Waydroid impossible |
| egress | open (incl. `dl.google.com`, `ci.android.com`, GitHub, PyPI) | can fetch SDKs/images/tools |
| APK-mirror hosts | reachable via a cloud browser, not always direct | see §2 |
| CPU / RAM | 64 cores / 125 GB | comfortable for an emulator + build |

## 1. The deciding question

Is the SafePath backend replayable by a script, or does it enforce **device
attestation / per-request signing** that a scripted client can't forge? Everything
hinges on this. See `docs/FINDINGS.md` for the answer (**VIABLE**) and the evidence.

## 2. Static analysis (answered most of it)

1. **Get the signed APK.** The dev box can't always pull APK mirrors directly, and
   there was no phone to `adb pull` from. The signed APK was fetched through the
   owner's own **Browserbase** cloud browser (apkcombo's download link is IP-locked
   to the browser session, so it must be driven through that browser). Result: the
   334 MB XAPK for `com.verizon.familybase.parent` v8.101.30. The APK is **not**
   committed.
2. **Explode it.** `unzip` the XAPK → `base.apk` (+ arch/dpi split APKs). `base.apk`
   holds 28 `classes*.dex` (~1.6M string constants).
3. **Scan the dex string table** for the deciding signatures (attestation, request
   signing, cert pinning, auth shape) and for API hosts. The `safe_cli`
   analyzer productizes this; the raw pass lived in a shell script.
4. **Read the verdict.** The ~80 "attestation" hits were mostly the broad `attest`
   substring. The real Play Integrity references all belong to the **Nok Nok
   FIDO/UAF** SDK (`com.noknok…`, `com.fido.uaf…`) — the *optional* biometric login
   path, not a per-request gate. No Firebase App Check; no `X-Integrity`/`X-AppCheck`
   header. Auth is plain **OAuth2/OIDC bearer**. → durable client is viable.
5. **Recover the contract.** Base URL `https://api.prd.vsf.aws.vz-connect.com`
   (AWS-fronted, Cognito-backed, platform codename "frisco"); **183 endpoints**
   harvested from Retrofit path constants (`docs/discovered-endpoints.txt`); OAuth2
   authorize/token/refresh/otp routes; `client_id`, redirect scheme, PKCE.

## 3. Why a dynamic capture is still needed

Replaying the OAuth `authorize` endpoint from scratch returns **`400 Invalid
Request`**. Adding the (dynamic) `identity_provider` param moves it 500→400, but the
request still fails a server-side contract check — the app performs a
**device-registration handshake** (`registeruser` / `deviceauth`) and sends headers
whose values are minted at runtime, not shipped in the APK. Those aren't fully
recoverable from static strings. The reliable way to learn the exact request shape is
to **watch the real app make it.**

## 4. Dynamic capture on ARM (in progress)

The doorways to Android-on-ARM, evaluated on this host:

- **AVD emulator** — Google ships no `linux-aarch64` emulator host build (only
  `darwin-aarch64` and `linux-x86_64`). Not usable here.
- **Waydroid** — needs the host kernel's `binder`; the Firecracker kernel has none.
- **Cuttlefish** — Google's KVM-based Android-for-servers. Viable on ARM with KVM;
  this is the path. Built from source (`github.com/google/android-cuttlefish`, the
  `base` package is a Bazel C++ build; `frontend` is Go). Then a `cvd-host_package`
  + an `aosp_cf_arm64` image from `ci.android.com`, launched headless.

**MITM plan** once Android boots:

1. `adb install-multiple` the `base.apk` + split APKs.
2. Run `mitmproxy` on the host; point the guest's proxy at it.
3. Install mitmproxy's CA as a **system** cert (userdebug image → `adb root`,
   remount, push to the system CA store).
4. Bypass the app's `CertificatePinner` with **frida** + the
   [httptoolkit frida-interception-and-unpinning](https://github.com/httptoolkit/frida-interception-and-unpinning)
   scripts (`frida-server` arm64 pushed to the guest).
5. Drive the app to the phone-number sign-in; the account owner completes the SMS
   OTP. Capture the `registeruser` / `authorize` / `token` requests — headers and
   bodies — to fill the descriptor's `confirmed:false` gaps.

> **GMS caveat.** AOSP Cuttlefish images ship without Google Play Services, which the
> app uses (Firebase, Maps, Play-Integrity via Nok Nok). If the sign-in flow can't be
> reached without GMS, we add it to the guest properly (GMS-enabled image / GApps) —
> not by skipping the step.

### 4.1 Building the Cuttlefish toolchain (clean, reproducible)

There are no prebuilt Cuttlefish debs for arm64, so build from source. This exact
order is distilled from a build that hit every trap below — follow it as written and
a fresh box builds first-try.

**1. Install Bazel *first*.** The `base` package is a Bazel C++ build; without Bazel
on `PATH` its `debian/rules` aborts with *"Bazel install is broken"*.

```bash
sudo apt-get update
sudo apt-get install -y git devscripts equivs config-package-dev debhelper-compat golang curl
# bazelisk auto-selects the Bazel version the repo pins
curl -sL -o /tmp/bazelisk https://github.com/bazelbuild/bazelisk/releases/latest/download/bazelisk-linux-arm64
sudo install /tmp/bazelisk /usr/local/bin/bazel
```

**2. Build the debs — as your user, one at a time** (never under `sudo`):

```bash
git clone https://github.com/google/android-cuttlefish && cd android-cuttlefish
git checkout 786f4ac2be42519fd4b023a36114dd6ae7ffc04b   # the revision this build was validated on (v1.57.0-dev)
for d in base frontend; do
  ( cd "$d"
    sudo mk-build-deps -i -t 'apt-get -y'   # build-deps install *with* sudo …
    dpkg-buildpackage -uc -us -b )          # … the build itself must NOT be sudo
done
```

**3. Install + join the host groups:**

```bash
sudo apt-get install -y ./cuttlefish-base_*.deb ./cuttlefish-user_*.deb ./cuttlefish-orchestration_*.deb
sudo usermod -aG kvm,cvdnetwork,render "$USER"
# Log out and back in so ALL three groups apply — a plain `newgrp` only switches one
# group, leaving kvm/render inactive. Scripted equivalent: exec su -l "$USER"
```

**4. Fetch an image + launch headless:**

```bash
cvd fetch --default_build=aosp-main/aosp_cf_arm64_only_phone-userdebug
HOME="$PWD" launch_cvd --daemon --report_anonymous_usage_stats=n
adb connect 0.0.0.0:6520 && adb wait-for-device
```

**Traps this order avoids — each one cost a full rebuild:**

- **Never build under `sudo`.** A sudo build leaves root-owned files in
  `debian/<pkg>/…`; the next run's `dh_clean` then dies with `rm: … Permission
  denied`. Recovery: `sudo chown -R "$USER" base/debian` then rebuild.
- **Exactly one build at a time.** Concurrent `dpkg-buildpackage` runs fight over the
  single Bazel server and the shared `../*.deb` output, producing corrupt/no debs.
- **`debian/rules clean` wipes Bazel's cache**, so a *re-run* recompiles from scratch
  — budget ~15–20 min on a many-core box. The `base` link uses `-flto=auto`, so the
  final `ld.lld` is slow and mostly single-threaded: that is normal, not a hang.
- **Don't `pkill -f bazel`** from a shell whose own command line contains the word
  `bazel` — `pkill -f` matches your shell too and kills it. Kill by PID, or use
  `bazel shutdown` from the workspace.

## 5. From capture to code

Captured request shapes become the `method` + headers + body of each descriptor
operation, flipping `confirmed:false` → `true`. The generic OAuth2 + device-auth
client (parameterized by the descriptor) then performs real `auth login`, and the
`print/info/create/update/delete` + action commands light up. Because the descriptor
is the single source of truth, the same engine serves every SafePath **carrier**
(Verizon Family, T-Mobile FamilyMode, …) — one CLI, many provider descriptors.

## 6. Reproduce (abbreviated)

```bash
# Static
unzip verizon-family.xapk -d bundle
# the base APK holds the dex; the config.* entries are arch/dpi splits.
unzip "bundle/com.verizon.familybase.parent.apk" -d base
strings -n 5 base/classes*.dex | grep -EiC0 'IntegrityManager|PlayIntegrity|Bearer|/oauth|/token'
grep -hoE '(vsf|frisco)/[A-Za-z0-9._/{}-]*v[0-9]+/[A-Za-z0-9._/{}-]*' <(strings base/classes*.dex) | sort -u

# Dynamic (Cuttlefish, ARM): build the toolchain per §4.1 (clean, reproducible),
# then: cvd fetch -> launch_cvd --daemon -> adb install-multiple -> mitmproxy + frida unpin
```

## 7. Dynamic capture — results (Cuttlefish on ARM, confirmed)

The toolchain from §4.1 booted **Android 17 / arm64-v8a** headless on Cuttlefish. The
signed app installed with `adb install-multiple` (base + `config.arm64_v8a` +
`config.hdpi`) and — importantly — **ran to its sign-in screen without Google Play
Services** (the GMS caveat in §4 did not block login; only a Firebase-analytics job
warns). Driving the phone-number sign-in through mitmproxy + frida produced the real
request contract:

```
POST https://api.prd.vsf.aws.vz-connect.com/auth/frisco/frisco-iam-device-auth/v7/user/auth/otp
  x-source-app: AndroidMAPP          x-mobile-app-version: 8.101.30
  x-appuuid: <uuidv3>                x-timestamp: <epoch-ms>
  x-transaction-id: <40-digit>        x-trace-transaction-id: <uuidv4>
  x-signature: <64 hex>               content-type: application/json
  user-agent: okhttp/4.12.0
  body: {"mdn":"<10-digit>"}
→ 200 {"mdn":"…","state":"OTP_SENT","statusCode":200}
```

**Two corrections to the static guesses this forces:**

1. **The auth path prefix is `/auth/frisco/…`, not `/frisco/…`.** The blind replays in
   §3 used the wrong base path — one reason they returned `400`.
2. **Requests are signed.** The device-auth calls carry `x-signature` (64 hex =
   HMAC-SHA256) over request **metadata** — `AppVersion + SourceApp + x-transaction-id
   + method + x-timestamp + x-appuuid`, **not** the body (exact string verified in §11). This is the
   per-request signing hinted at statically (`SignRequest`/`computeSignature`/`HmacSHA256`). It is
   **not** device attestation and is **replicable** — the signing key ships in the app,
   and the CLI sources it at runtime from the operator's own copy rather than embedding
   it (§11). The exact signed string is now recovered and reproduced (§11); the
   `otp/validate` → token exchange is the remaining step.

### The working MITM recipe (what actually captured traffic)

1. `mitmdump -p 8080 -w flows.mitm` on the host; `adb reverse tcp:8080 tcp:8080` so the
   guest reaches it at `127.0.0.1:8080`.
2. Concatenate httptoolkit's scripts into **one** file (`config.js` first) and run them
   through the **frida CLI** (not raw bindings — see footguns): proxy override + system
   cert injection + certificate unpinning + fallback.
3. `frida -U -p <live-pid> -l combined.js` with `tail -f /dev/null` as stdin.
4. Drive the UI with `adb shell input`; confirmed decrypted HTTPS for mapbox, Firebase,
   Instabug, and `api.prd.vsf.aws.vz-connect.com`.

## 8. Footguns (each of these cost real time — avoid them)

- **frida 17 removed the built-in `Java`/`ObjC` bridges.** Raw Python `frida` bindings
  throw `ReferenceError: 'Java' is not defined`. Use the **frida CLI** (frida-tools
  bundles and auto-injects the bridges), or bundle `frida-java-bridge` yourself.
- **Load the unpinning scripts as ONE shared scope.** `config.js` defines globals
  (`PROXY_HOST`, `CERT_PEM`) the hook scripts read; loading each as a separate frida
  script isolates scopes → `PROXY_HOST is not defined`. Concatenate into one file,
  config first.
- **`frida-script.js` is a deprecated stub** now; load the per-platform scripts
  (`android/android-certificate-unpinning.js`, `…-fallback.js`,
  `android-proxy-override.js`, `android-system-certificate-injection.js`,
  `android-disable-root-detection.js`).
- **`CERT_PEM` must start exactly with `-----BEGIN CERTIFICATE-----`** (no leading
  newline) or `config.js` rejects it as "not in PEM format".
- **`pkill -f <word>` also matches your own shell** when `<word>` is in its command line
  (`pkill -f bazel`, `pkill -f frida`) → it kills the shell running it (exit 143/144).
  Kill by explicit PID, or use a graceful stop (`bazel shutdown`).
- **Prefer frida ATTACH over `-f` spawn for a multi-step UI flow.** The CLI's
  spawn + `sleep|frida` keep-alive can crash on shutdown ("could not acquire lock for
  stdin at interpreter shutdown") and lose the app on re-fork ("Process terminated").
  `frida -U -p <pid> -l combined.js` with `tail -f /dev/null` stdin is stable; auth
  calls fire on the sign-in tap, so attaching after launch still captures them.
- **The app gets a new PID on every `am start`/`monkey` when it isn't already alive**,
  which detaches frida. Attach to the **live** pid and don't relaunch mid-flow.
- **`adb shell input text` drops characters** when typing fast — a digit went missing
  from the phone number. Type digit-by-digit with short sleeps and verify the field
  value before submitting anything that costs an OTP send.
- **zsh does not word-split unquoted variables.** `A="adb -s 127.0.0.1:6520"; $A shell …`
  runs the whole string as one command name ("no such file or directory"). Put only the
  binary in the var and pass args literally, or use a function.
- **Cuttlefish external storage resolves to `/dev/null/Android/data/…`** in this image,
  so the app spams `SecurityException: Invalid mkdirs path` and can misbehave mid-flow
  (contributes to the app losing state). To investigate: ensure `/sdcard` is mounted /
  `EXTERNAL_STORAGE` is set before launching.

## 9. The full authentication flow (captured end-to-end)

Driving the real sign-in through the MITM showed login is a **three-factor,
multi-service chain**, not one API call. Captured request sequence (Akamai bot-sensor
noise omitted):

1. **SafePath device OTP** (line verification), signed frisco API —
   `POST /auth/frisco/frisco-iam-device-auth/v7/user/auth/otp` body `{"mdn":"<phone>"}`
   → `200 {"state":"OTP_SENT"}`  (SMS #1 to the line).
2. **Validate device OTP** —
   `POST /auth/frisco/frisco-iam-device-auth/v7/user/auth/otp/validate`
   body `{"mdn":"<phone>","otp":"<code>"}`
   → `207 {"state":"AM_LOGIN_PAGE","tokens":[{"token_type":"login_recom_token",
   "id_token":"…","expires_in":1800}]}`.
3. **Hand off to Verizon Account-Management (AM) login** —
   `GET /frisco/frisco-iam-device-auth/v5/oauth2/authorize` → `302` → the My Verizon
   hosted login at `secure.verizon.com/signin/oauth2/vendor/authorize` (loads in a
   WebView; its TLS is decrypted too because the WebView is in-process).
4. **My Verizon account login** (User ID / mobile + password), Akamai-protected —
   `GET …/vendor/api/v1/verifymdn`, `POST …/getconfig`, `GET …/getmvaurlswithflags`,
   then `POST secure.verizon.com/signin/gw/oauth2/vendor/api/v1/authenticate` → `200`.
5. **My Verizon account 2FA** — a *second* SMS, separate from step 1:
   `POST …/api/v1/initialize2fa` → `200` (SMS #2), then `POST …/api/v1/update2fa` with
   the code → an OAuth authorization code.
6. **Token exchange** — the code is exchanged back through frisco for the real
   `access`/`refresh`/`id` tokens the parental-control API accepts as
   `Authorization: Bearer`.

Notes:

- The device-auth calls are **signed**: `x-signature` (HMAC-SHA256) over request
  metadata (`AppVersion + SourceApp + x-transaction-id + method + x-timestamp +
  x-appuuid`, **not** the body — see §11), with `x-source-app: AndroidMAPP` and
  `x-mobile-app-version`.
- **Mixed path prefixes:** `/auth/frisco/…` for the OTP steps, `/frisco/…` for the OAuth
  authorize. `/auth/frisco/…/v5/user/login/audit` returns `400` and is non-blocking.
- **Architectural consequence for the CLI:** steps 3–5 are a *hosted web login*
  (secure.verizon.com, Akamai bot protection, **two** SMS factors) — not cheaply
  automated headlessly. So `auth login` should run this chain **once** via an assisted
  browser/WebView, capture the resulting **`refresh_token`**, and thereafter use the
  refresh token + the signed REST API directly. Everyday commands never repeat the full
  chain.

### 9a. The last-mile wall: bot protection on the My Verizon account login

Automating steps 4–5 end-to-end via `adb input` reaches the point of a **correct** 2FA
entry (code in the field, Continue enabled) but then the hosted login resets with
*"To keep your information safe, Please start over."* — a security/bot-detection reset on
`secure.verizon.com` (Akamai-style), triggered by the robotic interaction, **not** a
wrong code. Consequences:

- The **SafePath parental-control API is fully scriptable** once you hold tokens: clean
  signed REST (`Authorization: Bearer` + the `x-signature` set).
- The **initial token acquisition is not reliably headless** — Verizon's *account* login
  (steps 3–5) is a bot-protected hosted web flow with two SMS factors. So `auth login`
  should be an **assisted login**: let a human complete the `secure.verizon.com` login in
  a real WebView/browser once, capture the resulting **`refresh_token`**, and persist it.
  Everyday commands then use the refresh token + signed REST directly and never touch the
  hosted login again.
- Still fully recoverable headlessly (no bot wall — it's in-app): the **`x-signature`
  algorithm**, via a frida hook on the signing function. That plus a one-time
  assisted-login `refresh_token` is everything the CLI needs.

## 10. Token & API model — the durable CLI design

Proven by replaying captured tokens **from outside the emulator**:

- **Token set** from `POST /auth/frisco/frisco-iam-device-auth/v7/user/auth/token`:
  an **online** set (`id_token` JWT + `access_token` + `refresh_token`, `expires_in`
  1800) and an **offline** set (`id_token` + `refresh_token`, `expires_in` 86400).
- **Two auth schemes on the backend:**
  1. **Config / content endpoints** (e.g. `/auth/frisco/mappcontent/v6/configs`):
     `Authorization: <id_token>` — the **raw JWT, NO `Bearer ` prefix** — plus mundane
     headers (`x-source-app`, `x-mobile-app-version`, `x-transaction-id`,
     `user-agent: okhttp/4.12.0`). **Confirmed `200` replayed from a plain host.**
  2. **The parental-control operations** (`…/parental-control/…`) return `403
     "Authorization header requires 'Credential' parameter"` — i.e. **AWS SigV4**
     (API Gateway IAM). The app trades the `id_token` for **temporary AWS credentials
     via a Cognito Identity Pool**, then **SigV4-signs** each control request (matches
     the `X-Amz-*` headers and the Cognito references in the APK).
- **Refresh** (`…/v6/deviceauth/refreshtoken`) needs `grant_type` **and** `client_id`
  (`6ebckm2cmaijai6kfb7251ar9a`), `app_uuid`, `x-trace-transaction-id`, and a `code`
  field — a device-auth refresh, not a bare OAuth refresh.

**Durable CLI auth design:**

1. `login` (slow, one-time; assisted for the Akamai-protected My Verizon web step):
   run the 3-factor flow → capture the **offline** token set → persist the
   `refresh_token` (OS keyring / encrypted file).
2. **Refresh** the `id_token` on demand via `deviceauth/refreshtoken` (client_id +
   app_uuid + refresh_token). No OTP.
3. **Call the API** by class: `Authorization: <id_token>` for config/content;
   `id_token → Cognito GetCredentialsForIdentity → SigV4` for parental-control ops.
4. Persist tokens/creds; refresh transparently; re-run `login` only when the offline
   refresh finally expires.

## 11. The `x-signature` request signer — algorithm and the bring-your-own-key model

The device-auth endpoints (OTP send/validate, token refresh) require an
`x-signature` header. `internal/signing` reproduces the algorithm; the synthetic
vectors in `signing_test.go` pin the concatenation and primitive with a **fake** key
and independently-computed digests — no real key and no captured per-session data is
committed.

**Algorithm** — `GenerateHmacSignatureUseCase`:

```
x-signature = hex( HMAC-SHA256( key = <app signing key, supplied at runtime>,
                                msg = AppVersion + SourceApp + x-transaction-id
                                      + method + x-timestamp + x-appuuid ) )
```

It signs request **metadata**, not the body. `x-transaction-id` comes from
`com.verizon.network.TransactionId.get()`.

**The key is not shipped with this tool** — the repo carries the algorithm, not the
vendor's credential (`com.verizon.familybase.feature.identity.BuildConfig.HMAC_SIGNING_SECRET`).
Publishing interoperability code is different from committing the vendor's live shared
credential, and the repo carries neither the key nor any per-session capture (see
`CLAUDE.md` § Secrets). The operator supplies it at runtime from **their own licensed
copy of the app** — the APK is already on their machine, so no special handling ceremony
is needed. Extract the constant from the decompiled APK and hand it to the CLI:

```bash
jadx -d out <your com.verizon.familybase.parent.apk>
KEY=$(grep HMAC_SIGNING_SECRET \
  out/sources/com/verizon/familybase/feature/identity/BuildConfig.java \
  | grep -oE '[0-9a-f]{64}')
SAFE_CLI_SIGNING_KEY=$KEY safe_cli <command>   # scoped to this one invocation
# or save $KEY to a file once and set SAFE_CLI_SIGNING_KEY_FILE=<path> for reuse
```

Prefer the scoped form over `export` so the value isn't inherited by every other child
process in the shell. A planned `safe_cli auth extract-key --apk <your.apk>` will do the
extraction in one step. The CLI itself never persists, logs, or prints the key.

If the vendor rotates the key in a later app version, re-extract from the newer APK
(and update the pinned `AppVersion`).

**Footgun — the transaction id is decimal, not base-64.** `TransactionId.get()` is
`new BigInteger(130, SecureRandom).toString(64)`. Java's `BigInteger.toString(radix)`
silently falls back to **radix 10** for any radix outside 2–36, so `toString(64)`
renders a decimal string (~39–40 digits), *not* base-64. A re-implementation that
takes `64` at face value produces the wrong id and every signature fails. `internal/
signing.TransactionID` generates a 130-bit value rendered in decimal to match.
