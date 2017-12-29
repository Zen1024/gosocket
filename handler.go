package socket

type Handler interface {
	//连接被accepted时的回调方法
	OnConnect(*Conn) bool
	//处理具体的业务逻辑
	OnMessage(*Conn, ConnPacket) bool
	//连接关闭的回调
	OnClose(*Conn)
}
