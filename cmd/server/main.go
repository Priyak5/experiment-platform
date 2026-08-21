package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/zomato/cohort-service/internal/config"
	"github.com/zomato/cohort-service/internal/controller"
	pgrepo "github.com/zomato/cohort-service/internal/repository/postgres"
	rdsrepo "github.com/zomato/cohort-service/internal/repository/redis"
	"github.com/zomato/cohort-service/internal/service"
	"github.com/zomato/cohort-service/internal/worker"
)

func main() {
	migrateOnly := flag.Bool("migrate", false, "run migrations and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// --- Postgres ---
	pgPool, err := pgrepo.NewPool(rootCtx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pgPool.Close()

	if err := pgrepo.Migrate(rootCtx, pgPool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if *migrateOnly {
		log.Println("migrations applied; exiting")
		return
	}

	// Reader pool: same DSN in POC, but with a read-only role in prod.
	readerPool, err := pgrepo.NewPool(rootCtx, cfg.PostgresReaderDSN)
	if err != nil {
		log.Fatalf("postgres reader: %v", err)
	}
	defer readerPool.Close()

	// --- Redis ---
	rds, err := rdsrepo.NewClient(rootCtx, cfg.RedisAddr)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer func() { _ = rds.Close() }()

	streamRepo, err := rdsrepo.NewStreamRepo(rootCtx, rds, cfg.RefreshStream, cfg.RefreshConsumerGroup)
	if err != nil {
		log.Fatalf("stream repo: %v", err)
	}

	// --- Repos + services ---
	cohortRepo := pgrepo.NewCohortRepo(pgPool)
	userRepo := pgrepo.NewUserRepo(readerPool, cfg.SQLStatementTimeout)
	memRepo := rdsrepo.NewMembershipRepo(rds)

	cohortSvc := service.NewCohortService(cohortRepo, memRepo, streamRepo)
	lookupSvc := service.NewLookupService(memRepo)
	refreshSvc := service.NewRefreshService(cohortRepo, userRepo, memRepo)

	// --- HTTP ---
	ctrl := controller.New(cohortSvc, lookupSvc)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           controller.NewRouter(ctrl),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// --- Worker ---
	refresher := worker.NewRefresher(streamRepo, refreshSvc, cfg.RefreshConsumerName, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		refresher.Run(rootCtx)
	}()

	go func() {
		log.Printf("server listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http server: %v", err)
			rootCancel()
		}
	}()

	// --- Signal handling & graceful shutdown ---
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	rootCancel() // stops the worker's Consume loop
	wg.Wait()
	log.Println("clean exit")
}
