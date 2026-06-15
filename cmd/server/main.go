package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KurisuNo1/InterviewAgent/config"
	"github.com/KurisuNo1/InterviewAgent/internal/app"
)

func main() {
	configPath := "config/config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	config.LoadEnv(".env")
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Wire all dependencies: L3 capabilities → L2 orchestration → L1 interaction
	application, err := app.Wire(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}
	defer application.Close()

	// Set up HTTP routes
	mux := http.NewServeMux()

	// Static web frontend (no-cache for dev)
	webFS := http.FileServer(http.Dir("web"))
	noCacheFS := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		webFS.ServeHTTP(w, r)
	})
	mux.Handle("/web/", http.StripPrefix("/web/", noCacheFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			http.ServeFile(w, r, "web/index.html")
			return
		}
		http.NotFound(w, r)
	})

	// REST API
	mux.Handle("/api/", application.RESTRouter)
	mux.Handle("/api", application.RESTRouter)

	// WebSocket
	mux.HandleFunc(cfg.Server.WSPath, application.WSHub.ServeWS)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		fmt.Printf("╔══════════════════════════════════════════╗\n")
		fmt.Printf("║   InterviewAgent Server                  ║\n")
		fmt.Printf("╠══════════════════════════════════════════╣\n")
		fmt.Printf("║  REST API:  http://%s/api    ║\n", addr)
		fmt.Printf("║  WebSocket: ws://%s%s       ║\n", addr, cfg.Server.WSPath)
		fmt.Printf("║  Health:    http://%s/health ║\n", addr)
		fmt.Printf("║  LLM:       %-28s ║\n", cfg.LLM.Model)
		fmt.Printf("║  Web     :  http://%s        ║\n", addr)
		fmt.Printf("╚══════════════════════════════════════════╝\n")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nServer shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	fmt.Println("Server exited")
}
