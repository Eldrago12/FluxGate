package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/Eldrago12/FluxGate/internal/api"
	"github.com/Eldrago12/FluxGate/internal/config"
	"github.com/Eldrago12/FluxGate/internal/limiter"
	"github.com/Eldrago12/FluxGate/pkg/shutdown"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	apiToLimit := flag.String("api", "", "API URL to rate limit")
	flag.Parse()

	if *apiToLimit == "" {
		log.Fatal("API URL to rate limit must be provided")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	distributedLimiter, err := limiter.NewDistributedLimiter(cfg.GCPProjectID, cfg.GCPRegion, cfg.RedisName, cfg.Rate, cfg.BucketSize)
	if err != nil {
		log.Fatalf("Failed to create distributed limiter: %v", err)
	}
	defer distributedLimiter.Close()

	dynamicLimiter := limiter.NewDynamicLimiter(cfg.Rate, cfg.BucketSize)
	dynamicLimiter.Start(1 * time.Minute)

	handler := api.NewHandler(distributedLimiter, dynamicLimiter, *apiToLimit)

	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: handler,
	}

	go func() {
		log.Printf("Server starting on %s, rate limiting API: %s", cfg.ListenAddr, *apiToLimit)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	shutdown.Graceful(server, 30*time.Second)
}
