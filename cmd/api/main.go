package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/thomas-dev7/goflow/internal/auth"
	"github.com/thomas-dev7/goflow/internal/config"
	"github.com/thomas-dev7/goflow/internal/httpapi"
	"github.com/thomas-dev7/goflow/internal/platform"
	"github.com/thomas-dev7/goflow/internal/repository"
)

func main() {
	log,_ := zap.NewProduction()
	defer log.Sync()

	cfg,err := config.Load()
	if err != nil { log.Fatal("config",zap.Error(err)) }

	ctx,cancel := signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM)
	defer cancel()

	db,err := platform.OpenPostgres(ctx,cfg.DatabaseURL)
	if err != nil { log.Fatal("postgres",zap.Error(err)) }
	defer db.Close()

	rdb := redis.NewClient(&redis.Options{Addr:cfg.RedisAddr})
	defer rdb.Close()

	api := httpapi.New(repository.New(db),auth.New(cfg.JWTSecret,cfg.AccessTokenTTL),rdb,log)
	srv := &http.Server{
		Addr: ":"+cfg.HTTPPort,
		Handler: api.Routes(),
		ReadHeaderTimeout: 5*time.Second,
		ReadTimeout: 10*time.Second,
		WriteTimeout: 15*time.Second,
		IdleTimeout: 60*time.Second,
	}

	go func(){
		log.Info("api_started",zap.String("port",cfg.HTTPPort))
		if err:=srv.ListenAndServe(); err!=nil && !errors.Is(err,http.ErrServerClosed) { log.Fatal("http",zap.Error(err)) }
	}()

	<-ctx.Done()
	shutdown,c := context.WithTimeout(context.Background(),10*time.Second)
	defer c()
	_ = srv.Shutdown(shutdown)
	log.Info("api_stopped")
}
