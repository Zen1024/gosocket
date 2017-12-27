package socket

import (
	"net"
	"sync"
	"sync/atomic"
)

type ConnPacket interface {
	Serialize() []byte
}

type ConnProtocol interface {
	ReadConnPacket(conn *net.TCPConn) (ConnPacket, error)
}

type Handler interface {
	//连接被accepted时的回调方法
	OnConnect(*Conn) bool
	//处理具体的业务逻辑
	OnMessage(*Conn, ConnPacket) bool
	//连接关闭的回调
	OnClose(*Conn)
}

type Conn struct {
	addr string
	mu   sync.Mutex
	wg   *sync.WaitGroup

	rawConn *net.TCPConn

	protocol ConnProtocol
	handler  Handler

	sendCh    chan ConnPacket
	receiveCh chan ConnPacket

	closed  int32
	closeCh chan struct{}

	extra []byte
}

func (c *Conn) GetRawConn() *net.TCPConn {
	return c.rawConn
}

func (c *Conn) SetExtra(data []byte) {
	c.mu.Lock()
	c.extra = data
	c.mu.Unlock()
}

func (c *Conn) GetExtra() []byte {
	return c.extra
}

func (c *Conn) Closed() bool {
	closed := atomic.LoadInt32(&c.closed)
	return closed == 1
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.rawConn.RemoteAddr()
}

func newConn(conn *net.TCPConn, sendLimit, receiveLimit uint) *Conn {
	return &Conn{
		rawConn:   conn,
		closeCh:   make(chan struct{}),
		sendCh:    make(chan ConnPacket, sendLimit),
		receiveCh: make(chan ConnPacket, receiveLimit),
	}
}

func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() {
		if ex := recover(); ex != nil {
			log.Printf("close error:%v\n", ex)
		}
	}()

	if !c.Closed() {
		err := c.rawConn.Close()
		atomic.StoreInt32(&c.closed, 1)
		c.handler.OnClose(c)
		close(c.closeCh)
		close(c.receiveCh)
		close(c.sendCh)
		return err
	}
	return nil

}

func (c *Conn) connect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rawConn != nil && !c.Closed() {
		log.Printf("connect on connected,addr:%s\n", c.addr)
	}

	c.closeCh = make(chan struct{})
	c.sendCh = make(chan ConnPacket, defaultWriteChanLimit)
	c.receiveCh = make(chan ConnPacket, defaultReadChanLimit)
	rawConn, err := net.DialTimeout("tcp", c.addr, defaultDialTcpTimeout)
	if err != nil {
		log.Printf("dial tcp error ,addr:%s,err:%s\n", c.addr, err.Error())
		c.Close()
		return
	}
	c.rawConn = rawConn.(*net.TCPConn)
	atomic.StoreInt32(&c.closed, 0)
	c.handler.OnConnect(c)
}
