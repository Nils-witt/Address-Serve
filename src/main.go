package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Print("no .env file found, relying on environment variables")
	}

	db, err := openDB()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := migrate(db); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	streets := &streetStore{db: db}
	houseNumbers := &houseNumberStore{db: db}

	mux := http.NewServeMux()
	registerStreetRoutes(mux, streets)
	registerHouseNumberRoutes(mux, houseNumbers)
	registerDocsRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := loggingMiddleware(corsMiddleware(getenvDefault("CORS_ALLOWED_ORIGIN", "*"))(mux))

	addr := getenvDefault("LISTEN_ADDR", ":8080")
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
