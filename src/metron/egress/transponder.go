package egress

import (
	v2 "plumbing/v2"
)

type Nexter interface {
	Next() *v2.Envelope
}

type Writer interface {
	Write(msg *v2.Envelope) error
}

type Transponder struct {
	nexter Nexter
	writer Writer
}

func NewTransponder(n Nexter, w Writer) *Transponder {
	return &Transponder{
		nexter: n,
		writer: w,
	}
}

func (t *Transponder) Start() {
	for {
		envelope := t.nexter.Next()
		// TODO: emit a metric here
		t.writer.Write(envelope)
	}
}
