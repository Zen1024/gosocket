package socket

import (
	"net"
	"time"
)

//异步写,不需要等待返回
func (c *Conn) AsyncWrite(p ConnPacket, deadline time.Duration) error {
	if c.Closed() {
		return ErrConnClosed
	}
	var err error
	defer func() {
		if ex := recover(); ex != nil {
			log.Printf("AsyncWrite:exception:%v\n", ex)
			DumpStack()
			err = ErrConnClosed
		}
	}()

	if deadline != 0 {
		select {
		case <-c.closeCh:
			return ErrConnClosed
		case <-time.After(deadline):
			return ErrWriteTimeout
		case c.sendCh <- p:
			return nil
		default:
		}

	} else {
		select {
		case <-c.closeCh:
			return ErrConnClosed
		case c.sendCh <- p:
			return nil
		default:
		}
	}
	return nil
}

//同步写
func (c *Conn) Write(p ConnPacket, deadline time.Duration) (ConnPacket, error) {
	if err := c.AsyncWrite(p, deadline); err != nil {
		return nil, err
	}
	re, err := c.readPacket(defaultReadTimeout)
	if err != nil {
		return nil, err
	}
	return re, nil
}

func (c *Conn) readPacket(timeout time.Duration) (ConnPacket, error) {
	if c.Closed() {
		return nil, ErrConnClosed
	}

	select {
	case p := <-c.receiveCh:
		return p, nil
	case <-c.closeCh:
		return nil, ErrConnClosed
	case <-time.After(timeout):
		return nil, ErrReadTimeout
	}
}

func write(conn *net.TCPConn, buf []byte) (int, error) {
	lb := len(buf)
	var total, n int
	var err error

	for total < lb && err == nil {
		n, err = conn.Write(buf[total:])
		total += n
	}

	if total >= lb {
		err = nil
	}
	return total, err
}
