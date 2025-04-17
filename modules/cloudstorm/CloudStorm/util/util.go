package util

//mixed bits that need a home

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"time"
)

var RippleAlphabet = []byte("rpshnaf39wBUDNEGHJKLM4PQRST7VWXYZ2bcdeCg65jkm8oFqi1tuvAxyz")

func Base58Encode(input []byte) string {
	zeros := 0
	for _, b := range input {
		if b == 0 {
			zeros++
		} else {
			break
		}
	}
	num := new(big.Int).SetBytes(input)
	var encoded []byte
	base := big.NewInt(58)
	mod := new(big.Int)
	for num.Cmp(big.NewInt(0)) > 0 {
		num.DivMod(num, base, mod)
		encoded = append(encoded, RippleAlphabet[mod.Int64()])
	}
	for i := 0; i < zeros; i++ {
		encoded = append(encoded, RippleAlphabet[0])
	}
	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}
	return string(encoded)
}

func ComputeBlockHash(block interface{}, extra ...string) (string, error) {
	data, err := json.Marshal(block)
	if err != nil {
		return "", err
	}
	for _, s := range extra {
		data = append(data, []byte(s)...)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func ComputeChallenge(serviceID string) string {
	h := sha256.Sum256([]byte(serviceID))
	return hex.EncodeToString(h[:])
}

func CurrentTimeUTC() time.Time {
	return time.Now().UTC()
}
