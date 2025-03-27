// -------------------- cryptoutil/cryptoutil.go --------------------

// Certificate Authority functionality requires this
package cryptoutil

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

func GenerateRSAKey(bits int) (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, bits)
}

func VerifySignature(pub *rsa.PublicKey, data []byte, sigStr string) error {
	sig, err := base64.StdEncoding.DecodeString(sigStr)
	if err != nil {
		return err
	}
	h := sha256.Sum256(data)
	err = rsa.VerifyPKCS1v15(pub, crypto.SHA256, h[:], sig)
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}
	return nil
}
