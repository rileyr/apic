package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

type wsServer struct {
	data  chan []byte
	conns map[string]chan []byte
}

func runWsServer(c *cobra.Command, args []string) error {
	s := &wsServer{
		data:  make(chan []byte, 32),
		conns: map[string]chan []byte{},
	}

	wg, ctx := errgroup.WithContext(context.Background())
	_ = ctx

	// listen for incoming websocket connections:
	wg.Go(func() error {
		addr := fmt.Sprintf(":%d", port)
		http.Handle("/ws", http.HandlerFunc(s.serve))
		return http.ListenAndServe(addr, nil)
	})

	return wg.Wait()
}

func (s *wsServer) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		slog.Default().Error(err.Error())
		return
	}
}

func (s *wsServer) handleConn() {}
