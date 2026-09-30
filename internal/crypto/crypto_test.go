package crypto

import (
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
)

func TestEncryptDecrypt_roundTrip(t *testing.T) {
	key := DeriveKey("test-secret")
	faker := gofakeit.New(7)
	for i := 0; i < 20; i++ {
		secret := faker.UUID() + ":" + faker.LetterN(24)
		sealed, err := Encrypt(key, secret)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(sealed, secret) {
			t.Fatal("ciphertext leaks the plaintext")
		}
		opened, err := Decrypt(key, sealed)
		if err != nil {
			t.Fatal(err)
		}
		if opened != secret {
			t.Fatalf("round trip = %q, want %q", opened, secret)
		}
	}
}

func TestEncrypt_nonceIsRandom(t *testing.T) {
	key := DeriveKey("test-secret")
	first, err := Encrypt(key, "same-input")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encrypt(key, "same-input")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("encrypting the same value twice produced identical ciphertext")
	}
}

func TestDecrypt_wrongKeyFails(t *testing.T) {
	sealed, err := Encrypt(DeriveKey("secret-a"), "application-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(DeriveKey("secret-b"), sealed); err == nil {
		t.Fatal("decrypt with the wrong key must fail")
	}
}

func TestDecrypt_tamperedPayloadFails(t *testing.T) {
	key := DeriveKey("test-secret")
	sealed, err := Encrypt(key, "application-key")
	if err != nil {
		t.Fatal(err)
	}
	flipped := sealed[:len(sealed)-2] + "AA"
	if _, err := Decrypt(key, flipped); err == nil {
		t.Fatal("tampered payload must fail to open")
	}
}

func TestDeriveKey_isStable(t *testing.T) {
	if string(DeriveKey("abc")) != string(DeriveKey("abc")) {
		t.Fatal("DeriveKey is not deterministic")
	}
	if string(DeriveKey("abc")) == string(DeriveKey("abd")) {
		t.Fatal("different secrets produced the same key")
	}
	if len(DeriveKey("abc")) != 32 {
		t.Fatalf("key length = %d, want 32", len(DeriveKey("abc")))
	}
}
