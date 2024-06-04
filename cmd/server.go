package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"ipfilter/pkg/api"
	"ipfilter/pkg/config"
	"ipfilter/pkg/iplist"
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

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{}, 1)

	settings, err := config.Load("./config.json")
	if err != nil {
		logger.Error(fmt.Sprintf("unable to load configuration: %e", err))
		return
	}

	loader := iplist.NewGitHubLoader(
		settings.ArchiveURL,
		settings.MaxDownloadBytes,
		settings.FileSuffixList,
		logger,
	)
	list := iplist.NewList(1_000_000)

	refreshDuration := time.Duration(settings.RefreshSeconds)

	healthHandle := api.Health(list, refreshDuration)

	_, err = loader.Load(list)
	if err != nil {
		if errors.Is(err, iplist.UnchangedVersion) {
			// This is the first time the List was loaded, so the version
			// must be different. The problem may rectify itself on a following
			// refresh, but a warning should be raised.
			logger.Warn("bad ip list version unchanged", "version", list.Version)
		} else {
			logger.Error(fmt.Sprintf("unable to load bad ip list: %e", err))
			return
		}
	}

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
				if err != nil && !errors.Is(err, iplist.UnchangedVersion) {
					logger.Warn(fmt.Sprintf("unable to load bad ip list: %e", err))
				} else {
					logger.Debug("bad ip list refreshed.", "new", found, "stored", list.Len())
				}
			}
		}
		logger.Info("refresher shutting down")
		done <- struct{}{}
	}()

	r := gin.Default()

	r.Use(api.ListProvider(list))

	r.GET("health", healthHandle)
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
