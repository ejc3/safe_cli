# Process: how the SafePath protocol was reverse-engineered — and how to do it again

This is the end-to-end runbook behind `safe_cli`: how we established that a durable
client is viable, recovered the backend contract from the decompiled app, captured the
live traffic that static analysis can't reach, and verified the CLI behaves like the
app. It is written so the work is reproducible and auditable.

> **Scope & authorization.** Everything here is interoperability research on the account
> owner's *own* Verizon Family account and their *own* licensed copy of the app,
> comparable to the community clients for Google Family Link, Microsoft Family Safety,
> and Nintendo parental controls. No credentials, tokens, app binaries, or per-user
> values are committed to this repo.

**The short version of the method, so you don't repeat our dead ends:**
static-decompile the app to recover the contract (§2, §6); run it on a **Cuttlefish**
emulator on ARM (§4); capture its TLS **with eCapture (eBPF), not mitmproxy/frida**
(§5, and §12 for why); drive the multi-factor **login through the emulator's own
WebView** and grab the `vsfapp://` redirect from the app-chooser (§9); then **verify
parity** by byte-diffing captured bodies against `call --dry-run` and sweeping live
reads (§11).

## 0. The environment

The work runs on an AWS **Graviton (ARM64)** dev box that is itself a **Firecracker**
microVM. Properties, all verified rather than assumed:

| Property | Result | Consequence |
|---|---|---|
| `/dev/kvm` + `KVM_CREATE_VM` | works (nested KVM) | can run a nested Android VM (Cuttlefish) |
| `binder` kernel module | **absent** (`7.0.14-fcvm` kernel) | Waydroid impossible |
| egress | open (incl. `dl.google.com`, `ci.android.com`, GitHub, PyPI) | can fetch SDKs/images/tools |
| CPU / RAM | 64 cores / 125 GB | comfortable for an emulator + a Bazel build |

The Android guest is **Cuttlefish, API 37 / arm64-v8a**, reachable at
`adb 127.0.0.1:6520`. It is a **userdebug** image, so `adb root` works — needed for
`am dumpheap`, reading app data, and running eCapture as root. Crucially, we build the
guest kernel, so it carries the eBPF features eCapture requires (§5).

## 1. The deciding question

Is the SafePath backend replayable by a script, or does it enforce **device
attestation / per-request signing** that a scripted client can't forge? Everything
hinges on this. Answer: **VIABLE** — see `docs/FINDINGS.md` for the verdict and
evidence.

## 2. Static analysis (answered most of it)

1. **Get the signed APK.** The dev box can't always pull APK mirrors directly and there
   was no phone to `adb pull` from. The signed APK was fetched through the owner's own
   **Browserbase** cloud browser (apkcombo's download link is IP-locked to the browser
   session). Result: the 334 MB XAPK for `com.verizon.familybase.parent` **v8.101.30
   (build 810100030)**. The APK is **not** committed.
2. **Explode it.** `unzip` the XAPK → `base.apk` (+ arch/dpi split APKs). `base.apk`
   holds 28 `classes*.dex` (~1.6M string constants).
3. **Scan the dex string table** for the deciding signatures (attestation, request
   signing, cert pinning, auth shape) and for API hosts. The raw pass lived in a shell
   script; its one reusable piece — a pure-Go dex string/field parser — survives as
   `internal/apkkey`, used by `auth extract-key`. (A standalone `analyze` subcommand was
   once planned but **dropped**: the analysis is complete and encoded in the descriptor.)
4. **Read the verdict.** The ~80 "attestation" hits were mostly the broad `attest`
   substring. The real Play Integrity references all belong to the **Nok Nok FIDO/UAF**
   SDK (`com.noknok…`, `com.fido.uaf…`) — the *optional* biometric login path, not a
   per-request gate. No Firebase App Check; no `X-Integrity`/`X-AppCheck` header. Auth is
   a plain bearer-style **id_token**. → a durable scripted client is viable.
5. **Recover the contract.** Base URL `https://api.prd.vsf.aws.vz-connect.com`
   (AWS-fronted, platform codename **frisco**); an initial **183** path constants
   harvested from Retrofit interfaces (`docs/discovered-endpoints.txt` — the *raw* early
   count, not the final surface); the OAuth authorize/token/refresh/otp routes;
   `client_id`, redirect scheme, PKCE. The curated command surface is later pinned at
   **459 operations / 59 entities** in the descriptor (§10).

## 3. Why a dynamic capture is still needed

Static reading recovers *most* of the contract, but two things need the live app:

- **The exact login handshake.** Replaying the OAuth `authorize` route cold returns
  `400 Invalid Request`; the real login is a signed **device-OTP** call
  (`POST /auth/frisco/frisco-iam-device-auth/v7/user/auth/otp`) whose `x-signature` and
  identity headers are minted at runtime — not shippable guesses. You have to watch the
  app perform it (§8, §9).
- **Confirming request/response bodies.** The mutation body *shapes* come from the
  decompiled model classes, but a subset needs live confirmation — byte-matching against
  real traffic and checking the backend accepts them (§10, §11).

So dynamic capture here is **confirmation + login-recovery**, not primary discovery.

## 4. Android on ARM: Cuttlefish

Doorways to Android-on-ARM, evaluated on this host:

- **AVD emulator** — Google ships no `linux-aarch64` emulator host build. Not usable.
- **Waydroid** — needs the host kernel's `binder`; the Firecracker kernel has none.
- **Cuttlefish** — Google's KVM-based Android-for-servers. Viable on ARM with KVM;
  **this is the path.** Built from source (`github.com/google/android-cuttlefish`;
  `base` is a Bazel C++ build, `frontend` is Go), plus a `cvd-host_package` and an
  `aosp_cf_arm64` image, launched headless.

The capture method on top of this is **eCapture (eBPF)** — see §5 — which is why the
guest kernel must carry `CONFIG_BPF_SYSCALL` + `CONFIG_DEBUG_INFO_BTF` + `CONFIG_UPROBES`
(building the guest ourselves is what makes that guaranteed). We do **not** use
mitmproxy or frida; §12 records why that whole class of approach is a dead end here.

> **GMS caveat.** AOSP Cuttlefish images ship without Google Play Services. In practice
> the app runs to its sign-in screen and through the whole login **without** GMS (only a
> Firebase-analytics job warns), so this did not block the work.

### 4.1 Building the Cuttlefish toolchain (clean, reproducible)

No prebuilt Cuttlefish debs exist for arm64, so build from source. This exact order is
distilled from a build that hit every trap below.

**1. Install Bazel *first*.** The `base` package is a Bazel C++ build; without Bazel on
`PATH` its `debian/rules` aborts with *"Bazel install is broken"*.

```bash
sudo apt-get update
sudo apt-get install -y git devscripts equivs config-package-dev debhelper-compat golang curl
curl -sL -o /tmp/bazelisk https://github.com/bazelbuild/bazelisk/releases/latest/download/bazelisk-linux-arm64
sudo install /tmp/bazelisk /usr/local/bin/bazel
```

**2. Build the debs — as your user, one at a time** (never under `sudo`):

```bash
git clone https://github.com/google/android-cuttlefish && cd android-cuttlefish
git checkout 786f4ac2be42519fd4b023a36114dd6ae7ffc04b   # the revision this was validated on (v1.57.0-dev)
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
# group. Scripted equivalent: exec su -l "$USER"
```

**4. Fetch an eBPF-capable image + launch headless.** The guest kernel must have
`CONFIG_BPF_SYSCALL` / `CONFIG_DEBUG_INFO_BTF` / `CONFIG_UPROBES` for eCapture (§5); a
stock `cvd fetch` image may lack BTF, so use/build a kernel that has them and verify
in-guest before relying on capture:

```bash
cvd fetch --default_build=aosp-main/aosp_cf_arm64_only_phone-userdebug
HOME="$PWD" launch_cvd --daemon --report_anonymous_usage_stats=n
adb connect 127.0.0.1:6520 && adb wait-for-device
adb shell 'zcat /proc/config.gz | grep -E "BTF|UPROBES|BPF_SYSCALL"'   # must all be =y
```

**Build traps this order avoids** (each cost a full rebuild):

- **Never build under `sudo`** — leaves root-owned files in `debian/<pkg>/…`; the next
  `dh_clean` dies with `Permission denied`. Recovery: `sudo chown -R "$USER" base/debian`.
- **Exactly one build at a time** — concurrent `dpkg-buildpackage` runs fight over the
  single Bazel server and shared `../*.deb` output.
- **`debian/rules clean` wipes Bazel's cache**, so a re-run recompiles from scratch
  (~15–20 min); the `base` link uses `-flto=auto`, so the final single-threaded `ld.lld`
  is slow, not hung.
- **Don't `pkill -f bazel`** from a shell whose command line contains `bazel` — `pkill
  -f` matches your own shell and kills it. Kill by PID or `bazel shutdown`.

### 4.2 The reset cycle (important)

`cvd reset` **wipes guest userdata**. After a reset (or a fresh `cvd create`) you must
rebuild the guest state before capturing again:

```bash
adb install-multiple base.apk config.arm64_v8a.apk config.hdpi.apk   # the split APK
adb push ecapture /data/local/tmp/ && adb shell chmod 755 /data/local/tmp/ecapture
# then re-run the login (§9) — tokens do not survive the wipe.
```

Also clear any dead proxy state before launching the app — a leftover
`global_http_proxy_host=127.0.0.1:8080` pointing at a stopped proxy makes every request
fail (`Failed to connect to /127.0.0.1:8080`) and looks like "stale data":
`adb shell settings put global http_proxy :0`.

## 5. Capturing traffic — eBPF (eCapture) is the method that works

The clean way to read this app's TLS is **eCapture (`gojue/ecapture`)** — eBPF uprobes
on conscrypt's `libssl.so` `SSL_write`/`SSL_read`. It reads plaintext **after** TLS
decrypt and after any request signing, with **no** injected `.so`, **no** ptrace, and
**no** CA install — so it is blind to the app's anti-frida watchdog and immune to its
certificate pinning (§12 explains why the usual rigs fail here).

```bash
# verify the guest kernel first (see §4.1), then:
adb push ecapture /data/local/tmp/ ; adb shell chmod 755 /data/local/tmp/ecapture
adb shell su -c '/data/local/tmp/ecapture tls -m text \
  --libssl=/apex/com.android.conscrypt/lib64/libssl.so \
  --ssl_version="boringssl 1.1.1"' > ecap.out
# then drive the app; the plaintext streams into ecap.out.
```

**Text-mode limitation that shapes verification.** The backend speaks **HTTP/2**. In
text mode eCapture gives you the raw `SSL_write`/`SSL_read` buffers, so:

- **DATA frames (request/response JSON bodies) come out as plaintext** — greppable, and
  the decisive artifact for confirming and diffing mutation bodies.
- **HEADERS frames are HPACK-compressed** and **not** byte-decodable from a mid-attach
  capture: HPACK is stateful (a per-connection dynamic table built up over the whole
  connection), and attaching after the long-lived connection is established means you
  never saw the table get seeded. So request **bodies are diffable, but the app's exact
  wire headers are not** — which is why header parity is proven from the decompiled
  interceptor contract + live server acceptance (§11), not a raw byte-diff.

## 6. The parental-control API model — plain id_token, **not** SigV4/Cognito

An early inference (and a `call` that drew `403 "Authorization header requires
'Credential' parameter"`) suggested the parental-control endpoints needed **AWS SigV4**
with Cognito temp credentials. **That was wrong** — a red herring that cost real time.
Three independent proofs settle it:

1. **Memory forensics.** With the app logged in as the parent and running, a full scan
   of its native + Java memory (`setenforce 0`; `/proc/<pid>/mem` across all `rw`
   regions; `am dumpheap`) found **zero** `cognito-identity`, no identity-pool id, and no
   `ASIA…`/`AKIA…` credentials. The app never mints AWS creds for this API. The bundled
   `com.amazonaws.services.cognitoidentity.*` SDK is dead code; the only real AWS use is
   **Driving Insights** (cmtelematics S3, session creds handed in) and video-calling.
2. **The decompiled interceptor.**
   `com/verizon/data/network/interceptor/TokenAwareInterceptor` sets
   `Authorization: <token>` (raw JWT, **no `Bearer`**) via
   `getOnlineToken()`/`getOfflineToken()`, and applies the `x-signature` HMAC **only**
   when the per-call `RetrofitTagParams` has `hmacRequired=true` — confined to the
   device-auth OTP/token endpoints. Every parental-control call is
   `RetrofitTagParams(ONLINE, hmac=false)`: online id_token, no HMAC, no SigV4.
3. **The 403 explained.** `ApiConstants.PARENTAL_CONTROL_ICON_URL =
   /vsf/parental-control/v5/icon` is an S3/IAM-fronted **icon asset**; hitting it (or a
   real control path while omitting the `x-fp-identifier-*` headers) draws the
   API-Gateway "Credential parameter" 403 that started the chase.

**The real contract.** Control calls send `Authorization: <online id_token>` (raw JWT,
no `Bearer` prefix) plus `x-fp-identifier-*` request-identity headers. The
near-universal one is `x-fp-identifier-target-serviceid` = the **target (child's)
service id** — the parent acts on a *child*, not their own service (the parent's own
serviceid returns `403 "no permissions on this serviceId"`). `safe_cli members`
enumerates the family and prints each member's service/profile/device ids and role
(GUARDIAN vs DEPENDENT) so you know what to pass as `--service-id`.

## 7. The `x-signature` request signer — algorithm and bring-your-own-key

The device-auth endpoints — **OTP send and OTP validate only** — require an
`x-signature` header. (The token **refresh/exchange** to `/v7/user/auth/token` is
**unsigned**; do not add `x-signature` to it.) `internal/signing` reproduces the
algorithm; `signing_test.go` pins the concatenation with a **fake** key and
independently-computed digests — no real key, no captured per-session data committed.

**Algorithm** — `GenerateHmacSignatureUseCase`:

```
x-signature = hex( HMAC-SHA256( key = <app signing key, supplied at runtime>,
                                msg = AppVersion + SourceApp + x-transaction-id
                                      + method + x-timestamp + x-appuuid ) )
```

It signs request **metadata**, not the body. `x-transaction-id` comes from
`com.verizon.network.TransactionId.get()`.

**The key is not shipped with this tool** — the repo carries the algorithm, not the
vendor's credential
(`com.verizon.familybase.feature.identity.BuildConfig.HMAC_SIGNING_SECRET`). The
operator supplies it at runtime from **their own licensed copy of the app**:

```bash
export SAFE_CLI_SIGNING_KEY=$(safe_cli auth extract-key --apk <your.apk>)   # 64-hex, never touches a shell arg
# or, manually:
jadx -d out <your.apk>
grep HMAC_SIGNING_SECRET out/sources/com/verizon/familybase/feature/identity/BuildConfig.java | grep -oE '[0-9a-f]{64}'
```

`auth extract-key` is the one command whose job is to hand you the key; otherwise the
CLI never writes it to disk or logs it. If the vendor rotates the key in a later app
version, re-extract from the newer APK (and update the pinned `AppVersion`).

**Footgun — the transaction id is decimal, not base-64.** `TransactionId.get()` is
`new BigInteger(130, SecureRandom).toString(64)`. Java's `BigInteger.toString(radix)`
silently falls back to **radix 10** for any radix outside 2–36, so `toString(64)`
renders a ~39–40 digit **decimal** string, not base-64. `internal/signing.TransactionID`
matches this.

## 8. The full authentication flow (captured end-to-end)

Login is a **multi-factor, multi-service chain**, not one API call. Captured with
eCapture (the hosted-login WebView is in-process, so its TLS is decrypted too):

1. **SafePath device OTP** (line verification), signed frisco API —
   `POST /auth/frisco/frisco-iam-device-auth/v7/user/auth/otp` body `{"mdn":"<phone>"}`
   → `200 {"state":"OTP_SENT"}` (SMS #1 to the line).
2. **Validate device OTP** —
   `POST /auth/frisco/frisco-iam-device-auth/v7/user/auth/otp/validate`
   body `{"mdn":"<phone>","otp":"<code>"}` → `207 {"state":"AM_LOGIN_PAGE","tokens":
   [{"token_type":"login_recom_token","id_token":"…","expires_in":1800}]}`. The
   **loginRecommendation** JWT fuses factor-1 into the exchange.
3. **Hosted Verizon Account-Management login** —
   `GET /frisco/frisco-iam-device-auth/v5/oauth2/authorize` → `302` → the My Verizon
   hosted login at `secure.verizon.com/signin/oauth2/vendor/authorize`.
4. **My Verizon account login** (User ID / mobile + password), Akamai-protected → an
   authenticate call.
5. **My Verizon account 2FA** — a *second* SMS, separate from step 1; entering it yields
   an OAuth authorization `code` delivered to the app's custom scheme
   `vsfapp://com.verizon.familybase.parent/signin?code=…`.
6. **Token exchange** — `POST /auth/frisco/frisco-iam-device-auth/v7/user/auth/token`
   with the `code`, its PKCE `codeVerifier`, **and** the recom `token` mints the real
   `id_token`/`access_token` + a durable **offline** `refresh_token`. Thereafter the
   parental-control API accepts `Authorization: <online id_token>` (raw, no `Bearer`).

**Mixed path prefixes:** `/auth/frisco/…` for the OTP/token steps, `/frisco/…` for the
OAuth authorize. The device-auth calls are signed (§7); the token exchange/refresh is
not.

## 9. Driving the login — the reproducible recipe

Steps 3–5 are a bot-protected hosted web login (Akamai on `secure.verizon.com`, **two**
SMS factors). A datacenter's plain HTTP automation is fingerprinted and blocked, and
adb-`input`-driving the login page directly trips an Akamai *"start over"* reset. The
method that works is to let a **real browser** do the hosted leg while the CLI owns the
signed frisco legs. We used the **emulator's own browser** — no external service needed.

**A. Device-OTP leg (CLI owns it).** `auth login` sends the signed OTP call and prompts
for the code. Two gotchas the CLI now handles:

- The backend answers **2xx even on failure**, distinguishing by a `state` field —
  only `state=="OTP_SENT"` actually sent a code (`OTP_DISPATCH_FAILED`,
  `OTP_ATTEMPTS_EXCEEDED`, `OTP_NOT_FOUND`, … still return 2xx). `RequestDeviceOTP`
  verifies the state and errors otherwise, instead of telling you to wait for a code
  that never comes.
- **Each `auth login` sends a NEW OTP that supersedes the last**, so on a retry don't
  restart — reuse the code already delivered: `auth login --otp <code>` skips the
  request and validates that code directly.

**B. Hosted login via the emulator WebView.** Run `auth login --no-browser --paste`
(driven over a FIFO so its two stdin reads — the OTP, then the pasted redirect — can span
separate steps). It prints the authorize URL; load that in the emulator's browser:

```bash
adb shell "am start -a android.intent.action.VIEW -d '<authorize-url>'"   # org.chromium.webview_shell
```

`secure.verizon.com` loads (Akamai passes — the emulator's in-process Chromium presents
a real browser fingerprint even on the datacenter host). Drive User ID / password / 2FA
with `adb shell input` + `uiautomator dump` to find fields.

**C. Capture the redirect from the app-chooser.** On success the flow redirects to
`vsfapp://com.verizon.familybase.parent/signin?code=…`. Because the app registers that
scheme, Android raises the **app-chooser** (`ResolverActivity`) rather than showing the
URL. Grab it from the intent and **press BACK so the app doesn't consume the one-time
code** before you exchange it:

```bash
adb shell dumpsys activity activities | grep -oE 'vsfapp://[^ }]*code=[^ }]*'
adb shell input keyevent KEYCODE_BACK
```

Paste that full URL into the waiting CLI; it exchanges the `code` (+ PKCE verifier + the
recom token) for the durable token set. `auth refresh` renews the id_token thereafter
with no browser/OTP, and every `call` auto-refreshes on a 401.

**Alternative (CI): Browserbase.** Instead of the emulator, drive a
[Browserbase](https://browserbase.com) cloud browser over CDP (Playwright
`connect_over_cdp`): navigate the authorize URL, fill credentials, submit the 2FA, and
intercept the `vsfapp://` navigation. Footguns: create the session with
`proxies:true, keepAlive:true, timeout:1800` (the default 5-min timeout cuts you off
mid-2FA); a fresh cookieless session avoids a **stale 2FA page that never sends a code**;
auth `code`s expire in ~60 s, so exchange immediately.

### 9a. Heap token recovery (one-shot, no login)

If the app is already logged in on the emulator, you can lift its live tokens instead of
re-running the login — useful for a quick one-shot. With root, the decrypted id_tokens
sit in the app heap:

```bash
PID=$(adb shell pidof com.verizon.familybase.parent)
adb shell "am dumpheap $PID /data/local/tmp/app.hprof"
adb shell "strings /data/local/tmp/app.hprof | grep -oE 'eyJraWQ[A-Za-z0-9_-]+\\.eyJ[A-Za-z0-9_-]+\\.[A-Za-z0-9_-]+' | sort -u"
# → the online id_token (iss .../frisco-iam-device-auth) + offline (iss .../frisco-iam-auth)
```

Build a TokenSet JSON (`{"mdn":…,"tokens":[{"id_token":…,"frisco_token_type":"online"},
{"id_token":…,"frisco_token_type":"offline"}]}`) and `safe_cli auth import <file>`.
**Caveat: this is one-shot.** The online id_token is short-lived (~30 min) and the heap
holds **no** `refresh_token` — the refresh tokens live encrypted at rest in
EncryptedSharedPreferences under keys `REFRESH_TOKEN` / `OFFLINE_REFRESH_TOKEN`, so they
never appear in the heap plaintext. Durable auth still needs `auth login` (§9), which is
what mints a refresh_token.

## 10. From contract to CLI surface — the descriptor, `call`, and mutation bodies

The recovered contract becomes `internal/descriptor/verizon_family.json` — **459
operations / 59 entities**, the single source of truth from which the command surface and
data model are generated. (The raw Retrofit harvest in `docs/api-catalog.md` /
`docs/vsf-endpoints.json` is the larger superset — **489 methods across 110 interfaces**
— which de-dupes to the 459/59 curated surface.)

The CLI is **not** a set of `print/info/create/update/delete` verbs (those were never
built). It is a single generic engine, `call <entity> <op> [id]`, that fills each op's
method/path/headers/body from the descriptor plus `--service-id`/`--data`/`--query`/
`--header`/`--path` and the id_token claims. `--dry-run` prints the exact request
without sending it. Around it sit `entities`, `describe`, `members`, `raw`, and
`auth {login,refresh,import,status,logout,extract-key}`. Because the descriptor is the
source of truth, the same engine could serve other SafePath carriers via different
descriptors.

**The mutation-body pipeline.** Each op's request-body *shape* was extracted from the
decompiled **jadx model classes** via a fan-out workflow (one agent per model → a schema
keyed by the op), then **confirmed live**: byte-matched against eCapture-captured app
traffic and checked accepted by prod. Worked examples that flipped `confirmed:false →
true`:

- `app_block.blockApp` — `POST /parental-control/frisco/v8/subcategory`, body
  `{"subcategory":{name,id,enabled,categoryId,categoryShortName}}` (enabled=true blocks).
- `schedules.putSchedule` — `PUT /parental-control/frisco/v6/schedules`, body a
  `schedules[]` array of `{scheduleId,scheduleType,name,days[],startTime,endTime,
  timezone,alertOn,blockContent}` (`scheduleId` targets the existing schedule; the
  earlier inferred `{"id":…}` stub was incomplete — the capture corrected it).

## 11. Parity verification — proving the CLI matches the app

Three complementary checks establish that the CLI's requests are what the app sends:

1. **Bodies, byte-for-byte.** Capture the app performing a mutation (§5) and set the
   plaintext DATA-frame body beside `call <op> --dry-run` for the same op. Confirmed
   identical for the mutations above.
2. **Live server acceptance.** Run the readable surface against production: a sweep of
   **74 GET ops** returned 35 × `2xx`, and the rest were the server's own
   `403`/`404`/`401` for features this account isn't entitled to — **0 malformed
   requests**. A live `putSchedule` write returned `200` and a read-back confirmed it.
   This is the backend certifying the CLI's requests, headers included.
3. **Header contract.** Since HPACK blocks a raw header byte-diff (§5), header parity
   rests on the decompiled `TokenAwareInterceptor` contract plus (2) — a wrong/missing
   identity header is exactly what produces a 403, and the non-entitlement calls went
   through.

**Note:** `call --dry-run` renders the request as **HTTP/1.1** only because Go's
`httputil.DumpRequestOut` serializes that way; the CLI's actual wire is **HTTP/2** (Go's
default transport negotiates h2 via ALPN), matching the app. So both sides HPACK-compress
headers on the wire — another reason the meaningful comparison is the decompressed
contract + server acceptance, not wire bytes (which two correct h2 clients need not match
even for identical logical headers).

## 12. Dead ends — why NOT mitmproxy / frida

We spent real time on the mitm+frida route before eCapture. Record it so nobody repeats
it: this class of app defeats mitm/unpin three ways at once, and the environment adds a
fourth.

- **Anti-frida:** a native watchdog in `libSummitSdk.so` **self-terminates the app ~15 s
  after any frida attach** — so frida hooks and frida-based TLS unpinning both die.
- **Code-level pinning:** TLS pinning is an OkHttp `CertificatePinner` in *code*, not in
  `network_security_config.xml` (no `<pin-set>`), so a trusted system CA alone can't
  decrypt the frisco host.
- **API-37 cert isolation:** a `tmpfs` bind-mount of a mitm CA over
  `/apex/com.android.conscrypt/cacerts` **does not propagate into app mount namespaces**
  even after restarting zygote — without Magisk/`nsenter` the app never trusts the CA.
- **Bot protection on the login** trips a "start over" reset under robotic `adb input`.

eCapture (§5) sidesteps all of it: eBPF uprobes read plaintext after decrypt, with no
injection (anti-frida blind), no CA (pinning/APEX irrelevant). Do **not** re-enter the
frida / Magisk-Zygisk / conscrypt-CA-bind-mount / `nsenter` fight for this app.

## 13. Footguns worth keeping

- **`adb shell input text` drops characters** when typing fast, and mangles segmented
  OTP fields. Type digit-by-digit with short sleeps and verify the field before
  submitting anything that costs an OTP. Spaces break `input text` entirely (only the
  first token lands); special chars in a password work via Android-side single-quoting
  (synthetic example): `adb shell "input text 'Ex4mple#p&ss^\$'"`.
- **zsh does not word-split unquoted variables.** `A="adb -s 127.0.0.1:6520"; $A shell …`
  runs the whole string as one command name. Put only the binary in the var.
- **`pkill -f <word>` also matches your own shell** when `<word>` is in its command line
  → it kills the shell (exit 143/144). Kill by PID, or use a graceful stop.
- **`cvd reset` wipes userdata** — see §4.2; you must reinstall the APK, re-push
  eCapture, and re-login afterward.
- **Cuttlefish external storage resolves to `/dev/null/Android/data/…`** in some images,
  so the app spams `SecurityException: Invalid mkdirs path`; ensure `/sdcard` is mounted /
  `EXTERNAL_STORAGE` is set if the app misbehaves mid-flow.

## 14. Reproduce (abbreviated)

```bash
# Static
unzip verizon-family.xapk -d bundle
unzip "bundle/com.verizon.familybase.parent.apk" -d base       # base holds the dex; config.* are splits
strings -n 5 base/classes*.dex | grep -EiC0 'IntegrityManager|PlayIntegrity|/oauth|/token|CertificatePinner'
grep -hoE '(vsf|frisco)/[A-Za-z0-9._/{}-]*v[0-9]+/[A-Za-z0-9._/{}-]*' <(strings base/classes*.dex) | sort -u

# Dynamic (Cuttlefish, ARM): build the toolchain (§4.1) with an eBPF-capable kernel, then
#   adb install-multiple base + config.arm64_v8a + config.hdpi        # (§4.2)
#   adb push ecapture /data/local/tmp/ ; ecapture tls -m text --libssl=.../libssl.so ...   # capture (§5)
#   auth login --no-browser --paste  + emulator WebView + app-chooser redirect grab        # login (§9)
#   diff captured bodies vs `call --dry-run`; sweep live reads                              # parity (§11)
```
