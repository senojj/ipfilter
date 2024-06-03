package main

import (
	"context"
	"errors"
	"firehol/pkg/api"
	"firehol/pkg/badip"
	"firehol/pkg/config"
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	envLogLevel := os.Getenv("LOG_LEVEL")

	var logLevel slog.Level

	switch strings.ToLower(envLogLevel) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))

	settings, err := config.Load("./config.json")
	if err != nil {
		logger.Error(fmt.Sprintf("unable to load configuration: %e", err))
		return
	}

	loader := badip.NewGitHubLoader(settings.ArchiveURL, settings.FileSuffixList, logger)
	list := badip.NewList(1_000_000)
	found, err := loader.Load(list)
	if err != nil {
		logger.Error(fmt.Sprintf("unable to load bad ip list: %e", err))
		return
	}

	length := list.Len()
	if length != found {
		logger.Warn("number of found bad addresses was not equal to stored number", "found", found, "stored", length)
	}
	logger.Debug("bad ip list refreshed.", "found", found, "stored", length)

	r := gin.Default()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{}, 1)

	refreshDuration := time.Duration(settings.RefreshSeconds)

	go func() {
		refreshTimer := time.NewTimer(refreshDuration * time.Second)
	L:
		for {
			select {
			case <-sigs:
				break L
			case <-refreshTimer.C:
				refreshTimer.Reset(refreshDuration * time.Second)
				found, err := loader.Load(list)
				if err != nil {
					logger.Warn(fmt.Sprintf("unable to load bad ip list: %e", err))
				} else {
					logger.Debug("bad ip list refreshed.", "found", found, "stored", list.Len())
				}
			}
		}
		logger.Info("refresher shutting down")
		done <- struct{}{}
	}()

	r.Use(api.ListProvider(list))

	r.GET("health", api.Health(list, refreshDuration))
	r.GET("is-bad-ip", api.IsBadIP)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r.Handler(),
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	<-done

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = srv.Shutdown(ctx)
	if err != nil {
		log.Fatal("shutdown: ", err)
	}
	<-ctx.Done()
	logger.Info("shutting down")
}
