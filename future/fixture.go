package future

import (
	"log"
)

type Msg struct {
	Subject string
	Body    []byte
}

func Main() {
	msg := Msg{
		Subject: "Hello World",
		Body:    []byte("Hello!"),
	}

	log.Printf("Running publisher msg: %#v", msg)
}
