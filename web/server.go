package web

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"smart-home/config"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/jwtauth/v5"
)

type Server struct {
	cfg    *config.WebConfig
	router *chi.Mux
	ws     *WebSocketServer
}

func NewWebServer(
	cfg *config.WebConfig,
	tokenAuth *jwtauth.JWTAuth,
	ws *WebSocketServer,
	sensors *SensorsController,
	daily *SensorsDailyController,
	configurable *SensorsConfigurableController,
	devices *DevicesController,
	control *DeviceControlController,
	auth *AuthController,
) *Server {
	r := chi.NewMux()

	// Cors for all
	if cfg.Cors.Allowed {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{cfg.Cors.Host},
			AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "Accept"},
			AllowCredentials: false,
		}))
	}

	// Protected classic routes
	r.Group(func(r chi.Router) {
		verifier := func(ja *jwtauth.JWTAuth) func(http.Handler) http.Handler {
			return jwtauth.Verify(ja, jwtauth.TokenFromHeader)
		}(tokenAuth)
		// Auth
		r.Use(authLogMiddleware)
		r.Use(verifier)
		r.Use(jwtauth.Authenticator(tokenAuth))

		// Routing
		r.Get("/api/sensors", sensors.Get)
		r.Get("/api/devices/{deviceId}/sensors/daily", daily.Get)
		r.Get("/api/devices/{deviceId}/sensors/{duration}/{scale}", configurable.Get)
		r.Get("/api/devices/{deviceId}/control", control.Get)
		r.Post("/api/devices/{deviceId}/control", control.Do)
		r.Get("/api/devices", devices.GetAll)
		r.Get("/api/devices/{deviceId}", devices.Get)
		r.Get("/api/auth/verify", auth.Verify)
	})

	// Protected WS route
	r.Group(func(r chi.Router) {
		verifier := func(ja *jwtauth.JWTAuth) func(http.Handler) http.Handler {
			return jwtauth.Verify(ja, jwtauth.TokenFromQuery)
		}(tokenAuth)

		// Auth
		r.Use(authLogMiddleware)
		r.Use(verifier)
		r.Use(jwtauth.Authenticator(tokenAuth))

		// Routing
		r.Get("/api/ws", ws.handleConnections)
	})

	// Public routes
	// Routing
	r.Post("/api/auth/login", auth.Login)

	return &Server{
		cfg:    cfg,
		router: r,
	}
}

func (s *Server) Start(ctx context.Context) error {

	server := &http.Server{
		Addr:    s.cfg.Host,
		Handler: s.router,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}
	slog.Info("Listening on " + server.Addr)
	err := server.ListenAndServe()

	return err
}

func (s *Server) corsMiddleware(next http.Handler, cfg *config.CorsConfig) http.Handler {
	if !cfg.Allowed {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", cfg.Host)
		next.ServeHTTP(w, r)
	})
}

func authLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(wrapped, r)

		if wrapped.Status() == http.StatusOK {
			return
		}

		// WS is handled in the WS module
		if wrapped.Status() == 0 {
			if r.URL.Path == "/api/ws" {
				return
			}
		}

		slog.Warn(
			"Failed request",
			"Status",
			wrapped.Status(),
			"URL",
			r.Method+" "+r.URL.Path,
			"Authorization",
			r.Header.Get("Authorization"),
			"Sec-WebSocket-Protocol",
			r.Header.Get("Sec-WebSocket-Protocol"),
		)
	})
}
