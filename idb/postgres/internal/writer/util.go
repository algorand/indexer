package writer

import (
	"github.com/jackc/pgx/v4"
)

// Implements pgx.CopyFromSource.
type copyFromChannelStruct struct {
	ch   chan []interface{}
	next []interface{}
}

func (c *copyFromChannelStruct) Next() bool {
	var ok bool
	c.next, ok = <-c.ch
	return ok
}

func (c *copyFromChannelStruct) Values() ([]interface{}, error) {
	return c.next, nil
}

func (c *copyFromChannelStruct) Err() error {
	return nil
}

func copyFromChannel(ch chan []interface{}) pgx.CopyFromSource {
	return &copyFromChannelStruct{ch: ch}
}
