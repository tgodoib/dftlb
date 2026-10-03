package handlers

import (
	"DFT_LB/codec"
	"DFT_LB/local"
	"log/slog"
	"sync"
	"time"
)

type Heartbeat struct {
	mu sync.RWMutex

	node     *local.Node
	interval time.Duration
	timeout  time.Duration
	lastSeen map[int]time.Time
}

func NewHeartbeat(node *local.Node, interval time.Duration, timeout time.Duration) *Heartbeat {
	return &Heartbeat{
		node:     node,
		interval: interval,
		timeout:  timeout,
		lastSeen: make(map[int]time.Time),
	}
}

func (h *Heartbeat) HandleMsg(env codec.IncomingEnvelope) {
	h.mu.Lock()
	h.lastSeen[env.From] = time.Now()
	h.mu.Unlock()
	//slog.Debug("Received heartbeat", "from", env.From)
}

func (h *Heartbeat) LogicLoop() {
	sendTicker := time.NewTicker(h.interval)
	checkTicker := time.NewTicker(time.Second)

	for {
		select {
		case <-sendTicker.C:
			for pID := range h.node.Peers {
				h.node.Bus.Send(codec.OutgoingEnvelope{
					To: pID,
					Msg: codec.Message{
						Type:    codec.MsgHeartbeat,
						Payload: nil,
					},
				})
				//slog.Debug("Heartbeat Sent", "to", pID)
			}
		case <-checkTicker.C:
			var timedOut []int

			h.mu.Lock()
			for pID, lastTime := range h.lastSeen {
				if time.Since(lastTime) > h.timeout {
					slog.Debug("Heartbeat timeout", "peerID", pID)
					delete(h.lastSeen, pID)
					timedOut = append(timedOut, pID)
				}
			}
			h.mu.Unlock()

			for _, pID := range timedOut {
				h.node.RemovePeer(pID)
			}
		}
	}
}

func (h *Heartbeat) RemoveTimer(peerID int) {
	h.mu.Lock()
	delete(h.lastSeen, peerID)
	h.mu.Unlock()
}
