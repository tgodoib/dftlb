package transport

import (
	"DFT_LB/codec"
	"errors"
	"io"
	"log/slog"
	"net"
	"time"
)

type Peer struct {
	Conn     net.Conn
	Incoming chan *codec.Message
	Outgoing chan *codec.Message
	Done     chan struct{}
}

func NewPeer(conn net.Conn) *Peer {
	p := &Peer{
		Conn:     conn,
		Incoming: make(chan *codec.Message, 64),
		Outgoing: make(chan *codec.Message, 64),
		Done:     make(chan struct{}),
	}

	go p.readLoop()
	go p.writeLoop()

	return p
}

func (p *Peer) readLoop() {
	defer func() {
		close(p.Incoming)
		close(p.Done)
		p.Conn.Close()
	}()

	for {
		p.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		msg, err := codec.Decode(p.Conn)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) && err != io.EOF {
				slog.Error("Decoding msg" + err.Error())
			}
			return
		}
		p.Incoming <- msg
	}
}

func (p *Peer) writeLoop() {
	for {
		select {
		case msg, ok := <-p.Outgoing:
			if !ok {
				return
			}
			data, err := codec.Encode(*msg)
			if err != nil {
				slog.Error("Encoding msg" + err.Error())
				return
			}
			p.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_, err = p.Conn.Write(data)
			if err != nil {
				slog.Error("Writing msg" + err.Error())
				return
			}
		case <-p.Done:
			return
		}
	}
}
