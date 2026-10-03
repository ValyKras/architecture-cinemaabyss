package main

import (
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type config struct {
	Port                   string
	MonolithURL            string
	MoviesServiceURL       string
	EventsServiceURL       string
	GradualMigration       bool
	MoviesMigrationPercent int
}


func loadConfig() config {
	cfg := config{
		Port:                   "8000",
		MonolithURL:            "http://localhost:8080",
		MoviesServiceURL:       "http://localhost:8081",
		EventsServiceURL:       "http://localhost:8082",
		GradualMigration:       false,
		MoviesMigrationPercent: 0,
	}

	if v := os.Getenv("PORT"); v != "" {
		cfg.Port = v
	}
	if v := os.Getenv("MONOLITH_URL"); v != "" {
		cfg.MonolithURL = v
	}
	if v := os.Getenv("MOVIES_SERVICE_URL"); v != "" {
		cfg.MoviesServiceURL = v
	}
	if v := os.Getenv("EVENTS_SERVICE_URL"); v != "" {
		cfg.EventsServiceURL = v
	}
	if v := os.Getenv("GRADUAL_MIGRATION"); v != "" {
		cfg.GradualMigration = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("MOVIES_MIGRATION_PERCENT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			log.Printf("WARNING: invalid MOVIES_MIGRATION_PERCENT %q, defaulting to 0", v)
			p = 0
		}
		if p < 0 {
			p = 0
		}
		if p > 100 {
			p = 100
		}
		cfg.MoviesMigrationPercent = p
	}

	return cfg
}


func newReverseProxy(target string) *httputil.ReverseProxy {
	targetURL, err := url.Parse(target)
	if err != nil {
		log.Fatalf("invalid target URL %q: %v", target, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)


	defaultDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		defaultDirector(req)
		log.Printf("PROXY -> %s %s", req.Method, req.URL.String())
	}

	// Return upstream errors as plain text with the original status code.
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("ERROR proxying %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
	}

	return proxy
}


func shouldRouteToMoviesService(cfg config) bool {
	if !cfg.GradualMigration {
		return false
	}
	if cfg.MoviesMigrationPercent <= 0 {
		return false
	}
	if cfg.MoviesMigrationPercent >= 100 {
		return true
	}
	return rand.Intn(100) < cfg.MoviesMigrationPercent
}


func moviesHandler(cfg config, monolithProxy, moviesProxy *httputil.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if shouldRouteToMoviesService(cfg) {
			log.Printf("MOVIES ROUTE -> movies-service (migration %d%%)", cfg.MoviesMigrationPercent)
			moviesProxy.ServeHTTP(w, r)
			return
		}
		log.Printf("MOVIES ROUTE -> monolith")
		monolithProxy.ServeHTTP(w, r)
	}
}

func main() {
	cfg := loadConfig()

	log.Printf("Starting Strangler Fig API Gateway on port %s", cfg.Port)
	log.Printf("  MONOLITH_URL=%s", cfg.MonolithURL)
	log.Printf("  MOVIES_SERVICE_URL=%s", cfg.MoviesServiceURL)
	log.Printf("  EVENTS_SERVICE_URL=%s", cfg.EventsServiceURL)
	log.Printf("  GRADUAL_MIGRATION=%v", cfg.GradualMigration)
	log.Printf("  MOVIES_MIGRATION_PERCENT=%d%%", cfg.MoviesMigrationPercent)

	monolithProxy := newReverseProxy(cfg.MonolithURL)
	moviesProxy := newReverseProxy(cfg.MoviesServiceURL)
	eventsProxy := newReverseProxy(cfg.EventsServiceURL)


	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Strangler Fig Proxy is healthy"))
	})


	http.HandleFunc("/api/movies", moviesHandler(cfg, monolithProxy, moviesProxy))
	http.HandleFunc("/api/movies/", moviesHandler(cfg, monolithProxy, moviesProxy))

	http.HandleFunc("/api/events/", func(w http.ResponseWriter, r *http.Request) {
		eventsProxy.ServeHTTP(w, r)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		monolithProxy.ServeHTTP(w, r)
	})

	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}