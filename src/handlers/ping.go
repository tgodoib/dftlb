package handlers

import (
	"DFT_LB/codec"
	"DFT_LB/local"
)

type Ping struct {
	bus *local.Bus
}

func NewPing(bus *local.Bus) *Ping {
	return &Ping{bus: bus}
}

func (p *Ping) HandleMsg(env codec.IncomingEnvelope) {
	if string(env.Msg.Payload) == "ping" {
		p.bus.Send(codec.OutgoingEnvelope{
			To: env.From,
			Msg: codec.Message{
				Type:    codec.MsgPing,
				Payload: []byte("pong"),
			},
		})
	}
}

func (p *Ping) LogicLoop() {}
