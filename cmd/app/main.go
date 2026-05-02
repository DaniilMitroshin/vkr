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

	"github.com/jackc/pgx/v5/pgxpool"

	"vkr/internal/bot"
	"vkr/internal/config"
	"vkr/internal/httpapi"
	"vkr/internal/repository"
	"vkr/internal/service"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseDSN)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}
	if err = repository.RunMigrations(ctx, pool, "migrations"); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	store := repository.New(pool)
	svc := service.New(store)
	handler := httpapi.New(svc)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("HTTP server listening on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	if cfg.TelegramBotToken != "" {
		tgBot, err := bot.New(cfg.TelegramBotToken, svc, cfg.AdminTelegramIDs)
		if err != nil {
			log.Fatalf("telegram bot: %v", err)
		}
		go func() {
			log.Printf("Telegram bot started")
			if err := tgBot.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("telegram bot stopped: %v", err)
			}
		}()
	} else {
		log.Printf("TELEGRAM_BOT_TOKEN is empty; bot is disabled, REST API is still available")
	}

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}
