package auth

import "testing"

func TestHashing(t *testing.T) {
	plain := "hello pass"
	hashed, err := HashPassword(plain)

	if err != nil {
		t.Errorf("Error while hashing: %v", err)
	}

	err = CheckPasswordHash(plain, hashed)

	if err != nil {
		t.Errorf("Error while comparing: %v", err)
	}
}
