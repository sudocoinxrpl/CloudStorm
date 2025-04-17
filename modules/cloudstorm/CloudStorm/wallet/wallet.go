// wallet/wallet.go

// ripple wallet functionality, need to correct wallet generation procedure
package wallet

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"math/big"
	"os"
	"strings"

	ripemd160 "CloudStorm/enc"
	"CloudStorm/util"

	"github.com/btcsuite/btcd/btcec/v2"
)

func generateRandomSeed() ([]byte, error) {
	seed := make([]byte, 16)
	_, err := rand.Read(seed)
	return seed, err
}

func encodeFamilySeed(seed []byte) string {
	version := []byte{0x21}
	payload := append(version, seed...)
	h1 := sha256.Sum256(payload)
	h2 := sha256.Sum256(h1[:])
	checksum := h2[:4]
	full := append(payload, checksum...)
	return util.Base58Encode(full)
}

func derivePrivateKeyFromSeed(seed []byte) (*btcec.PrivateKey, error) {
	hash := sha512.Sum512(seed)
	d := new(big.Int).SetBytes(hash[:32])
	order := btcec.S256().N
	d.Mod(d, order)
	if d.Sign() == 0 {
		d = big.NewInt(1)
	}
	keyBytes := d.Bytes()
	if len(keyBytes) < 32 {
		padding := make([]byte, 32-len(keyBytes))
		keyBytes = append(padding, keyBytes...)
	}
	priv, _ := btcec.PrivKeyFromBytes(keyBytes)
	return priv, nil
}

func GenerateRippleWallet() (address, familySeed string, err error) {
	seed, err := generateRandomSeed()
	if err != nil {
		return "", "", err
	}
	familySeed = encodeFamilySeed(seed)
	privKey, err := derivePrivateKeyFromSeed(seed)
	if err != nil {
		return "", "", err
	}
	pubKey := privKey.PubKey()
	sha256Hash := sha256.Sum256(pubKey.SerializeCompressed())
	ripeHasher := ripemd160.New()
	ripeHasher.Write(sha256Hash[:])
	hash160 := ripeHasher.Sum(nil)
	versionedPayload := append([]byte{0x00}, hash160...)
	first := sha256.Sum256(versionedPayload)
	second := sha256.Sum256(first[:])
	checksum := second[:4]
	fullPayload := append(versionedPayload, checksum...)
	address = util.Base58Encode(fullPayload)
	return address, familySeed, nil
}

func LoadRippleWallet(filepath string) (address, familySeed string, err error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return "", "", err
	}
	familySeed = strings.TrimSpace(string(data))
	// Decoding not fully implemented; for demonstration return familySeed as address.
	return familySeed, familySeed, nil
}
