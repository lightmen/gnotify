package gnotify

import (
	"fmt"
)

//notify event
type Event struct {
	Name string
	Op   Op
}

type Op uint32

const (
	Create Op = 1 << iota
	Modify
	Delete

	AllOp = Create | Modify | Delete
)

var (
	op2String = map[Op]string{
		Create: "Create",
		Modify: "Modify",
		Delete: "Delete",
	}
)

func (op Op) String() string {
	s, ok := op2String[op]
	if ok {
		return s
	}

	return fmt.Sprintf("Unknown(%d)", op)
}
