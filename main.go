package main

import (
	"log"
	"net/http"

	"dbaas/handlers"
)

func main() {
	mux := http.NewServeMux()

	// Servir archivos estáticos
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Página principal
	mux.HandleFunc("/", handlers.IndexHandler)

	// API endpoints
	mux.HandleFunc("/api/provision", handlers.ProvisionHandler)
	mux.HandleFunc("/api/instances", handlers.ListInstancesHandler)
	mux.HandleFunc("/api/instances/delete", handlers.DeleteInstanceHandler)
	mux.HandleFunc("/api/instances/stop", handlers.StopInstanceHandler)
	mux.HandleFunc("/api/logs", handlers.LogsHandler)

	log.Println("Servidor iniciado en http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
