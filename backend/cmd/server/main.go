package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alshatilov/dengovod/backend/internal/repository/sqlite"
	"github.com/alshatilov/dengovod/backend/internal/service"
	httptransport "github.com/alshatilov/dengovod/backend/internal/transport/http"
)

func main() {
	store, err := sqlite.Open(os.Getenv("DATABASE_PATH"))
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = ":8080"
	}
	server := &http.Server{
		Addr: address, Handler: httptransport.NewRouter(service.New(store)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("dengovod backend listening on %s", address)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
