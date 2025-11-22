package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"pr-reviewer-service/internal/api"
    "pr-reviewer-service/internal/storage"

    "github.com/gorilla/mux"
    _ "github.com/lib/pq"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/prservice?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(20)
	db.SetConnMaxIdleTime(5 * time.Minute)

	store := storage.NewSQLStore(db)
	handler := api.NewHandler(store)

	r := mux.NewRouter()
	handler.RegisterRoutes(r)

	srv := &http.Server{
		Handler:      r,
		Addr:         ":8080",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	log.Println("listening :8080")
	log.Fatal(srv.ListenAndServe())
}