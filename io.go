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
			log.Printf("AsyncWrite:%v\n", ex)
			err = ErrConnClosed
		}
	}()

	if deadline != 0 {
		for {
			select {
			case c.sendCh <- p:
				return nil
			case <-c.closeCh:
				return ErrConnClosed
			case <-time.After(deadline):
				return ErrWriteTimeout
			}
		}
	} else {
		for {
			select {
			case c.sendCh <- p:
				return nil
			default:
				return ErrWriteBlocking
			}
		}
	}
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

func (c *Conn) read() {
	for {
		select {
		case <-c.closeCh:
			return
		default:
		}
		p, err := c.protocol.ReadConnPacket(c.rawConn)
		if err != nil {
			log.Printf("error read conn packet:%s\n", err.Error())
			return
		}
		c.receiveCh <- p
	}
}

func (c *Conn) write() {
	for {
		select {
		case <-c.closeCh:
			return
		case p, ok := <-c.sendCh:
			if c.Closed() {
				return
			}
			if !ok {
				return
			}
			if _, err := write(c.rawConn, p.Serialize()); err != nil {
				log.Printf("error write:%s\n", err.Error())
				return
			}
		}
	}
}

//server端
func (c *Conn) handle() {
	for {
		select {
		case <-c.closeCh:
			return
		case p, ok := <-c.receiveCh:
			if !ok {
				return
			}
			c.handleMsg(p)
		}
	}

}

func (c *Conn) handleMsg(p ConnPacket) {
	defer func() {
		if ex := recover(); ex != nil {
			log.Printf("error handler message:%v\n", ex)
		}
	}()

	if c.Closed() {
		return
	}

	if !c.handler.OnMessage(c, p) {
		return
	}
}

func (c *Conn) initFunc(fnc func()) {
	c.wg.Add(1)
	go func() {
		defer func() {
			if ex := recover(); ex != nil {
				log.Printf("init func exception:%v", ex)
			}
			c.Close()
			c.wg.Done()
		}()
		fnc()
	}()
}
