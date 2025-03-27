// File: trinity/trinity.go

// onboard trinity instance (this is what connects the local consensus network between port 7501 with the greater consensus network that occurs over TOR via websocket and raft.go)

package trinity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Node represents a single directory or file entry in our hash-chained structure.
type Node struct {
	IsDir    bool
	RelPath  string   // e.g. ".", "subdir/file.txt", etc.
	Hash     [32]byte // computed recursively
	FileSize int64
	Children []Node // for directories
	// We do not store file contents in memory after hashing, to keep memory usage lower
}

// buildNode walks a path, constructing either a file node or a dir node with children.
func buildNode(absPath, relPath string) (Node, error) {
	fi, err := os.Stat(absPath)
	if err != nil {
		return Node{}, err
	}
	node := Node{
		IsDir:   fi.IsDir(),
		RelPath: relPath,
	}
	if node.IsDir {
		// gather children
		entries, err := os.ReadDir(absPath)
		if err != nil {
			return Node{}, err
		}
		// sort by name so the hashing is stable
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name() < entries[j].Name()
		})
		for _, e := range entries {
			childName := e.Name()
			childAbs := filepath.Join(absPath, childName)
			childRel := filepath.Join(relPath, childName)
			// build child node
			childNode, errC := buildNode(childAbs, childRel)
			if errC != nil {
				return Node{}, errC
			}
			node.Children = append(node.Children, childNode)
		}
		// compute directory hash
		node.Hash = computeDirectoryHash(node)
	} else {
		// is file
		node.FileSize = fi.Size()
		fileBytes, err := os.ReadFile(absPath)
		if err != nil {
			return Node{}, err
		}
		node.Hash = computeFileHash(node.RelPath, fileBytes, node.FileSize)
	}
	return node, nil
}

// computeFileHash calculates SHA-256 over ("FILE" + relPath + fileSize + rawFileData).
func computeFileHash(relPath string, data []byte, fileSize int64) [32]byte {
	h := sha256.New()
	h.Write([]byte("FILE"))
	h.Write([]byte(relPath))

	// fileSize as 8 bytes (little-endian or big-endian—just be consistent)
	szBuf := make([]byte, 8)
	// we can do it in big-endian:
	for i := 7; i >= 0; i-- {
		szBuf[i] = byte(fileSize & 0xFF)
		fileSize >>= 8
	}
	h.Write(szBuf)

	h.Write(data)

	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// computeDirectoryHash calculates SHA-256 over:
// ("DIR" + relPath + childCount + for each child -> child.relPath + child.Hash)
func computeDirectoryHash(dir Node) [32]byte {
	h := sha256.New()
	h.Write([]byte("DIR"))
	h.Write([]byte(dir.RelPath))

	// childCount as 8 bytes
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

// computeRootNode recursively builds the Node for baseDir and returns it.
func computeRootNode(baseDir string) (Node, error) {
	// ensure baseDir is absolute for convenience
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return Node{}, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return Node{}, err
	}
	if !fi.IsDir() {
		return Node{}, fmt.Errorf("ComputeServiceID: baseDir is not a directory: %s", baseDir)
	}
	return buildNode(abs, ".")
}

// ComputeServiceID calls the external C++ "trinity" binary in --oneshot mode.
func ComputeServiceID(baseDir string) (string, error) {
	// Example: ./trinity --oneshot <baseDir>
	cmd := exec.Command("./trinity", "--oneshot", baseDir)
	output, err := cmd.Output() // This captures stdout
	if err != nil {
		return "", fmt.Errorf("failed running trinity: %w", err)
	}
	// The output is the service ID, possibly with a newline
	sid := strings.TrimSpace(string(output))
	// You can sanity-check if it's empty or default
	if len(sid) == 0 {
		return "", fmt.Errorf("no service ID returned from trinity")
	}
	return sid, nil
}

// GenerateProofKeyHash just returns a random 32-byte hex string.
func GenerateProofKeyHash() (string, error) {
	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// ------------------- Trinity Global State for /consensus endpoint -------------------

var (
	globalMutex        sync.Mutex
	globalServiceID    string
	globalProofKeyHash string
	// Optionally track if we want to re-hash on file change, etc.
)

// StartTrinityServer calculates the serviceID from baseDir, generates a random proofKeyHash,
// and starts an HTTP server on the specified port that provides /consensus JSON.
func StartTrinityServer(baseDir string, port int) error {
	sid, err := ComputeServiceID(baseDir)
	if err != nil {
		return fmt.Errorf("ComputeServiceID failed: %w", err)
	}
	pkh, err := GenerateProofKeyHash()
	if err != nil {
		return fmt.Errorf("GenerateProofKeyHash failed: %w", err)
	}
	setTrinityState(sid, pkh)

	// set up /consensus handler
	http.HandleFunc("/consensus", func(w http.ResponseWriter, r *http.Request) {
		sID, pk := getTrinityState()
		// Return JSON
		fmt.Fprintf(w, `{"service_id":"%s","proof_key_hash":"%s"}`, sID, pk)
	})

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	log.Printf("[Trinity] Starting server on %s (BaseDir=%s)\n", addr, baseDir)
	return http.ListenAndServe(addr, nil)
}

// SetTrinityState can be used externally if you want to dynamically update the serviceID & proofKey.
func setTrinityState(sid, pkh string) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	globalServiceID = sid
	globalProofKeyHash = pkh
}

// getTrinityState returns the current serviceID and proofKeyHash
func getTrinityState() (string, string) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	return globalServiceID, globalProofKeyHash
}

// If you want a helper function that blocks forever:
func RunTrinityServerBlocking(baseDir string) {
	port := 7501
	if err := StartTrinityServer(baseDir, port); err != nil {
		log.Fatalf("Trinity server failed: %v", err)
	}
}

// If you want advanced usage or to rehash on demand, you can call ComputeServiceID again
// and update the global with setTrinityState(newSid, oldProofKeyHashOrNew).
