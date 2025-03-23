package main

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"strings"
	"sync"
)

// WebSocket42 A message starting with the number 42 and then a JSON array. The 1st element is the action/event e.g. on_upload_success, on_asset_delete. Other elements vary depending on the action
type WebSocket42 []any

func (wsMsg WebSocket42) getAction() string {
	if len(wsMsg) < 2 {
		return ""
	}
	if v, ok := wsMsg[0].(string); ok {
		return v
	}
	return ""
}

func (wsMsg WebSocket42) getAsset() Asset {
	if len(wsMsg) < 2 {
		return nil
	}
	if v, ok := wsMsg[1].(map[string]any); ok {
		return v
	}
	return nil
}

func handleWebSocketConn(cliConn, srvConn *websocket.Conn, logger *customLogger) {
	var wg sync.WaitGroup
	wg.Add(2)
	logger.SetErrPrefix("websocket proxy")
	go func() {
		defer wg.Done()
		var err error
		var msgType int
		var message []byte
		for {
			if msgType, message, err = srvConn.ReadMessage(); logger.Error(err, "srv ReadMessage") {
				break
			}
			//fmt.Printf("SRV: Type: %d Message: %s\n", msgType, message)
			if msgType == websocket.TextMessage && len(message) > 2 && bytes.Equal(message[:2], []byte("42")) {
				var wsMsg WebSocket42
				if err = json.Unmarshal(message[2:], &wsMsg); logger.Error(err, "json unmarshal") {
					continue
				}
				if asset := wsMsg.getAsset(); asset != nil && wsMsg.getAction() == "on_upload_success" {
					mapLock.RLock()
					asset.toOriginalAsset()
					mapLock.RUnlock()
					if message, err = json.Marshal(wsMsg); logger.Error(err, "json encode") {
						continue
					}
					message = append([]byte("42"), message...)
				}
			}
			if err = cliConn.WriteMessage(msgType, message); logger.Error(err, "cli WriteMessage") {
				break
			}
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		var msgType int
		var message []byte
		for {
			if msgType, message, err = cliConn.ReadMessage(); logger.Error(err, "cli ReadMessage") {
				break
			}
			if err = srvConn.WriteMessage(msgType, message); logger.Error(err, "srv WriteMessage") {
				break
			}
		}
	}()
	wg.Wait()
}

func upgradeWebSocketRequest(w http.ResponseWriter, r *http.Request, logger *customLogger) {
	var err error
	logger.SetErrPrefix("websocket")
	logger.Printf("websocket upgrade")
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	var cliConn, srvConn *websocket.Conn
	if cliConn, err = upgrader.Upgrade(w, r, nil); logger.Error(err, "upgrade") {
		return
	}
	defer cliConn.Close()
	if srvConn, _, err = websocket.DefaultDialer.Dial("ws"+upstreamURL[strings.Index(upstreamURL, ":"):]+r.URL.String(), webSocketSafeHeader(r.Header)); logger.Error(err, "dial") {
		return
	}
	defer srvConn.Close()
	handleWebSocketConn(cliConn, srvConn, logger)
}
