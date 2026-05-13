package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/helloamj/keyradb/internal/db"
)

func main() {
	addr := flag.String("addr", ":6380", "HTTP listen address")
	dataDir := flag.String("data", "./data", "Directory for database files")
	memMB := flag.Int64("mem-mb", 4, "Memtable size threshold in MiB before flush")
	flag.Parse()

	opts := db.DefaultOptions()
	opts.MemtableMaxBytes = *memMB * 1024 * 1024

	store, err := db.Open(*dataDir, opts)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	log.Printf("keyradb listening on %s  (data=%s, memtable=%d MiB)", *addr, *dataDir, *memMB)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /keys/{key}", handleGet(store))
	mux.HandleFunc("PUT /keys/{key}", handlePut(store))
	mux.HandleFunc("DELETE /keys/{key}", handleDelete(store))
	mux.HandleFunc("POST /flush", handleFlush(store))
	mux.HandleFunc("GET /health", handleHealth())

	srv := &http.Server{Addr: *addr, Handler: mux}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("shutting down...")
		srv.Close()
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
}

func handleGet(store *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		if key == "" {
			httpError(w, "key is required", http.StatusBadRequest)
			return
		}

		val, err := store.Get([]byte(key))
		if errors.Is(err, db.ErrKeyNotFound) {
			httpError(w, fmt.Sprintf("key %q not found", key), http.StatusNotFound)
			return
		}
		if err != nil {
			httpError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"key":   key,
			"value": string(val),
		})
	}
}

func handlePut(store *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		if key == "" {
			httpError(w, "key is required", http.StatusBadRequest)
			return
		}

		var body struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpError(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := store.Put([]byte(key), []byte(body.Value)); err != nil {
			httpError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"key":    key,
			"status": "ok",
		})
	}
}

func handleDelete(store *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		if key == "" {
			httpError(w, "key is required", http.StatusBadRequest)
			return
		}

		if err := store.Delete([]byte(key)); err != nil {
			httpError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"key":    key,
			"status": "deleted",
		})
	}
}

func handleFlush(store *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.Flush(); err != nil {
			httpError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "flushed"})
	}
}

func handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: encode error: %v", err)
	}
}

func httpError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		log.Printf("httpError: encode error: %v", err)
	}
}
