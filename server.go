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
	wg      *sync.WaitGroup

	readTimeout   time.Duration
	writeTimeout  time.Duration
	acceptTimeout time.Duration

	//channel长度限制
	sendLimit    uint
	receiveLimit uint
	//客户端连接数限制
	maxClient   uint
	releaseOnce sync.Once

	exitCh chan struct{}
	sem    chan struct{}

	protocol ConnProtocol
}

func (s *Server) acquire() {
	s.sem <- struct{}{}
}

func (s *Server) release() {
	s.releaseOnce.Do(func() {
		<-s.sem
	})
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
		releaseOnce:  sync.Once{},
		protocol:     protocol,
		handler:      h,
		wg:           &sync.WaitGroup{},
	}
}

func (s *Server) start(l *net.TCPListener) {
	defer func() {
		l.Close()
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

		if err != nil || rawConn == nil {
			continue
		}

		s.acquire()

		go func() {
			conn := newConn(rawConn, s.sendLimit, s.receiveLimit)
			conn.handler = s.handler
			conn.protocol = s.protocol
			s.serveConn(conn)
		}()
	}
}

func (s *Server) Stop() {
	log.Print("server stop")
	s.exitCh <- struct{}{}
}

func (s *Server) serveConn(c *Conn) {
	if ok := s.handler.OnConnect(c); !ok {
		log.Print("srv on connect fail")
		return
	}
	s.wg.Add(3)
	s.wrapLoop(c, s.readLoop)
	s.wrapLoop(c, s.writeLoop)
	s.wrapLoop(c, s.handleLoop)
	s.wg.Wait()
}

func (s *Server) readLoop(c *Conn) {
	for {
		select {
		case <-c.closeCh:
			s.release()
			return
		default:
		}
		p, err := s.protocol.ReadConnPacket(c.rawConn)
		if err != nil || p == nil {
			continue
		}
		c.receiveCh <- p
	}
}

func (s *Server) writeLoop(c *Conn) {
	for {
		select {
		case <-c.closeCh:
			s.release()
			return
		case p, ok := <-c.sendCh:
			if c.Closed() {
				return
			}
			if !ok {
				continue
			}
			if _, err := write(c.rawConn, p.Serialize()); err != nil {
				log.Printf("error write:%s\n", err.Error())
				continue
			}
		}
	}

}

func (s *Server) handleLoop(c *Conn) {
	for {
		select {
		case <-c.closeCh:
			s.release()
			return
		case p, ok := <-c.receiveCh:
			if !ok {
				continue
			}
			s.handleMsg(c, p)
		}
	}
}

func (s *Server) handleMsg(c *Conn, p ConnPacket) {
	defer func() {

		if ex := recover(); ex != nil {
			log.Printf("handleMsg:exception:%v\n", ex)
			DumpStack()
		}
	}()

	if c.Closed() {
		s.release()
		return
	}
	s.handler.OnMessage(c, p)

}

func (s *Server) wrapLoop(c *Conn, fnc func(*Conn)) {
	go func() {
		defer func() {
			if ex := recover(); ex != nil {
				log.Printf("wrapLoop exception:%v\n", ex)
				DumpStack()
			}
			c.Close()
			s.wg.Done()
		}()
		fnc(c)
	}()
}

func (s *Server) Serve() {
	addr := s.addr
	if addr == "" {
		addr = ":10000"
	}

	tcpaddr, err := net.ResolveTCPAddr("tcp", addr)
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
