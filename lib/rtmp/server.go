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

type InboundManager interface {
	NewInboundConn(ctx context.Context, c net.Conn, r *bufio.Reader, w *bufio.Writer,
		authHandler InboundAuthHandler, maxChannelNumber int) (InboundConn, error)
	OnConnectAuth(ibConn InboundConn, connectReq *Command) bool
}

type Server struct {
	listener    net.Listener
	network     string
	bindAddress string
	exit        bool
	ctx         context.Context
	cancel      context.CancelFunc
	iManager    InboundManager
}

// NewServer
func NewServer(ctx context.Context, network, bindAddress string) (*Server, error) {
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

// StartServer
func StartServer(ctx context.Context, network, bindAddress string) error {
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
	if server.listener, err = net.Listen(server.network, server.bindAddress); err != nil {
		return err
	}
	logger.Debugf("rtmp server started on %s...", bindAddress)

	server.iManager = defaultInManager

	return server.mainLoop()
}

func (s *Server) Ctx() context.Context {
	return s.ctx
}

func (s *Server) mainLoop() (err error) {
	for {
		select {
		case <-s.ctx.Done():
			logger.Debugf("server loop break, err: %s", s.ctx.Err().Error())
			return s.ctx.Err()
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
	var err error
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
			logger.Errorf("Server Handshake err: %s", err.Error())
		}
	}()
	br := bufio.NewReader(c)
	bw := bufio.NewWriter(c)
	timeout := time.Duration(10) * time.Second
	if err = SHandshake(c, br, bw, timeout); err != nil {
		logger.Errorf("SHandshake err: %s", err.Error())
		c.Close()
		return
	}

	// if s.iManager != nil {
	// _, err = s.iManager.NewInboundConn(s.ctx, c, br, bw, s.iManager, 100)
	// } else {
	_, err = NewInboundConn(s.ctx, c, br, bw, s, 100)
	// }
	if err != nil {
		logger.Debugf("%+v", errors.Wrap(err, "NewInboundConn"))
		c.Close()
	}
}
