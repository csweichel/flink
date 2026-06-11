package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const passwordHashIterations = 60000

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashPassword(password string) (string, error) {
	var salt [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", err
	}
	sum := derivePasswordHash(password, salt[:], passwordHashIterations)
	return fmt.Sprintf("v1$%d$%s$%s", passwordHashIterations, hex.EncodeToString(salt[:]), hex.EncodeToString(sum)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "v1" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := derivePasswordHash(password, salt, iterations)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func derivePasswordHash(password string, salt []byte, iterations int) []byte {
	h := sha256.New()
	_, _ = h.Write(salt)
	_, _ = h.Write([]byte(password))
	sum := h.Sum(nil)
	for i := 1; i < iterations; i++ {
		h.Reset()
		_, _ = h.Write(sum)
		_, _ = h.Write(salt)
		_, _ = h.Write([]byte(password))
		sum = h.Sum(nil)
	}
	return sum
}
