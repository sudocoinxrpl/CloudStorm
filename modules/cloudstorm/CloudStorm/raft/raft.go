// -------------------- raft/raft.go (fully integrated + exported IBT types) --------------------
package raft

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	// Hypothetical imports for XRPL / NFT
	"CloudStorm/nft"
	"CloudStorm/xumm"
)

// MasterIssuerAddress is the XRPL address recognized as the "master" for host licensing.
const MasterIssuerAddress = "rBZYpQCRfxiy2NhVDjEj9p74PXnErTXpWk"

// RaftState enumerates a node's possible raft states.
type RaftState int

const (
	Follower RaftState = iota
	Candidate
	Leader
)

// ------------------------------------------------------------------------
// LogEntry & Data Structures
// ------------------------------------------------------------------------

// LogEntry holds a term, index, and a command payload.
type LogEntry struct {
	Index   int         `json:"index"`
	Term    int         `json:"term"`
	Command interface{} `json:"command"`
}

// Network represents a CloudStorm network bound to XRPL assets (master licensing).
type Network struct {
	ID              string `json:"id"`
	TokenIssuerAddr string `json:"token_issuer_address"`
	MasterLicenseID string `json:"master_license_id"`
}

// ContainerConsensus represents container-level consensus state, updated via raft.
type ContainerConsensus struct {
	ContainerID string `json:"container_id"`
	StateHash   string `json:"state_hash"`
	Timestamp   int64  `json:"timestamp"`
}

// Job holds metadata about posted or accepted tasks (including e.g. "NodeOnboarding").
type Job struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Payload       string `json:"payload"`
	Issuer        string `json:"issuer"`
	LicenseNFTCID string `json:"license_nft_cid"`
	RippleAddress string `json:"ripple_address"`
	Status        string `json:"status"`
}

// ------------------------------------------------------------------------
// Raft RPC Structures
// ------------------------------------------------------------------------

type VoteRequest struct {
	Term          int    `json:"term"`
	CandidateID   string `json:"candidate_id"`
	LastLogIndex  int    `json:"last_log_index"`
	LastLogTerm   int    `json:"last_log_term"`
	ServiceID     string `json:"service_id"`
	ProofKeyHash  string `json:"proof_key_hash"`
	CombinedProof string `json:"combined_proof"`
}

type VoteResponse struct {
	Term        int  `json:"term"`
	VoteGranted bool `json:"vote_granted"`
}

type AppendEntriesRequest struct {
	Term          int        `json:"term"`
	LeaderID      string     `json:"leader_id"`
	PrevLogIndex  int        `json:"prev_log_index"`
	PrevLogTerm   int        `json:"prev_log_term"`
	Entries       []LogEntry `json:"entries"`
	LeaderCommit  int        `json:"leader_commit"`
	ServiceID     string     `json:"service_id"`
	ProofKeyHash  string     `json:"proof_key_hash"`
	CombinedProof string     `json:"combined_proof"`
}

type AppendEntriesResponse struct {
	Term    int  `json:"term"`
	Success bool `json:"success"`
}

// ------------------------------------------------------------------------
// iBT Interconnect Logic (Optional 3D/ND iBT example for scheduling or routing).
// ------------------------------------------------------------------------

// IBTDimension describes a single dimension's size and bypass arcs (capitalized for export).
type IBTDimension struct {
	Size          int   // e.g., number of positions
	BypassSchemes []int // e.g., [8,12] means arcs that skip 8 or 12 nodes
}

// IBTCoordinates are the dimension coordinates of a node (for distance computations).
type IBTCoordinates []int

// ComputeIBTDistance calculates the number of hops between two nodes in a d-dimensional iBT network.
//
//	dims     = array describing each dimension (size + bypass arcs).
//	nodeA,B  = IBTCoordinates in the same dimension layout.
//	allPorts = if true, we can move in multiple dimensions simultaneously (like an all-port).
func ComputeIBTDistance(nodeA, nodeB IBTCoordinates, dims []IBTDimension, allPorts bool) int {
	if len(dims) != len(nodeA) || len(dims) != len(nodeB) {
		// Dimension mismatch => invalid
		return 999999999
	}
	distPerDim := make([]int, len(dims))

	for i, dconf := range dims {
		diff := (nodeB[i] - nodeA[i]) % dconf.Size
		if diff < 0 {
			diff += dconf.Size
		}
		// ring distance
		ringDist := diff
		if ringDist > dconf.Size-ringDist {
			ringDist = dconf.Size - ringDist
		}
		best := ringDist
		// Check each bypass arc to see if it shortens the hop count
		for _, bsize := range dconf.BypassSchemes {
			hops := int(math.Ceil(float64(ringDist) / float64(bsize)))
			if hops < best {
				best = hops
			}
		}
		distPerDim[i] = best
	}

	if allPorts {
		// all-port => total distance is max among dimension distances
		maxDist := 0
		for _, d := range distPerDim {
			if d > maxDist {
				maxDist = d
			}
		}
		return maxDist
	}
	// one-port => total distance is sum among dimension distances
	sumDist := 0
	for _, d := range distPerDim {
		sumDist += d
	}
	return sumDist
}

// ------------------------------------------------------------------------
// RaftNode Definition
// ------------------------------------------------------------------------

// RaftNode represents a node within the CloudStorm consensus cluster.
type RaftNode struct {
	mutex       sync.Mutex
	state       RaftState
	currentTerm int
	votedFor    string
	log         []LogEntry

	commitIndex int
	lastApplied int

	nextIndex  map[string]int
	matchIndex map[string]int

	id        string
	peers     []string
	db        *bolt.DB
	tlsConfig *tls.Config

	// Internal Data
	jobQueue             map[string]Job
	Networks             map[string]Network
	ContainerConsensusDB map[string]ContainerConsensus

	// iBT NodeCoord storage (OPTIONAL for scheduling)
	nodeCoords map[string]IBTCoordinates
	ibtDims    []IBTDimension
	allPorts   bool

	electionTimeout time.Duration
	heartbeat       time.Duration
	stopChan        chan struct{}
	wg              sync.WaitGroup
}

// NewRaftNode initializes a RaftNode with a sentinel log entry + local DB + optional TLS + iBT dims.
func NewRaftNode(
	id string,
	peers []string,
	dbPath string,
	tlsCfg *tls.Config,
	dims []IBTDimension,
	useAllPorts bool,
) (*RaftNode, error) {

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, err
	}
	return &RaftNode{
		state:     Follower,
		log:       []LogEntry{{Index: 0, Term: 0}}, // sentinel entry
		id:        id,
		peers:     peers,
		db:        db,
		tlsConfig: tlsCfg,

		jobQueue:             make(map[string]Job),
		Networks:             make(map[string]Network),
		ContainerConsensusDB: make(map[string]ContainerConsensus),
		nodeCoords:           make(map[string]IBTCoordinates),
		ibtDims:              dims,
		allPorts:             useAllPorts,

		electionTimeout: 150 * time.Millisecond,
		heartbeat:       50 * time.Millisecond,
		stopChan:        make(chan struct{}),
		nextIndex:       make(map[string]int),
		matchIndex:      make(map[string]int),
	}, nil
}

func (rn *RaftNode) Start() {
	rn.wg.Add(1)
	go rn.run()
}

func (rn *RaftNode) Stop() {
	close(rn.stopChan)
	rn.wg.Wait()
	rn.db.Close()
}

// SetNodeCoordinate optionally tracks a node's coordinate in iBT space (for scheduling).
func (rn *RaftNode) SetNodeCoordinate(nodeID string, coord IBTCoordinates) {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()
	rn.nodeCoords[nodeID] = coord
}

// GetNodeCoordinate retrieves a node's iBT coordinate.
func (rn *RaftNode) GetNodeCoordinate(nodeID string) (IBTCoordinates, bool) {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()
	coord, ok := rn.nodeCoords[nodeID]
	return coord, ok
}

// CreateNetwork securely verifies a host license via XRPL, then appends the new Network to the log.
func (rn *RaftNode) CreateNetwork(networkID, tokenIssuerAddr, xrplTxID string) error {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()
	if rn.state != Leader {
		return errors.New("only leader can create networks")
	}

	masterLicenseID, err := rn.verifyMasterHostLicense(tokenIssuerAddr, xrplTxID)
	if err != nil {
		return fmt.Errorf("failed master license verification: %w", err)
	}
	netw := Network{
		ID:              networkID,
		TokenIssuerAddr: tokenIssuerAddr,
		MasterLicenseID: masterLicenseID,
	}
	rn.Networks[networkID] = netw

	return rn.AppendCommand(netw)
}

// verifyMasterHostLicense checks XRPL ledger data from xumm for a valid license transaction.
func (rn *RaftNode) verifyMasterHostLicense(issuerAddr, xrplTxID string) (string, error) {
	tx, err := xumm.FetchTransaction(xrplTxID)
	if err != nil {
		return "", err
	}
	if tx.Account != issuerAddr || tx.Destination != MasterIssuerAddress {
		return "", errors.New("transaction issuer mismatch or incorrect master issuer destination")
	}
	if !tx.Validated {
		return "", errors.New("XRPL transaction not validated yet")
	}
	if tx.TransactionType != "Payment" && tx.TransactionType != "NFTokenMint" {
		return "", errors.New("unsupported transaction type for license")
	}
	return tx.Hash, nil
}

// UpdateContainerConsensus sets container-level state in the ContainerConsensusDB, replicates it.
func (rn *RaftNode) UpdateContainerConsensus(containerID, stateHash string) error {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()

	cons := ContainerConsensus{
		ContainerID: containerID,
		StateHash:   stateHash,
		Timestamp:   time.Now().Unix(),
	}
	rn.ContainerConsensusDB[containerID] = cons

	return rn.AppendCommand(cons)
}

// PostJob enqueues a new job in this node's jobQueue and replicates it across the cluster.
func (rn *RaftNode) PostJob(job Job) error {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()
	if _, exists := rn.jobQueue[job.ID]; exists {
		return errors.New("job already exists")
	}
	job.Status = "queued"
	rn.jobQueue[job.ID] = job
	return rn.AppendCommand(job)
}

// AcceptJob transitions a queued job to accepted, replicates that update.
func (rn *RaftNode) AcceptJob(jobID string) error {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()
	job, ok := rn.jobQueue[jobID]
	if !ok {
		return errors.New("job not found")
	}
	if job.Status != "queued" {
		return errors.New("job is not in a queued state")
	}
	job.Status = "accepted"
	rn.jobQueue[jobID] = job
	return rn.AppendCommand(job)
}

// run is the main entrypoint for the node's internal raft state machine.
func (rn *RaftNode) run() {
	defer rn.wg.Done()
	for {
		select {
		case <-rn.stopChan:
			return
		default:
			switch rn.state {
			case Follower:
				rn.runFollower()
			case Candidate:
				rn.runCandidate()
			case Leader:
				rn.runLeader()
			}
		}
	}
}

func (rn *RaftNode) runFollower() {
	timer := time.NewTimer(rn.electionTimeout)
	defer timer.Stop()

	for {
		select {
		case <-rn.stopChan:
			return
		case <-timer.C:
			rn.mutex.Lock()
			rn.state = Candidate
			rn.mutex.Unlock()
			return
		}
	}
}

func (rn *RaftNode) runCandidate() {
	rn.mutex.Lock()
	rn.currentTerm++
	rn.votedFor = rn.id
	votes := 1 // self-vote
	lastLogIndex := len(rn.log) - 1
	lastLogTerm := rn.log[lastLogIndex].Term
	rn.mutex.Unlock()

	timer := time.NewTimer(rn.electionTimeout)
	defer timer.Stop()

	voteChan := make(chan bool, len(rn.peers))
	for _, peer := range rn.peers {
		go func(pr string) {
			req := VoteRequest{
				Term:         rn.currentTerm,
				CandidateID:  rn.id,
				LastLogIndex: lastLogIndex,
				LastLogTerm:  lastLogTerm,
			}
			resp, err := sendVoteRequest(pr, req, 3, 100*time.Millisecond)
			if err != nil {
				log.Printf("Vote request to %s failed: %v", pr, err)
				voteChan <- false
				return
			}
			rn.mutex.Lock()
			defer rn.mutex.Unlock()
			if resp.Term > rn.currentTerm {
				rn.currentTerm = resp.Term
				rn.state = Follower
				rn.votedFor = ""
				voteChan <- false
				return
			}
			voteChan <- resp.VoteGranted
		}(peer)
	}

	for {
		select {
		case <-rn.stopChan:
			return
		case <-timer.C:
			return // election timed out
		case granted := <-voteChan:
			if granted {
				votes++
			}
			if votes > len(rn.peers)/2 {
				rn.mutex.Lock()
				rn.state = Leader
				for _, p := range rn.peers {
					rn.nextIndex[p] = len(rn.log)
					rn.matchIndex[p] = 0
				}
				rn.mutex.Unlock()
				return
			}
		}
	}
}

func (rn *RaftNode) runLeader() {
	rn.sendHeartbeats()
	ticker := time.NewTicker(rn.heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-rn.stopChan:
			return
		case <-ticker.C:
			rn.sendHeartbeats()
			rn.updateCommitIndex()
		}
	}
}

func (rn *RaftNode) sendHeartbeats() {
	rn.mutex.Lock()
	term := rn.currentTerm
	logLen := len(rn.log)
	commitIndex := rn.commitIndex
	rn.mutex.Unlock()

	for _, peer := range rn.peers {
		go func(pr string) {
			rn.mutex.Lock()
			prevLogIndex := rn.nextIndex[pr] - 1
			prevLogTerm := 0
			if prevLogIndex >= 0 && prevLogIndex < len(rn.log) {
				prevLogTerm = rn.log[prevLogIndex].Term
			}
			var entries []LogEntry
			if rn.nextIndex[pr] < logLen {
				entries = rn.log[rn.nextIndex[pr]:]
			}
			req := AppendEntriesRequest{
				Term:         term,
				LeaderID:     rn.id,
				PrevLogIndex: prevLogIndex,
				PrevLogTerm:  prevLogTerm,
				Entries:      entries,
				LeaderCommit: commitIndex,
			}
			rn.mutex.Unlock()

			resp, err := sendAppendEntries(pr, req, 3, 100*time.Millisecond)
			if err != nil {
				log.Printf("AppendEntries to %s failed: %v", pr, err)
				return
			}
			rn.mutex.Lock()
			defer rn.mutex.Unlock()
			if resp.Term > rn.currentTerm {
				rn.currentTerm = resp.Term
				rn.state = Follower
				rn.votedFor = ""
				return
			}
			if resp.Success {
				rn.nextIndex[pr] = logLen
				rn.matchIndex[pr] = logLen - 1
			} else {
				rn.nextIndex[pr]--
				if rn.nextIndex[pr] < 1 {
					rn.nextIndex[pr] = 1
				}
			}
		}(peer)
	}
}

func (rn *RaftNode) updateCommitIndex() {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()
	for n := rn.commitIndex + 1; n < len(rn.log); n++ {
		count := 1
		for _, p := range rn.peers {
			if rn.matchIndex[p] >= n {
				count++
			}
		}
		if count > len(rn.peers)/2 && rn.log[n].Term == rn.currentTerm {
			rn.commitIndex = n
			rn.applyLogEntries()
		}
	}
}

func (rn *RaftNode) applyLogEntries() {
	for rn.lastApplied < rn.commitIndex {
		rn.lastApplied++
		if err := ProcessLogEntry(rn.log[rn.lastApplied]); err != nil {
			log.Printf("Error applying log entry %d: %v", rn.lastApplied, err)
		}
	}
}

// ------------------------------------------------------------------------
// ProcessLogEntry
// ------------------------------------------------------------------------

// ProcessLogEntry checks whether the command is Network, Job, or ContainerConsensus
// and applies it accordingly (e.g. calls nft.IssueNFT for NodeOnboarding).
func ProcessLogEntry(entry LogEntry) error {
	data, err := json.Marshal(entry.Command)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	// 1) Check if it's a new Network command
	var netw Network
	if err := json.Unmarshal(data, &netw); err == nil && netw.MasterLicenseID != "" {
		log.Printf("New CloudStorm Network '%s' created, XRPL Issuer: %s, Master License: %s",
			netw.ID, netw.TokenIssuerAddr, netw.MasterLicenseID)
		return nil
	}

	// 2) Check if it's a Job command, e.g. node onboarding
	var job Job
	if err := json.Unmarshal(data, &job); err == nil && job.Type == "NodeOnboarding" {
		// For demonstration, use the nft package to issue an NFT for the license
		if err := nft.IssueNFT(job.Issuer, job.LicenseNFTCID); err != nil {
			return fmt.Errorf("failed to issue NFT: %w", err)
		}
		log.Printf("Node onboarded with issuer: %s", job.Issuer)
		return nil
	}

	// 3) Check if it's a ContainerConsensus update
	var consensus ContainerConsensus
	if err := json.Unmarshal(data, &consensus); err == nil && consensus.ContainerID != "" {
		log.Printf("Container consensus updated: Container %s, StateHash %s, Timestamp %d",
			consensus.ContainerID, consensus.StateHash, consensus.Timestamp)
		return nil
	}

	// If it doesn't match any known command type, we silently succeed.
	return nil
}

// ------------------------------------------------------------------------
// Additional Helper for iBT Scheduling
// ------------------------------------------------------------------------

// PickBestNodeForJob uses iBT distance among known nodeCoords to find the minimal distance node.
func (rn *RaftNode) PickBestNodeForJob() (string, error) {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()

	// Suppose we pick the best node for the job relative to our own coordinate
	selfCoord, ok := rn.nodeCoords[rn.id]
	if !ok {
		return "", errors.New("local node does not have iBT coordinates set")
	}
	bestID := ""
	bestDist := 999999999

	for nodeID, coord := range rn.nodeCoords {
		if nodeID == rn.id {
			continue // skip ourselves if we only want remote nodes
		}
		dist := ComputeIBTDistance(selfCoord, coord, rn.ibtDims, rn.allPorts)
		if dist < bestDist {
			bestDist = dist
			bestID = nodeID
		}
	}
	if bestID == "" {
		return "", errors.New("no other nodes found in iBT coordinate map")
	}
	return bestID, nil
}

// ScheduleJob example: picks the best node for the job, sets job fields, calls PostJob to replicate it.
func (rn *RaftNode) ScheduleJob(jobID, jobType, issuer, cid string) error {
	node, err := rn.PickBestNodeForJob()
	if err != nil {
		return err
	}
	newJob := Job{
		ID:            jobID,
		Type:          jobType,
		Issuer:        issuer,
		LicenseNFTCID: cid,
		RippleAddress: node, // store the chosen node as the job's assigned node
		Status:        "queued",
	}
	return rn.PostJob(newJob)
}

// ------------------------------------------------------------------------
// Global Node & LogStatus
// ------------------------------------------------------------------------

var globalNode *RaftNode

// SetGlobalNode assigns a RaftNode as the package-wide default (for referencing from main).
func SetGlobalNode(rn *RaftNode) {
	globalNode = rn
}

// LogStatus returns a textual summary of the default (global) node's state (for main.go usage).
func LogStatus() string {
	if globalNode == nil {
		return "No global node configured"
	}
	globalNode.mutex.Lock()
	defer globalNode.mutex.Unlock()
	return fmt.Sprintf(
		"Term: %d, Log length: %d, CommitIndex: %d",
		globalNode.currentTerm,
		len(globalNode.log),
		globalNode.commitIndex,
	)
}

// AppendCommand appends the given command to the local log if node is Leader.
func (rn *RaftNode) AppendCommand(command interface{}) error {
	rn.mutex.Lock()
	defer rn.mutex.Unlock()
	if rn.state != Leader {
		return errors.New("not the leader")
	}
	entry := LogEntry{
		Index:   len(rn.log),
		Term:    rn.currentTerm,
		Command: command,
	}
	rn.log = append(rn.log, entry)
	return nil
}

// ------------------------------------------------------------------------
// Vote & AppendEntries RPC Helpers (multi-attempt with local proof check)
// ------------------------------------------------------------------------

func sendVoteRequest(peerURL string, req VoteRequest, attempts int, baseTimeout time.Duration) (VoteResponse, error) {
	// Insert local consensus proof if empty:
	if req.ServiceID == "" || req.ProofKeyHash == "" {
		sid, pkh := getLocalConsensusProof()
		req.ServiceID = sid
		req.ProofKeyHash = pkh
	}
	if req.CombinedProof == "" {
		req.CombinedProof = CombineProof(req.ServiceID, req.ProofKeyHash)
	}
	if err := ValidateConsensusProof(req.ServiceID, req.ProofKeyHash, req.CombinedProof); err != nil {
		return VoteResponse{}, err
	}

	var voteResp VoteResponse
	data, err := json.Marshal(req)
	if err != nil {
		return voteResp, err
	}

	var finalErr error
	for i := 0; i < attempts; i++ {
		client := &http.Client{Timeout: baseTimeout}
		httpReq, err := http.NewRequest("POST", peerURL+"/requestVote", bytes.NewReader(data))
		if err != nil {
			finalErr = err
			time.Sleep(baseTimeout)
			continue
		}
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(httpReq)
		if err != nil {
			finalErr = err
			time.Sleep(baseTimeout)
			continue
		}
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(&voteResp); err != nil {
			finalErr = err
			time.Sleep(baseTimeout)
			continue
		}
		return voteResp, nil
	}
	return voteResp, finalErr
}

func sendAppendEntries(peerURL string, req AppendEntriesRequest, attempts int, baseTimeout time.Duration) (AppendEntriesResponse, error) {
	if req.ServiceID == "" || req.ProofKeyHash == "" {
		sid, pkh := getLocalConsensusProof()
		req.ServiceID = sid
		req.ProofKeyHash = pkh
	}
	if req.CombinedProof == "" {
		req.CombinedProof = CombineProof(req.ServiceID, req.ProofKeyHash)
	}
	if err := ValidateConsensusProof(req.ServiceID, req.ProofKeyHash, req.CombinedProof); err != nil {
		return AppendEntriesResponse{}, err
	}

	var finalResp AppendEntriesResponse
	data, err := json.Marshal(req)
	if err != nil {
		return finalResp, err
	}

	var finalErr error
	for i := 0; i < attempts; i++ {
		client := &http.Client{Timeout: baseTimeout}
		httpReq, err := http.NewRequest("POST", peerURL+"/appendEntries", bytes.NewReader(data))
		if err != nil {
			finalErr = err
			time.Sleep(baseTimeout)
			continue
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpResp, err := client.Do(httpReq)
		if err != nil {
			finalErr = err
			time.Sleep(baseTimeout)
			continue
		}
		defer httpResp.Body.Close()
		if err := json.NewDecoder(httpResp.Body).Decode(&finalResp); err != nil {
			finalErr = err
			time.Sleep(baseTimeout)
			continue
		}
		return finalResp, nil
	}
	return finalResp, finalErr
}

// ------------------------------------------------------------------------
// Local Trinity Proof Integration
// ------------------------------------------------------------------------

func getLocalConsensusProof() (string, string) {
	r, err := http.Get("http://localhost:7501/consensus")
	if err != nil {
		log.Printf("Error retrieving consensus proof: %v", err)
		return "", ""
	}
	defer r.Body.Close()
	var d struct {
		ServiceID    string `json:"service_id"`
		ProofKeyHash string `json:"proof_key_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		log.Printf("Error decoding consensus proof: %v", err)
		return "", ""
	}
	return d.ServiceID, d.ProofKeyHash
}

func CombineProof(serviceID, proofKeyHash string) string {
	data := []byte(serviceID + proofKeyHash)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func ValidateConsensusProof(serviceID, proofKeyHash, combinedProof string) error {
	if len(serviceID) != 64 {
		return errors.New("invalid serviceID length; must be 64 hex chars")
	}
	if len(proofKeyHash) != 64 {
		return errors.New("invalid proofKeyHash length; must be 64 hex chars")
	}
	expected := CombineProof(serviceID, proofKeyHash)
	if combinedProof != expected {
		return fmt.Errorf("combined proof mismatch: expected %s, got %s", expected, combinedProof)
	}
	return nil
}
