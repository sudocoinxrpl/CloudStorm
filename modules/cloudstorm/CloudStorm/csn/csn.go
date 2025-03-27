// -------------------- csn/csn.go --------------------

// name resolution module, resolves human readable addresses to serviceID/multiaddress
package csn

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

type ReservationSettings struct {
	CSNAddress string    `json:"csn_address"`
	Owner      string    `json:"owner"`
	Expiration time.Time `json:"expiration"`
}

type Reservation struct {
	ID                string              `json:"id"`
	Settings          ReservationSettings `json:"settings"`
	XRPRequirementMet bool                `json:"xrp_requirement_met"`
	CloudStormCosts   bool                `json:"cloudstorm_costs_covered"`
	ReservedAt        time.Time           `json:"reserved_at"`
	Expiry            time.Time           `json:"expiry"`
	NFTGenerated      bool                `json:"nft_generated"`
	NFTMarked         bool                `json:"nft_marked"`
	LedgerCID         string              `json:"ledger_cid"`
	CostPer24Hours    float64             `json:"cost_per_24_hours"`
}

var ReservationCostPer24Hours float64 = 5.0

func generateReservationID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func ReserveCSNAddress(settings ReservationSettings) (*Reservation, error) {
	id, err := generateReservationID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	res := &Reservation{
		ID:                id,
		Settings:          settings,
		XRPRequirementMet: false,
		CloudStormCosts:   false,
		ReservedAt:        now,
		Expiry:            settings.Expiration,
		NFTGenerated:      false,
		NFTMarked:         false,
		LedgerCID:         "",
		CostPer24Hours:    ReservationCostPer24Hours,
	}
	cid, err := CommitReservationToLedger(res)
	if err != nil {
		return nil, err
	}
	res.LedgerCID = cid
	return res, nil
}

func CommitReservationToLedger(res *Reservation) (string, error) {
	return "dummy-ledger-cid-" + res.ID, nil
}

func CheckXRPRequirement() bool {
	return false
}

func CheckCloudStormCosts() bool {
	return false
}

func TryGenerateNFT(res *Reservation) error {
	if res.NFTGenerated {
		return errors.New("NFT already generated")
	}
	if !res.XRPRequirementMet || !res.CloudStormCosts {
		return errors.New("conditions not met for NFT generation")
	}
	res.NFTGenerated = true
	res.NFTMarked = true
	cid, err := CommitReservationToLedger(res)
	if err != nil {
		return err
	}
	res.LedgerCID = cid
	return nil
}

func UpdateReservationStatus(res *Reservation) error {
	res.XRPRequirementMet = CheckXRPRequirement()
	res.CloudStormCosts = CheckCloudStormCosts()
	if res.XRPRequirementMet && res.CloudStormCosts && !res.NFTGenerated {
		return TryGenerateNFT(res)
	}
	return nil
}

func MarshalReservation(res *Reservation) ([]byte, error) {
	return json.Marshal(res)
}

func UnmarshalReservation(data []byte) (*Reservation, error) {
	var res Reservation
	err := json.Unmarshal(data, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
