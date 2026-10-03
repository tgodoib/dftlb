package local

import (
	"DFT_LB/codec"
)

type Bus struct {
	Incoming chan *codec.IncomingEnvelope
	Outgoing chan *codec.OutgoingEnvelope

	handlers map[codec.MessageType]Handler
}

type Handler interface {
	HandleMsg(env codec.IncomingEnvelope)
	LogicLoop()
}

func NewBus() *Bus {
	return &Bus{
		Incoming: make(chan *codec.IncomingEnvelope, 64),
		Outgoing: make(chan *codec.OutgoingEnvelope, 64),
		handlers: make(map[codec.MessageType]Handler),
	}
}

func (b *Bus) IncomingLoop() {
	for env := range b.Incoming {
		if h, ok := b.handlers[env.Msg.Type]; ok {
			h.HandleMsg(*env)
		}
	}
}

func (b *Bus) RegisterHandler(msgType codec.MessageType, h Handler) {
	b.handlers[msgType] = h

	go h.LogicLoop()
}

func (b *Bus) Receive(env codec.IncomingEnvelope) {
	b.Incoming <- &env
}

func (b *Bus) Send(env codec.OutgoingEnvelope) {
	b.Outgoing <- &env
}
