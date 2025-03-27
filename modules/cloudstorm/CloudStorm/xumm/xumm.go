package xumm

//transaction lookup and any xumm functionality needed

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type XRPLTransaction struct {
	Hash            string `json:"hash"`
	Account         string `json:"Account"`
	Destination     string `json:"Destination"`
	TransactionType string `json:"TransactionType"`
	Validated       bool   `json:"validated"`
}

func FetchTransaction(txID string) (XRPLTransaction, error) {
	url := fmt.Sprintf("https://s1.ripple.com:51234/?tx=%s", txID)
	resp, err := http.Get(url)
	if err != nil {
		return XRPLTransaction{}, err
	}
	defer resp.Body.Close()

	var res struct {
		Result XRPLTransaction `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return XRPLTransaction{}, err
	}

	return res.Result, nil
}
