package signing

import (
	"errors"
	"io"
	"math/big"
	"regexp"
	"testing"
	"time"
)

// These vectors are fully synthetic: a fake key and fake, non-personal inputs, with
// expected digests computed independently (Python's hmac over the documented
// concatenation). They pin the algorithm and field order without embedding the app's
// real signing key or any captured per-user/per-session data. If the concatenation
// order or primitive regresses, they stop matching.
func TestSignatureMatchesSyntheticVectors(t *testing.T) {
	const key = "synthetic-signing-key-for-tests-only"
	cases := []struct {
		name, method, txn, ts, appUUID, want string
	}{
		{
			"vec1", "POST",
			"123456789012345678901234567890123456789", "1700000000000",
			"00000000-0000-4000-8000-000000000000",
			"6d7218e34c81bf78a35f8e8aca9aeda0d6f5deed0ad74b6f638b63224085257e",
		},
		{
			"vec2", "POST",
			"999999999999999999999999999999999999999", "1699999999999",
			"11111111-2222-4333-8444-555555555555",
			"e89cb56456b77e4a5e05e168448eadca2303e55f90be9b0cfd997b4a8e56a92b",
		},
	}
	for _, c := range cases {
		if got := Signature(key, c.method, c.txn, c.ts, c.appUUID); got != c.want {
			t.Errorf("%s: Signature = %s, want %s", c.name, got, c.want)
		}
	}
}

// A different key must change the signature (the key actually participates).
func TestSignatureKeyIsUsed(t *testing.T) {
	args := []string{"POST", "1", "1700000000000", "00000000-0000-4000-8000-000000000000"}
	a := Signature("key-a", args[0], args[1], args[2], args[3])
	b := Signature("key-b", args[0], args[1], args[2], args[3])
	if a == b {
		t.Error("Signature ignored the key: different keys produced the same digest")
	}
}

func TestSignedHeadersShape(t *testing.T) {
	const key = "synthetic-signing-key-for-tests-only"
	h, err := SignedHeaders(key, "POST", "00000000-0000-4000-8000-000000000000", time.UnixMilli(1700000000000))
	if err != nil {
		t.Fatalf("SignedHeaders: %v", err)
	}
	if h["x-timestamp"] != "1700000000000" {
		t.Errorf("x-timestamp = %q", h["x-timestamp"])
	}
	if !regexp.MustCompile(`^\d{1,40}$`).MatchString(h["x-transaction-id"]) {
		t.Errorf("x-transaction-id = %q, want a decimal (<=40 digits)", h["x-transaction-id"])
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(h["x-trace-transaction-id"]) {
		t.Errorf("x-trace-transaction-id = %q, want uuid v4", h["x-trace-transaction-id"])
	}
	// The signature in the header set must be self-consistent with its own txn/ts.
	if want := Signature(key, "POST", h["x-transaction-id"], h["x-timestamp"], h["x-appuuid"]); h["x-signature"] != want {
		t.Errorf("x-signature not self-consistent")
	}
	if len(h["x-signature"]) != 64 {
		t.Errorf("x-signature len = %d, want 64 hex", len(h["x-signature"]))
	}
}

func TestTransactionIDShape(t *testing.T) {
	// Matches the app: a 130-bit random integer in decimal (<=40 digits, and < 2^130).
	re := regexp.MustCompile(`^\d{1,40}$`)
	limit := new(big.Int).Lsh(big.NewInt(1), 130)
	long := 0
	for i := 0; i < 200; i++ {
		id, err := TransactionID()
		if err != nil {
			t.Fatalf("TransactionID: %v", err)
		}
		if !re.MatchString(id) {
			t.Fatalf("TransactionID %q not a <=40-digit decimal", id)
		}
		n, ok := new(big.Int).SetString(id, 10)
		if !ok || n.Cmp(limit) >= 0 {
			t.Fatalf("TransactionID %q not in [0, 2^130)", id)
		}
		if len(id) >= 38 {
			long++
		}
	}
	// A 130-bit value is ~39-40 digits the vast majority of the time.
	if long < 150 {
		t.Errorf("only %d/200 ids were >=38 digits; generator looks too small", long)
	}
}

// errReader always fails, standing in for an exhausted/unavailable system CSPRNG.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

// A CSPRNG failure must propagate, never yield a usable header set built on
// degenerate identifiers. Without propagation, transactionID would return "0" and
// uuidV4 a fixed UUID, and signedHeaders would return a valid-looking signed request
// with reused ids and a nil error.
func TestEntropyFailurePropagates(t *testing.T) {
	var r io.Reader = errReader{}
	if _, err := transactionID(r); err == nil {
		t.Error("transactionID: want error on entropy failure, got nil")
	}
	if _, err := uuidV4(r); err == nil {
		t.Error("uuidV4: want error on entropy failure, got nil")
	}
	h, err := signedHeaders(r, "synthetic-signing-key-for-tests-only", "POST", "00000000-0000-4000-8000-000000000000", time.UnixMilli(1700000000000))
	if err == nil {
		t.Errorf("signedHeaders: want error on entropy failure, got headers %v", h)
	}
	if h != nil {
		t.Errorf("signedHeaders: want nil headers on failure, got %v", h)
	}
}
