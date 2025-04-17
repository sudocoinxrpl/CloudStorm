// -------------------- governance/governance.go --------------------

// Cloud Storm governance and self updating functionalities (through whitelisting)
package governance

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

type Proposal struct {
	ID           string    `json:"id"`
	ServiceID    string    `json:"service_id"`
	ProposedRate float64   `json:"proposed_rate"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	VotesFor     int       `json:"votes_for"`
	VotesAgainst int       `json:"votes_against"`
	Executed     bool      `json:"executed"`
}

// GovernanceState holds on-chain or local governance info.
type GovernanceState struct {
	PoolRates map[string]float64   `json:"pool_rates"`
	Whitelist map[string]bool      `json:"whitelist"`
	Proposals map[string]*Proposal `json:"proposals"`
	Mutex     sync.Mutex           `json:"-"`
}

var state = GovernanceState{
	PoolRates: make(map[string]float64),
	Whitelist: make(map[string]bool),
	Proposals: make(map[string]*Proposal),
}

func generateProposalID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ProposePoolRateChange creates a new proposal to change a "pool rate."
// Only whitelisted serviceIDs can propose changes.
func ProposePoolRateChange(serviceID string, proposedRate float64, votingDuration time.Duration) (string, error) {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	if _, ok := state.Whitelist[serviceID]; !ok {
		return "", errors.New("serviceID not whitelisted")
	}
	proposalID, err := generateProposalID()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	proposal := &Proposal{
		ID:           proposalID,
		ServiceID:    serviceID,
		ProposedRate: proposedRate,
		StartTime:    now,
		EndTime:      now.Add(votingDuration),
		VotesFor:     0,
		VotesAgainst: 0,
		Executed:     false,
	}
	state.Proposals[proposalID] = proposal
	return proposalID, nil
}

// VoteOnProposal increments votes for or against a proposal if still open.
func VoteOnProposal(proposalID string, vote bool) error {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	proposal, ok := state.Proposals[proposalID]
	if !ok {
		return errors.New("proposal not found")
	}
	if time.Now().UTC().After(proposal.EndTime) {
		return errors.New("voting period has ended")
	}
	if vote {
		proposal.VotesFor++
	} else {
		proposal.VotesAgainst++
	}
	return nil
}

// ExecuteProposal finalizes the proposal if voting is over, meets quorum, etc.
func ExecuteProposal(proposalID string, quorum int) error {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	proposal, ok := state.Proposals[proposalID]
	if !ok {
		return errors.New("proposal not found")
	}
	if time.Now().UTC().Before(proposal.EndTime) {
		return errors.New("voting period not ended")
	}
	if proposal.Executed {
		return errors.New("proposal already executed")
	}
	totalVotes := proposal.VotesFor + proposal.VotesAgainst
	if totalVotes < quorum {
		return errors.New("quorum not reached")
	}
	if proposal.VotesFor > proposal.VotesAgainst {
		// e.g. implement the rate:
		state.PoolRates[proposal.ServiceID] = proposal.ProposedRate
	}
	proposal.Executed = true
	return nil
}

// AddToWhitelist adds a serviceID to the local governance whitelist.
func AddToWhitelist(serviceID string) {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	state.Whitelist[serviceID] = true
}

// RemoveFromWhitelist removes a serviceID from the local governance whitelist.
func RemoveFromWhitelist(serviceID string) {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	delete(state.Whitelist, serviceID)
}

// GetWhitelist returns the currently whitelisted serviceIDs.
func GetWhitelist() []string {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	list := make([]string, 0, len(state.Whitelist))
	for id := range state.Whitelist {
		list = append(list, id)
	}
	return list
}

// MarshalGovernanceState serializes the entire governance state.
func MarshalGovernanceState() ([]byte, error) {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	return json.Marshal(state)
}

// UnmarshalGovernanceState loads the entire governance state from JSON.
func UnmarshalGovernanceState(data []byte) error {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	return json.Unmarshal(data, &state)
}
