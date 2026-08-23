// Package signing computes the frisco `x-signature` request signature.
//
// The SafePath device-auth endpoints (OTP send/validate, token refresh) require an
// HMAC signature over request metadata. This package implements only the algorithm;
// it deliberately embeds NO signing key.
//
// The key is an app-embedded build constant that ships inside the vendor's Android
// app. It is not distributed with this tool: the caller supplies it at runtime,
// sourced from the operator's own licensed copy of the app (see docs/PROCESS.md §11).
// This keeps the published repository to interoperability code — the algorithm — and
// out of redistributing the vendor's shared credential. Callers must never persist,
// log, or print the key.
package signing

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"time"
)

// AppVersion and SourceApp are part of the signed string and are also sent as
// request headers. They are app-wide build metadata (not per-user), so they are safe
// to embed.
const (
	AppVersion = "8.101.30"
	SourceApp  = "AndroidMAPP"
)

// Signature is the hex HMAC-SHA256 over the concatenation
//
//	AppVersion + SourceApp + transactionID + method + timestampMillis + appUUID
//
// keyed by the caller-supplied app signing key. This exactly matches the app's
// GenerateHmacSignatureUseCase. The key is never embedded here; see the package doc.
func Signature(key, method, transactionID, timestampMillis, appUUID string) string {
	data := AppVersion + SourceApp + transactionID + method + timestampMillis + appUUID
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(data))
	return hex.EncodeToString(m.Sum(nil))
}

// SignedHeaders returns the full x-* header set for a signed frisco request:
// x-transaction-id, x-trace-transaction-id, x-timestamp, x-appuuid, x-signature,
// x-source-app, x-mobile-app-version. The signing key is supplied by the caller.
//
// It returns an error if the system CSPRNG fails. A signed request whose
// x-transaction-id / x-trace-transaction-id came from a failed draw would carry
// degenerate, reused identifiers; rather than hand back an apparently-usable header
// set, the entropy failure is propagated so the caller aborts instead of sending it.
func SignedHeaders(key, method, appUUID string, now time.Time) (map[string]string, error) {
	return signedHeaders(rand.Reader, key, method, appUUID, now)
}

func signedHeaders(r io.Reader, key, method, appUUID string, now time.Time) (map[string]string, error) {
	txn, err := transactionID(r)
	if err != nil {
		return nil, fmt.Errorf("x-transaction-id: %w", err)
	}
	trace, err := uuidV4(r)
	if err != nil {
		return nil, fmt.Errorf("x-trace-transaction-id: %w", err)
	}
	ts := strconv.FormatInt(now.UnixMilli(), 10)
	return map[string]string{
		"x-transaction-id":       txn,
		"x-trace-transaction-id": trace,
		"x-timestamp":            ts,
		"x-appuuid":              appUUID,
		"x-signature":            Signature(key, method, txn, ts, appUUID),
		"x-source-app":           SourceApp,
		"x-mobile-app-version":   AppVersion,
	}, nil
}

// TransactionID reproduces the app's x-transaction-id: a 130-bit cryptographically
// random integer rendered in decimal (up to 40 digits). The app
// (com.verizon.network.TransactionId) computes `new BigInteger(130, SecureRandom)
// .toString(64)` — but Java's BigInteger.toString falls back to radix 10 for any
// radix outside 2-36, so the wire value is decimal, not base-64 (docs/PROCESS.md §11).
// It returns an error if the system CSPRNG fails.
func TransactionID() (string, error) {
	return transactionID(rand.Reader)
}

func transactionID(r io.Reader) (string, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 130) // 2^130
	n, err := rand.Int(r, limit)
	if err != nil {
		return "", err
	}
	return n.String(), nil
}

// uuidV4 returns a random RFC-4122 v4 UUID for x-trace-transaction-id, or an error
// if the random source is exhausted.
func uuidV4(r io.Reader) (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
