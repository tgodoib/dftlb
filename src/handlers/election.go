package handlers

import (
	"DFT_LB/codec"
	"DFT_LB/local"
	"log/slog"
	"time"
)

const (
	StateFollower uint8 = iota
	StateCandidate
	StateLeader
)

const (
	EventStart uint8 = iota
	EventElection
	EventOk
	EventMaster
	EventOkTimeout
	EventMasterTimeout
)

const (
	MsgElection string = "ELECTION"
	MsgOk       string = "OK"
	MsgMaster   string = "MASTER"
)

type Event struct {
	Type uint8
	From int
}

type Election struct {
	node          *local.Node
	okTimeout     time.Duration
	masterTimeout time.Duration

	state  uint8
	events chan Event
}

func NewElection(node *local.Node, okTimeout time.Duration, masterTimeout time.Duration) *Election {
	return &Election{
		node:          node,
		okTimeout:     okTimeout,
		masterTimeout: masterTimeout,

		state:  StateFollower,
		events: make(chan Event, 64),
	}
}

func (e *Election) HandleMsg(env codec.IncomingEnvelope) {
	//fmt.Printf("%-5s %-1d %-4s %-1d %-8s\n", "STATE", e.state, "FROM", env.From, string(env.Msg.Payload))
	slog.Debug(string(env.Msg.Payload), "STATE", e.state, "FROM", env.From)

	switch string(env.Msg.Payload) {
	case MsgElection:
		e.events <- Event{Type: EventElection, From: env.From}
	case MsgOk:
		e.events <- Event{Type: EventOk, From: env.From}
	case MsgMaster:
		e.events <- Event{Type: EventMaster, From: env.From}
	}
}

func (e *Election) LogicLoop() {
	okTimer := time.NewTimer(0)
	masterTimer := time.NewTimer(0)

	if !okTimer.Stop() {
		<-okTimer.C
	}
	if !masterTimer.Stop() {
		<-masterTimer.C
	}

	stopTimer := func(t *time.Timer) {
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
	}

	startOkTimer := func() {
		stopTimer(okTimer)
		okTimer.Reset(e.okTimeout)
	}

	startMasterTimer := func() {
		stopTimer(masterTimer)
		masterTimer.Reset(e.masterTimeout)
	}

	startElection := func() {
		for _, id := range e.node.ListPeersHigher() {
			e.send(id, MsgElection)
		}
		startOkTimer()
	}

	declareMaster := func() {
		e.node.SetMaster(e.node.SelfID)
		for _, id := range e.node.ListPeers() {
			e.send(id, MsgMaster)
		}
	}

	for {
		select {
		case ev := <-e.events:
			switch e.state {
			case StateFollower:
				switch ev.Type {
				case EventStart:
					if len(e.node.ListPeersHigher()) > 0 {
						startElection()
						e.state = StateCandidate
					} else {
						declareMaster()
						e.state = StateLeader
					}
				case EventElection:
					if e.node.SelfID > ev.From {
						e.send(ev.From, MsgOk)
						stopTimer(masterTimer)
						startElection()
						e.state = StateCandidate
					}
				case EventOk:
					// ignora
				case EventMaster:
					if e.node.SelfID > ev.From {
						stopTimer(masterTimer)
						startElection()
						e.state = StateCandidate
					} else {
						e.node.SetMaster(ev.From)
					}
				case EventOkTimeout:
					// ignora
				case EventMasterTimeout:
					// ignora
				}
			case StateCandidate:
				switch ev.Type {
				case EventStart:
					// ignore
				case EventElection:
					if e.node.SelfID > ev.From {
						e.send(ev.From, MsgOk)
					}
				case EventOk:
					stopTimer(okTimer)
					startMasterTimer()
				case EventMaster:
					stopTimer(masterTimer)
					stopTimer(okTimer)
					e.node.SetMaster(ev.From)
					e.state = StateFollower
				case EventOkTimeout:
					declareMaster()
					e.state = StateLeader
				case EventMasterTimeout:
					e.events <- Event{Type: EventStart}
					e.state = StateFollower
				}
			case StateLeader:
				switch ev.Type {
				case EventStart:
					if len(e.node.ListPeersHigher()) > 0 {
						startElection()
						e.state = StateCandidate
					} else {
						declareMaster()
					}
				case EventElection:
					if e.node.SelfID > ev.From {
						e.send(ev.From, MsgMaster)
						e.node.SetMaster(e.node.SelfID)
					} else {
						startMasterTimer()
						e.state = StateCandidate
					}
				case EventOk:
					// ignora
				case EventMaster:
					if e.node.SelfID > ev.From {
						e.send(ev.From, MsgMaster)
					} else {
						stopTimer(okTimer)
						stopTimer(masterTimer)
						e.node.SetMaster(ev.From)
						e.state = StateFollower
					}
				case EventOkTimeout:
					// ignora
				case EventMasterTimeout:
					// ignora
				}
			}
		case <-okTimer.C:
			//fmt.Printf("%-7s %-6s\n", "TIMEOUT", "OK")
			slog.Debug("Timeout OK")
			e.events <- Event{Type: EventOkTimeout}
		case <-masterTimer.C:
			//fmt.Printf("%-7s %-6s\n", "TIMEOUT", "MASTER")
			slog.Debug("Timeout MASTER")
			e.events <- Event{Type: EventMasterTimeout}
		}
	}
}

func (e *Election) send(to int, msg string) {
	//fmt.Printf("%-5s %-1d %-4s %-1d %-8s\n", "STATE", e.state, "TO", to, msg)
	slog.Debug(msg, "STATE", e.state, "TO", to)

	e.node.Bus.Send(codec.OutgoingEnvelope{
		To: to,
		Msg: codec.Message{
			Type:    codec.MsgElection,
			Payload: []byte(msg),
		},
	})
}

func (e *Election) Start() {
	//fmt.Println("ELECTION STARTED")
	slog.Info("Election started")
	e.events <- Event{Type: EventStart}
}

func (e *Election) LetKnowMaster(to int) {
	e.send(to, MsgMaster)
}
