package trinity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Node struct {
	IsDir    bool
	RelPath  string
	Hash     [32]byte
	FileSize int64
	Children []Node
}

type ConsensusSnapshot struct {
	ServiceID     string `json:"service_id"`
	ProofKeyHash  string `json:"proof_key_hash"`
	Certificate   string `json:"cert,omitempty"`
	PrivateKey    string `json:"key,omitempty"`
	ContainerHint string `json:"source,omitempty"`
}

var (
	globalMutex        sync.Mutex
	globalServiceID    string
	globalProofKeyHash string
	localConsensusView []ConsensusSnapshot
)

func GenerateProofKeyHash() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func computeFileHash(relPath string, data []byte, size int64) [32]byte {
	h := sha256.New()
	h.Write([]byte("FILE"))
	h.Write([]byte(relPath))
	szBuf := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		szBuf[i] = byte(size & 0xFF)
		size >>= 8
	}
	h.Write(szBuf)
	h.Write(data)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func computeDirectoryHash(dir Node) [32]byte {
	h := sha256.New()
	h.Write([]byte("DIR"))
	h.Write([]byte(dir.RelPath))
	count := int64(len(dir.Children))
	szBuf := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		szBuf[i] = byte(count & 0xFF)
		count >>= 8
	}
	h.Write(szBuf)
	for _, c := range dir.Children {
		h.Write([]byte(c.RelPath))
		h.Write(c.Hash[:])
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func buildNode(absPath, relPath string) (Node, error) {
	fi, err := os.Stat(absPath)
	if err != nil {
		return Node{}, err
	}
	node := Node{IsDir: fi.IsDir(), RelPath: relPath}
	if node.IsDir {
		entries, err := os.ReadDir(absPath)
		if err != nil {
			return Node{}, err
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name() < entries[j].Name()
		})
		for _, e := range entries {
			childAbs := filepath.Join(absPath, e.Name())
			childRel := filepath.Join(relPath, e.Name())
			childNode, err := buildNode(childAbs, childRel)
			if err != nil {
				return Node{}, err
			}
			node.Children = append(node.Children, childNode)
		}
		node.Hash = computeDirectoryHash(node)
	} else {
		node.FileSize = fi.Size()
		data, err := os.ReadFile(absPath)
		if err != nil {
			return Node{}, err
		}
		node.Hash = computeFileHash(node.RelPath, data, node.FileSize)
	}
	return node, nil
}

func computeRootNode(baseDir string) (Node, error) {
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return Node{}, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return Node{}, err
	}
	if !fi.IsDir() {
		return Node{}, fmt.Errorf("baseDir is not a directory: %s", baseDir)
	}
	return buildNode(abs, ".")
}

func ComputeServiceID(baseDir string) (string, error) {
	rootNode, err := computeRootNode(baseDir)
	if err != nil {
		return "", fmt.Errorf("failed hashing service tree: %w", err)
	}
	return hex.EncodeToString(rootNode.Hash[:]), nil
}

func FetchLocalConsensus(trinityHost string, port int) (string, string, error) {
	url := fmt.Sprintf("http://%s:%d/consensus", trinityHost, port)
	resp, err := http.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("unable to contact Trinity at %s: %w", url, err)
	}
	defer resp.Body.Close()

	var result struct {
		ServiceID    string `json:"service_id"`
		ProofKeyHash string `json:"proof_key_hash"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("invalid JSON from Trinity: %w", err)
	}

	if result.ServiceID == "" || result.ProofKeyHash == "" {
		return "", "", errors.New("Trinity returned incomplete consensus data")
	}

	setTrinityState(result.ServiceID, result.ProofKeyHash)
	return result.ServiceID, result.ProofKeyHash, nil
}

func setTrinityState(sid, pkh string) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	globalServiceID = sid
	globalProofKeyHash = pkh
}

func getTrinityState() (string, string) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	return globalServiceID, globalProofKeyHash
}

func pollLocalTrinityInstances(host string, ports []int) {
	for {
		var results []ConsensusSnapshot
		for _, port := range ports {
			url := fmt.Sprintf("http://%s:%d/consensus", host, port)
			resp, err := http.Get(url)
			if err != nil {
				continue
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				continue
			}
			var snapshot ConsensusSnapshot
			if err := json.Unmarshal(body, &snapshot); err == nil && snapshot.ServiceID != "" {
				snapshot.ContainerHint = fmt.Sprintf("Trinity:%d", port)
				results = append(results, snapshot)
			}
		}
		globalMutex.Lock()
		localConsensusView = results
		globalMutex.Unlock()
		time.Sleep(15 * time.Second)
	}
}

func StartPeerPolling(host string, ports []int) {
	go pollLocalTrinityInstances(host, ports)
}
