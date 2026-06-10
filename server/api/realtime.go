package api

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type Hub struct {
	mu    sync.Mutex
	rooms map[string]map[*websocket.Conn]bool
}

func NewHub() *Hub {
	return &Hub{rooms: map[string]map[*websocket.Conn]bool{}}
}

func (h *Hub) ServeRoom(w http.ResponseWriter, r *http.Request, room string) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.join(room, conn)
	defer h.leave(room, conn)
	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		h.broadcast(room, mt, msg, conn)
	}
}

func (h *Hub) join(room string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[room] == nil {
		h.rooms[room] = map[*websocket.Conn]bool{}
	}
	h.rooms[room][conn] = true
}

func (h *Hub) leave(room string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms[room], conn)
	_ = conn.Close()
	if len(h.rooms[room]) == 0 {
		delete(h.rooms, room)
	}
}

func (h *Hub) broadcast(room string, mt int, msg []byte, sender *websocket.Conn) {
	h.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(h.rooms[room]))
	for conn := range h.rooms[room] {
		if conn != sender {
			conns = append(conns, conn)
		}
	}
	h.mu.Unlock()
	for _, conn := range conns {
		_ = conn.WriteMessage(mt, msg)
	}
}
