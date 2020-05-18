package rtmp

import (
	"bufio"
	"context"
	"net"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"

	"ken/service"
)

type Server struct {
	listener    net.Listener
	network     string
	bindAddress string
	exit        bool
	ctx         context.Context
	cancel      context.CancelFunc

	// service handler
	shandler service.ServiceHandler
}

// StartServer
func StartServer(network, bindAddress string, shandler service.ServiceHandler) error {
	InitLog(zapcore.DebugLevel)
	server := &Server{
		network:     network,
		bindAddress: bindAddress,
		exit:        false,
		shandler:    shandler,
	}

	server.ctx, server.cancel = context.WithCancel(shandler.Ctx())
	var err error
	if server.listener, err = net.Listen(server.network, server.bindAddress); err != nil {
		return err
	}
	logger.Debugf("rtmp server started on %s...", bindAddress)

	return server.mainLoop()
}

func (s *Server) Ctx() context.Context {
	return s.ctx
}

func (s *Server) mainLoop() (err error) {
	for {
		c, err := s.listener.Accept()
		if s.ctx.Err() != nil {
			return s.ctx.Err()
		}
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
			logger.Errorf("Server Handshake err: %+v", errors.Wrap(err, "panic"))
		}
	}()
	br := bufio.NewReader(c)
	bw := bufio.NewWriter(c)
	timeout := time.Duration(10) * time.Second
	if err = SHandshake(c, br, bw, timeout); err != nil {
		logger.Errorf("%+v", errors.Wrap(err, "SHandshake"))
		c.Close()
		return
	}

	_, err = NewInboundConn(c, br, bw, s.shandler, 100)
	if err != nil {
		logger.Debugf("%+v", errors.Wrap(err, "NewInboundConn"))
		c.Close()
	}
}
