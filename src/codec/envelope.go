package codec

type IncomingEnvelope struct {
	From int
	Msg  Message
}

type OutgoingEnvelope struct {
	To  int
	Msg Message
}
