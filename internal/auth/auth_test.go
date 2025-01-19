package auth

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHashing(t *testing.T) {
	plain := "hello pass"
	hashed, err := HashPassword(plain)

	if err != nil {
		t.Fatalf("Error while hashing: %v", err)
	}

	err = CheckPasswordHash(plain, hashed)

	if err != nil {
		t.Fatalf("Error while comparing: %v", err)
	}
}

func TestJWT(t *testing.T) {
	tokenSecret := "TOPSECRETKEY"

	subject, err := uuid.NewRandom()
	if err != nil {
		t.Fatalf("could not generate random uuid v4: %v\n", err)
	}

	jwtStr, err := MakeJWT(subject, tokenSecret, time.Until(time.Now().UTC().Add(5*time.Second)))

	if err != nil {
		t.Fatalf("MakeJWT returned err: %v\n", err)
	}

	uid, err := ValidateJWT(jwtStr, tokenSecret)

	if err != nil {
		t.Fatalf("ValidateJWT returned err: %v\n", err)
	}

	if uid != subject {
		t.Fatalf("expected: %v, got %v\n", subject, uid)
	}
}

func TestJWTExpires(t *testing.T) {
	tokenSecret := "TOPSECRETKEY"

	subject, err := uuid.NewRandom()
	if err != nil {
		t.Fatalf("could not generate random uuid v4: %v\n", err)
	}

	jwtStr, err := MakeJWT(subject, tokenSecret, time.Until(time.Now().UTC().Add(1*time.Second)))

	if err != nil {
		t.Fatalf("MakeJWT returned err: %v\n", err)
	}

	time.Sleep(2 * time.Second)

	uid, err := ValidateJWT(jwtStr, tokenSecret)

	if err == nil {
		t.Fatalf("ValidateJWT must have returned error, got: %v\n", uid)
	}
}

func TestJWTWrongKey(t *testing.T) {
	tokenSecret := "TOPSECRETKEY"

	subject, err := uuid.NewRandom()
	if err != nil {
		t.Fatalf("could not generate random uuid v4: %v\n", err)
	}

	jwtStr, err := MakeJWT(subject, tokenSecret, time.Until(time.Now().UTC().Add(30*time.Second)))

	if err != nil {
		t.Fatalf("MakeJWT returned err: %v\n", err)
	}

	uid, err := ValidateJWT(jwtStr, "NOTSOSECRETKEY")

	if err == nil {
		t.Fatalf("ValidateJWT must have returned error, got: %v\n", uid)
	}
}

func TestGetBearerToken(t *testing.T) {
	expected := "SD45F1E564S5F4E"
	h := http.Header{}
	h.Set("Authorization", fmt.Sprintf("Bearer %s", expected))
	r, err := GetBearerToken(h)

	if err != nil {
		t.Fatalf("GetBearerToken must have succeeded, instead got: %v", err)
	}

	if expected != r {
		t.Fatalf("It must have been expected == r but instead got: %v", r)
	}
}

func TestGetBearerTokenFail(t *testing.T) {
	h := http.Header{}
	r, err := GetBearerToken(h)

	if err == nil {
		t.Fatalf("GetBearerToken must have failed, instead got: %v", r)
	}
}
