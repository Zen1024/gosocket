package socket

import (
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Server struct {
	addr    string
	handler Handler

	wg *sync.WaitGroup

	readTimeout   time.Duration
	writeTimeout  time.Duration
	acceptTimeout time.Duration

	//channel长度限制
	sendLimit    uint
	receiveLimit uint
	//客户端连接数限制
	maxClient uint

	exitCh chan struct{}
	sem    chan struct{}

	protocol ConnProtocol
}

func (s *Server) acquire() {
	s.sem <- struct{}{}
}

func (s *Server) release() {
	<-s.sem
}

func NewServer(addr string, maxClient uint, h Handler, protocol ConnProtocol, sendLimit, receiveLimit uint) *Server {
	MaxClient := maxClient
	SendLimit := sendLimit
	ReceiveLimit := receiveLimit

	if maxClient == 0 {
		MaxClient = defaultMaxClient
	}
	if sendLimit == 0 {
		SendLimit = defaultWriteChanLimit
	}

	if receiveLimit == 0 {
		ReceiveLimit = defaultReadChanLimit
	}

	return &Server{
		addr:         addr,
		maxClient:    MaxClient,
		sem:          make(chan struct{}, maxClient),
		exitCh:       make(chan struct{}),
		sendLimit:    SendLimit,
		receiveLimit: ReceiveLimit,
		wg:           &sync.WaitGroup{},
		protocol:     protocol,
	}
}

func (s *Server) start(l *net.TCPListener) {
	s.wg.Add(1)
	defer func() {
		l.Close()
		s.wg.Done()
	}()

	for {
		select {
		case <-s.exitCh:
			log.Print("server received exit chan")
			return
		default:
		}

		if s.acceptTimeout == 0 {
			s.acceptTimeout = defaultAcceptTimeout
		}

		l.SetDeadline(time.Now().Add(s.acceptTimeout))

		rawConn, err := l.AcceptTCP()
		if err != nil {
			continue
		}

		s.acquire()

		go func() {
			conn := newConn(rawConn, s.sendLimit, s.receiveLimit)
			s.serveConn(conn)
		}()
	}
}

func (s *Server) Stop() {
	log.Print("server stop")
	s.exitCh <- struct{}{}
	s.wg.Wait()
}

func (s *Server) serveConn(c *Conn) {
	if ok := s.handler.OnConnect(c); !ok {
		log.Print("srv on connect fail")
		return
	}
	s.wrapLoop(c, s.readLoop)
	s.wrapLoop(c, s.writeLoop)
	s.wrapLoop(c, s.handleLoop)
	c.wg.Wait()
}

func (s *Server) readLoop(c *Conn) {
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

func (s *Server) writeLoop(c *Conn) {
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

func (s *Server) handleLoop(c *Conn) {
	for {
		select {
		case <-c.closeCh:
			return
		case p, ok := <-c.receiveCh:
			if !ok {
				return
			}
			s.handleMsg(c, p)
		}
	}
}

func (s *Server) handleMsg(c *Conn, p ConnPacket) {
	defer func() {
		if ex := recover(); ex != nil {
			log.Printf("handler message exception:%v\n", ex)
		}
	}()

	if c.Closed() {
		return
	}
	s.handler.OnMessage(c, p)

}

func (s *Server) wrapLoop(c *Conn, fnc func(*Conn)) {
	c.wg.Add(1)
	go func() {
		defer func() {
			if ex := recover(); ex != nil {
				log.Printf("wrap func exception:%v\n", ex)
			}
			c.Close()
			c.wg.Done()
		}()
		fnc(c)
	}()
}

func (s *Server) Serve() {
	addr := s.addr
	if addr == "" {
		addr = ":10000"
	}

	tcpaddr, err := net.ResolveTCPAddr("tcp4", addr)
	if err != nil {
		log.Printf("resolve tcp addr error:%s\n", err.Error())
		return
	}

	l, err := net.ListenTCP("tcp", tcpaddr)
	if err != nil {
		log.Printf("listen tcp error:%s\n", err.Error())
		return
	}

	go s.start(l)

	chSig := make(chan os.Signal)
	signal.Notify(chSig, syscall.SIGINT, syscall.SIGTERM)
	sig := <-chSig
	log.Printf("receive signal:%v\n", sig)
	s.Stop()
}
