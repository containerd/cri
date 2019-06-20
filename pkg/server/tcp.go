package server

import (
	"net"

	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type tcpServer struct {
	l   net.Listener
	rpc *grpc.Server
}

func newTCPServer(c *criService, endpoint string) (*tcpServer, error) {
	l, err := net.Listen("tcp", endpoint)
	if err != nil {
		return nil, err
	}

	serverOpts := []grpc.ServerOption{
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
	}
	rpc := grpc.NewServer(serverOpts...)
	if err := c.Register(rpc); err != nil {
		return nil, errors.Wrap(err, "registering CRI on TCP server")
	}
	return &tcpServer{
		l:   l,
		rpc: rpc,
	}, nil
}

func (s *tcpServer) start() error {
	return s.rpc.Serve(s.l)
}

func (s *tcpServer) stop() {
	s.rpc.GracefulStop()
}
