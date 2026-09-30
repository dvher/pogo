package secure

import "testing"

func TestSealOpen(t *testing.T) {
	c, _ := NewCipher(randomKey())
	sealed := c.Seal([]byte("- [ ] buy milk"))
	got, err := c.Open(sealed)
	if err != nil || string(got) != "- [ ] buy milk" {
		t.Fatalf("roundtrip: %q %v", got, err)
	}
	other, _ := NewCipher(randomKey())
	if _, err := other.Open(sealed); err != ErrDecrypt {
		t.Fatalf("wrong key: %v", err)
	}
}

func TestE2ECheck(t *testing.T) {
	salt := NewSalt()
	c, _ := NewCipher(DeriveE2EKey("correct horse", salt))
	check := c.MakeCheck()
	same, _ := NewCipher(DeriveE2EKey("correct horse", salt))
	if !same.VerifyCheck(check) {
		t.Fatal("same passphrase rejected")
	}
	wrong, _ := NewCipher(DeriveE2EKey("battery staple", salt))
	if wrong.VerifyCheck(check) {
		t.Fatal("wrong passphrase accepted")
	}
}
