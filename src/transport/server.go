package transport

import (
	"log"
	"log/slog"
	"net"
)

func StartServer(addr string, onConn func(*Peer)) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Erro ao iniciar servidor: %v", err)
	}

	slog.Info("Server listening on " + addr)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				slog.Error("Accepting connection", err)
				continue
			}
			onConn(NewPeer(conn))
		}
	}()
}

func ConnectToServer(addr string) (*Peer, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return NewPeer(conn), nil
}
