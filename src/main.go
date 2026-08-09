package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Print("no .env file found, relying on environment variables")
	}

	log.Printf("address-serv %s (%s)", version, commit)

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable must be set")
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
	bulk := &bulkStore{db: db}

	mux := http.NewServeMux()
	registerStreetRoutes(mux, streets)
	registerHouseNumberRoutes(mux, houseNumbers)
	registerBulkRoutes(mux, bulk)
	registerDocsRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := loggingMiddleware(corsMiddleware(getenvDefault("CORS_ALLOWED_ORIGIN", "*"))(authMiddleware([]byte(jwtSecret))(mux)))

	addr := getenvDefault("LISTEN_ADDR", ":8080")
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("listening on %s", addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
