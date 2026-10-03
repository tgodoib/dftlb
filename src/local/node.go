package local

import (
	"DFT_LB/codec"
	"DFT_LB/transport"
	"log/slog"
	"sync"
)

type Node struct {
	mu sync.RWMutex

	SelfID   int
	MasterID int
	Bus      *Bus

	Peers map[int]*transport.Peer

	onPeerAdded   []func(peerID int, peerIP string)
	onPeerRemoved []func(peerID int, peerIP string)
	onMasterSet   []func(newID int, oldID int)
}

func NewNode(id int, bus *Bus) *Node {

	return &Node{
		SelfID:   id,
		MasterID: -1,
		Bus:      bus,
		Peers:    make(map[int]*transport.Peer),
	}
}

func (n *Node) OutgoingLoop() {
	for env := range n.Bus.Outgoing {

		n.mu.RLock()
		peer, ok := n.Peers[env.To]
		n.mu.RUnlock()

		if ok {
			select {
			case peer.Outgoing <- &env.Msg:
			default:
				slog.Warn("outgoing buffer full, dropping message", "to", env.To)
			}
		}
	}
}

func (n *Node) AddPeer(peerID int, conn *transport.Peer) {
	n.mu.Lock()

	if old, ok := n.Peers[peerID]; ok {
		old.Conn.Close()
	}

	n.Peers[peerID] = conn
	n.mu.Unlock()

	slog.Debug("Nova conexão com peer", "id", peerID)

	go func() {
		defer func() {
			slog.Debug("Defer in add peer (conn closed)", "peer", peerID)
			n.RemovePeer(peerID)
		}()

		for msg := range conn.Incoming {
			n.Bus.Receive(codec.IncomingEnvelope{
				From: peerID,
				Msg:  *msg,
			})
		}
	}()

	for _, fn := range n.onPeerAdded {
		fn(peerID, conn.Conn.RemoteAddr().String())
	}
}

func (n *Node) RemovePeer(peerID int) {
	slog.Debug("Removing peer", "id", peerID)
	n.mu.Lock()
	defer n.mu.Unlock()

	peer, ok := n.Peers[peerID]
	if !ok {
		return
	}

	peer.Conn.Close()
	delete(n.Peers, peerID)

	slog.Debug("Calling removed peer callback")
	for _, fn := range n.onPeerRemoved {
		fn(peerID, peer.Conn.RemoteAddr().String())
	}

	slog.Debug("Removed peer", "id", peerID)
}

func (n *Node) SetMaster(id int) {
	n.mu.Lock()
	old := n.MasterID
	n.MasterID = id
	n.mu.Unlock()

	slog.Info("Novo Master", "id", id)

	for _, fn := range n.onMasterSet {
		fn(id, old)
	}
}

func (n *Node) ListPeers() []int {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var peers []int
	for id := range n.Peers {
		peers = append(peers, id)
	}

	return peers
}

func (n *Node) ListPeersHigher() []int {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var peers []int
	for id := range n.Peers {
		if id > n.SelfID {
			peers = append(peers, id)
		}
	}

	return peers
}

func (n *Node) OnPeerAdded(fn func(peerID int, peerIP string)) {
	n.onPeerAdded = append(n.onPeerAdded, fn)
}

func (n *Node) OnPeerRemoved(fn func(peerID int, peerIP string)) {
	n.onPeerRemoved = append(n.onPeerRemoved, fn)
}

func (n *Node) OnMasterSet(fn func(newID int, oldID int)) {
	n.onMasterSet = append(n.onMasterSet, fn)
}
