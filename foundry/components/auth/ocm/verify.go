package ocm

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// verifyRS256 verifies an RS256 JWT signature.
// signingInput is the base64url(header).base64url(payload) string.
// sigB64 is the base64url-encoded signature from the JWT.
func verifyRS256(signingInput, sigB64 string, pub *rsa.PublicKey) error {
	sigBytes, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	digest := sha256.Sum256([]byte(signingInput))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sigBytes)
}

// signRS256 signs signingInput with the private key using RS256.
// Only used in tests to produce valid JWTs against the mock JWKS server.
func signRS256(signingInput string, priv *rsa.PrivateKey) (string, error) {
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sig), nil
}
