package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/stove/penpal/internal/db"
	"github.com/stove/penpal/internal/routing"
)

// Config holds server configuration.
type Config struct {
	ListenAddr string
	DBConnStr  string
	CityGraph  string // path to precomputed city graph
}

// Server is the penpal relay server.
type Server struct {
	cfg     Config
	db      *db.DB
	graph   *routing.Graph
	hub     *Hub
	httpSrv *http.Server
}

// New creates a new server instance.
func New(cfg Config) (*Server, error) {
	database, err := db.New(cfg.DBConnStr)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	graph, err := routing.LoadPrecomputedGraph(cfg.CityGraph)
	if err != nil {
		return nil, fmt.Errorf("loading city graph: %w", err)
	}
	log.Printf("loaded city graph: %d cities", len(graph.Cities))

	s := &Server{
		cfg:   cfg,
		db:    database,
		graph: graph,
	}
	s.hub = NewHub()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ws", s.handleWebSocket)
	mux.HandleFunc("/v1/health", s.handleHealth)

	s.httpSrv = &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s, nil
}

// Start runs the server, including the delivery loop and WebSocket hub.
func (s *Server) Start(ctx context.Context) error {
	// Run migrations
	if err := s.db.Migrate(ctx); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	log.Println("database migrations complete")

	// Start WebSocket hub
	go s.hub.Run(ctx)

	// Start delivery loop
	go s.deliveryLoop(ctx)

	// Start weekly stamp loop
	go s.weeklyStampLoop(ctx)

	log.Printf("server listening on %s", s.cfg.ListenAddr)
	if err := s.httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.httpSrv.Shutdown(ctx); err != nil {
		return err
	}
	return s.db.Close()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// deliveryLoop checks for messages ready to deliver every 30 seconds.
func (s *Server) deliveryLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			delivered, err := s.db.DeliverMessages(ctx)
			if err != nil {
				log.Printf("delivery error: %v", err)
				continue
			}
			for _, msg := range delivered {
				log.Printf("delivered message %s to user %s", msg.ID, msg.RecipientID)
				s.hub.SendToUser(msg.RecipientID, "new_delivery", map[string]any{
					"message_id":  msg.ID,
					"sender_id":   msg.SenderID,
				})
			}
		}
	}
}

// weeklyStampLoop awards weekly stamps to eligible users every hour.
func (s *Server) weeklyStampLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			users, err := s.db.GetUsersNeedingWeeklyStamp(ctx)
			if err != nil {
				log.Printf("weekly stamp error: %v", err)
				continue
			}
			for _, u := range users {
				s.awardWeeklyStamp(ctx, u.ID, u.HomeCity)
			}
			if len(users) > 0 {
				log.Printf("awarded weekly stamps to %d users", len(users))
			}
		}
	}
}

// Hub manages WebSocket connections for all connected users.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client // keyed by user ID string
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]*Client),
	}
}

func (h *Hub) Run(ctx context.Context) {
	<-ctx.Done()
}

func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client.userID.String()] = client
}

func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, client.userID.String())
}

func (h *Hub) SendToUser(userID interface{ String() string }, msgType string, payload any) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if client, ok := h.clients[userID.String()]; ok {
		client.Send(msgType, payload)
	}
}
