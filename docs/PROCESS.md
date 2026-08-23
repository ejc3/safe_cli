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
