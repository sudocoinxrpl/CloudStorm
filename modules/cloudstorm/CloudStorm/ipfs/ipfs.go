// -------------------- ipfs/ipfs.go --------------------

//middleware for ipfs functionalities

package ipfs

import (
	"bytes"
	"encoding/json"
	"io"

	shell "github.com/ipfs/go-ipfs-api"
)

type LedgerBlock struct {
	BlockHeight  int                    `json:"block_height"`
	PrevHash     string                 `json:"prev_hash"`
	BlockHash    string                 `json:"block_hash"`
	Timestamp    string                 `json:"timestamp"`
	Records      map[string]interface{} `json:"records"`
	Certificates map[string]interface{} `json:"certificates"`
	Term         int                    `json:"term"`
	CommitSig    string                 `json:"commit_sig"`
}

type IPFSClient struct {
	Shell *shell.Shell
}

func NewClient(apiEndpoint string) *IPFSClient {
	return &IPFSClient{Shell: shell.NewShell(apiEndpoint)}
}

func (c *IPFSClient) StoreLedger(lb LedgerBlock) (string, error) {
	data, err := json.Marshal(lb)
	if err != nil {
		return "", err
	}
	return c.Shell.Add(bytes.NewReader(data))
}

func (c *IPFSClient) FetchLedger(cid string) (LedgerBlock, error) {
	var lb LedgerBlock
	reader, err := c.Shell.Cat(cid)
	if err != nil {
		return lb, err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return lb, err
	}
	if err := json.Unmarshal(data, &lb); err != nil {
		return lb, err
	}
	return lb, nil
}

func (c *IPFSClient) StoreRecord(record interface{}) (string, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	return c.Shell.Add(bytes.NewReader(data))
}
