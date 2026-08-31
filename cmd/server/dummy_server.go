package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	serverId := os.Getenv("SID")
	if port == "" {
		port = "8081"
	}
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok\n"))

	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if s := r.URL.Query().Get("sleep"); s != "" {
			if d, err := time.ParseDuration(s + "s"); err == nil {
				time.Sleep(d)
			}
		}
		w.Write([]byte(fmt.Sprintf("PORT = %s, Server ID - %s", port, serverId)))
	})

	fmt.Println("Backend on", port)
	http.ListenAndServe(":"+port, nil)
}
