// File: ws/ws.go

// websocket handler for qtp protocol over TOR
package ws

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"

	"github.com/gorilla/websocket"
)

var Upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type QTPHeader struct {
	Data      string `json:"data"`
	Signature string `json:"signature"`
}

func WsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return
	}
	var hdr QTPHeader
	if err := json.Unmarshal(msg, &hdr); err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Invalid QTP header"))
		return
	}
	// For demonstration, signature verification is omitted.
	conn.WriteMessage(websocket.TextMessage, []byte("Ledger updated"))
}

func GetPublicKey(address string) string {
	out, err := exec.Command("./util/lookupPublicKey", address).Output()
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(string(out))
}
