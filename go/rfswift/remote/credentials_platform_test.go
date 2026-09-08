package remote

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

// Native CI opts in; ordinary tests do not write to a user's credential vault.
func TestOSCredentialStoreRoundTrip(t *testing.T) {
	if os.Getenv("RFSWIFT_TEST_OS_KEYRING") != "1" {
		t.Skip("native credential-store integration is opt-in")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	ref := "test/remote-credentials/" + hex.EncodeToString(secret[:8])
	store := OSSecretStore{}
	if err := store.Set(ref, secret); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Delete(ref); err != nil {
			t.Errorf("remove test credential: %v", err)
		}
	})
	got, err := store.Get(ref)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatal("credential round-trip changed the secret")
	}
}
