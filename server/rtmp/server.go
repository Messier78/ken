package rtmp

import (
	"bufio"
	"context"
	"net"
	"time"

	"github.com/pkg/errors"
)

type ServerHandler interface {
	NewConnection(conn InboundConn, connectReq *Command, server *Server) bool
}

type Server struct {
	listener    net.Listener
	network     string
	bindAddress string
	exit        bool
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewServer
func NewServer(network, bindAddress string, ctx context.Context) (*Server, error) {
	server := &Server{
		network:     network,
		bindAddress: bindAddress,
		exit:        false,
	}
	if ctx != nil {
		server.ctx, server.cancel = context.WithCancel(ctx)
	} else {
		server.ctx, server.cancel = context.WithCancel(context.Background())
	}
	var err error
	server.listener, err = net.Listen(server.network, server.bindAddress)
	if err != nil {
		return nil, err
	}

	go server.mainLoop()
	return server, nil
}

func (s *Server) mainLoop() {
	for {
		select {
		case <-s.ctx.Done():
			logger.Debugf("%+v", errors.Wrap(s.ctx.Err(), "server loop break"))
		default:
		}
		c, err := s.listener.Accept()
		if err != nil {
			s.rebind()
		}
		if c != nil {
			go s.Handshake(c)
		}
	}
}

func (s *Server) rebind() {
	listener, err := net.Listen(s.network, s.bindAddress)
	if err == nil {
		s.listener = listener
	} else {
		time.Sleep(time.Second)
	}
}

func (s *Server) Handshake(c net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			err := r.(error)
			logger.Errorf("Server Handshake err: %s", err.Error())
		}
	}()
	br := bufio.NewReader(c)
	bw := bufio.NewWriter(c)
	timeout := time.Duration(10) * time.Second
	if err := SHandshake(c, br, bw, timeout); err != nil {
		logger.Errorf("SHandshake err: %s", err.Error())
		c.Close()
		return
	}

	if _, err := NewInboundConn(c, br, bw, s, 100); err != nil {
		logger.Debugf("%+v", errors.Wrap(err, "NewInboundConn"))
		c.Close()
	}
}

func (s *Server) OnConnectAuth(conn InboundConn, req *Command) bool {
	return false
}