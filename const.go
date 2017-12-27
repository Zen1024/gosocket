package socket

import (
	"errors"
	"time"
)

var (
	ErrConnClosed    = errors.New("conn closed")
	ErrWriteTimeout  = errors.New("write timeout")
	ErrReadTimeout   = errors.New("read timeout")
	ErrWriteBlocking = errors.New("write blocking")
	ErrReadBlocking  = errors.New("read blocking")
)

const (
	defaultReadChanLimit  = 1000000
	defaultWriteChanLimit = 1000000
	defaultDialTcpTimeout = time.Second
	defaultAcceptTimeout  = time.Second
	defaultMaxClient      = 100000
	defaultReadTimeout    = time.Second
)
