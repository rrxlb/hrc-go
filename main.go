package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Go Discord Bot - Ready for deployment!"))
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","service":"go-discord-bot","message":"Dependencies will be added incrementally"}`))
	})

	fmt.Printf("Server starting on port %s\n", port)
	fmt.Println("Discord functionality will be added after basic deployment works")
	log.Fatal(http.ListenAndServe(":"+port, nil))
}