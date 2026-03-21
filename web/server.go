package web

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Server struct {
	router *chi.Mux
	ws     *WebSocketServer
}

func NewWebServer(
	router *chi.Mux,
	ws *WebSocketServer,
	sensors *SensorsController,
	devices *DevicesController,
	auth *AuthController,
) *Server {
	router.Get("/api/sensors", sensors.Get)
	router.Get("/api/devices", devices.Get)
	router.Get("/api/ws", ws.handleConnections)
	router.Post("/api/auth/login", auth.Login)
	router.Options(
		"/api/auth/login",
		func(w http.ResponseWriter, r *http.Request) {

			fmt.Println("123")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Accept")
			w.Header().Set("Access-Control-Allow-Origin", "*")
		},
	)

	return &Server{
		router: router,
	}
}

func (c *Server) Start(host string, ctx context.Context) error {
	server := &http.Server{
		Addr:    host,
		Handler: c.router,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}
	slog.Info("Listening on " + host)
	err := server.ListenAndServe()

	return err
}
