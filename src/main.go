package main

import (
	"DFT_LB/codec"
	"DFT_LB/config"
	"DFT_LB/handlers"
	"DFT_LB/local"
	"DFT_LB/transport"
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := config.Load("network.yaml")
	if err != nil {
		log.Fatal(err)
	}

	handler := config.NewPrettyLogger(os.Stdout, slog.LevelDebug)
	slog.SetDefault(slog.New(handler))

	lb := local.NewLoadBalancer(cfg.LB)
	bus := local.NewBus()
	node := local.NewNode(cfg.Self, bus)

	ping := handlers.NewPing(bus)
	election := handlers.NewElection(node, 500*time.Millisecond, 1000*time.Millisecond)
	heartbeat := handlers.NewHeartbeat(node, time.Second*1, time.Second*3)

	bus.RegisterHandler(codec.MsgPing, ping)
	bus.RegisterHandler(codec.MsgElection, election)
	bus.RegisterHandler(codec.MsgHeartbeat, heartbeat)

	lb.ReleaseMaster()
	lb.ReleaseFollower()

	node.OnPeerAdded(func(peerID int, peerIP string) {
		slog.Debug("OnPeerAdded", "peerID", peerID, "peerIP", peerIP)

		if node.SelfID == node.MasterID {
			lb.Enqueue(func() {
				lb.AddBackend(peerIP)
			})

			if peerID > node.SelfID {
				election.Start()
			} else {
				election.LetKnowMaster(peerID)
			}
		}
	})

	node.OnPeerRemoved(func(peerID int, peerIP string) {
		slog.Debug("OnPeerRemoved", "peerID", peerID, "peerIP", peerIP)

		heartbeat.RemoveTimer(peerID)

		if node.SelfID == node.MasterID {
			lb.Enqueue(func() {
				lb.RemoveBackend(peerIP)
			})
		}

		slog.Debug("Current master", "id", node.MasterID)
		if peerID == node.MasterID {
			slog.Debug("Master is down, starting election")
			election.Start()
		}
	})

	node.OnMasterSet(func(newID int, oldID int) {
		slog.Debug("OnMasterSet", "oldID", oldID, "newID", newID)

		if newID == oldID {
			if node.SelfID == newID {
				lb.Enqueue(lb.AcquireMaster)

				for _, p := range node.Peers {
					lb.Enqueue(func() {
						lb.AddBackend(p.Conn.RemoteAddr().String())
					})
				}
			}
		} else {
			if newID == node.SelfID {
				lb.Enqueue(lb.ReleaseFollower)
				lb.Enqueue(lb.AcquireMaster)

				for _, p := range node.Peers {
					lb.Enqueue(func() {
						lb.AddBackend(p.Conn.RemoteAddr().String())
					})
				}
			}

			if oldID == node.SelfID {
				lb.Enqueue(lb.ReleaseMaster)
				lb.Enqueue(lb.AcquireFollower)
			}

			if oldID == -1 && newID != node.SelfID {
				lb.Enqueue(lb.AcquireFollower)
			}
		}
	})

	go bus.IncomingLoop()
	go node.OutgoingLoop()

	listenToPeers(cfg, node)
	connectToPeers(cfg, node)

	go func() {
		time.Sleep(5*time.Second + time.Duration(len(cfg.Nodes)-cfg.Self)*2*time.Second)
		election.Start()
	}()

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	<-ctx.Done()

	lb.ReleaseMaster()
	lb.ReleaseFollower()
}

func listenToPeers(cfg *config.Config, node *local.Node) {
	addr := ":" + cfg.ListenPortString()
	transport.StartServer(addr, func(c *transport.Peer) {
		// TODO: Substituir isso por handshake
		id, err := cfg.IDFromIP(c.Conn.RemoteAddr().String())
		if err != nil {
			log.Println(err)
			return
		}
		node.AddPeer(id, c)
	})

}

func connectToPeers(cfg *config.Config, node *local.Node) {

	connectLoop := func(peerID int, ip string) {
		addr := fmt.Sprintf("%s:%d", ip, cfg.ListenPort)

		for {
			conn, err := transport.ConnectToServer(addr)
			if err != nil {
				//log.Printf("erro conectando com %d: %v", peerID, err)
				time.Sleep(5 * time.Second)
				continue
			}

			node.AddPeer(peerID, conn)

			<-conn.Done

			slog.Info("Removida conexão com peer", "id", peerID)

			time.Sleep(5 * time.Second)
		}
	}

	for _, peer := range cfg.Nodes {
		if peer.ID == cfg.Self || peer.ID < cfg.Self {
			continue
		}
		go connectLoop(peer.ID, peer.IP)
	}

}
