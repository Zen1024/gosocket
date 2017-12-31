package socket

import (
	"fmt"
	"sync"
)

type HandleFunc func(*Conn, ConnPacket)

type Mux struct {
	sync.RWMutex
	m map[int32]*muxObj
}

type muxObj struct {
	Handle HandleFunc
	Name   string
	Id     int32
}

func NewMux() *Mux {
	return &Mux{
		m: make(map[int32]*muxObj),
	}
}

func (m *Mux) Add(id int32, name string, handle HandleFunc) error {
	m.Lock()
	defer m.Unlock()
	if _, ok := m.m[id]; ok {
		return fmt.Errorf("multiple regist for id:%d,name:%s\n", id, name)
	}
	m.m[id] = &muxObj{
		Id:     id,
		Name:   name,
		Handle: handle,
	}
	return nil
}

func (m *Mux) GetMuxObj(id int32) *muxObj {
	m.Lock()
	defer m.Unlock()
	if obj, ok := m.m[id]; ok {
		return obj
	}
	return nil
}
