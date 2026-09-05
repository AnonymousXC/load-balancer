package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

func startServer(port string, serverID string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{"status":"healthy","port":"%s","server_id":"%s"}`, port, serverID)))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if s := r.URL.Query().Get("sleep"); s != "" {
			if d, err := time.ParseDuration(s + "s"); err == nil {
				time.Sleep(d)
			}
		}

		if codeStr := r.URL.Query().Get("status"); codeStr != "" {
			if code, err := strconv.Atoi(codeStr); err == nil && code >= 100 && code < 600 {
				w.WriteHeader(code)
				w.Write([]byte(fmt.Sprintf(`{"error":"simulated status","code":%d,"server_id":"%s"}`, code, serverID)))
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{"message":"Hello from backend","port":"%s","server_id":"%s","time":"%s"}`, port, serverID, time.Now().Format(time.RFC3339Nano))))
	})

	fmt.Printf("Backend %s listening on :%s\n", serverID, port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		fmt.Printf("Server %s on port %s stopped: %v\n", serverID, port, err)
	}
}

func main() {
	portFlag := flag.String("port", "", "Port to run the backend server on")
	allFlag := flag.Bool("all", false, "Start all 3 default backend instances (8082, 8083, 8084)")
	flag.Parse()

	if *allFlag || (*portFlag == "" && os.Getenv("PORT") == "") {
		var wg sync.WaitGroup
		backends := []struct {
			port string
			id   string
		}{
			{"8082", "backend-1"},
			{"8083", "backend-2"},
			{"8084", "backend-3"},
		}

		for _, b := range backends {
			wg.Add(1)
			go func(p, id string) {
				defer wg.Done()
				startServer(p, id)
			}(b.port, b.id)
		}
		wg.Wait()
		return
	}

	port := *portFlag
	if port == "" {
		port = os.Getenv("PORT")
	}
	serverID := os.Getenv("SID")
	if serverID == "" {
		serverID = "backend-" + port
	}

	startServer(port, serverID)
}
