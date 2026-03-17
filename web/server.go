package web

import (
	"context"
	"log/slog"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Server struct {
	router *chi.Mux
}

func NewWebServer(
	router *chi.Mux,
	sensors *SensorsController,
	devices *DevicesController,
) *Server {
	router.Get("/api/sensors", sensors.Get)
	router.Get("/api/devices", devices.Get)

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
