package secure

import "testing"

func TestEncryptRoundTrip(t *testing.T) {
	encoded, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	key, err := ParseKey(encoded)
	if err != nil {
		t.Fatal(err)
	}
	aad := EventAAD("cluster", "account_upsert", "entity")
	ciphertext, err := Encrypt(key, aad, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := Decrypt(key, aad, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != "secret" {
		t.Fatalf("unexpected plaintext %q", plaintext)
	}
	if _, err := Decrypt(key, []byte("wrong"), ciphertext); err == nil {
		t.Fatal("expected AAD authentication failure")
	}
}

func TestEntityIDIsStableAndNamespaced(t *testing.T) {
	key := make([]byte, KeySize)
	a := EntityID(key, "account", "same")
	b := EntityID(key, "active", "same")
	if a == b {
		t.Fatal("entity identifiers must be namespaced")
	}
	if a != EntityID(key, "account", "same") {
		t.Fatal("entity identifier must be stable")
	}
}
