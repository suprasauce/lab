package main

import (
	"context"
	"log"
	"os"

	"optionsbacktest/database"
	"optionsbacktest/repository"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	db, err := database.NewPostgres(context.Background(), dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	marketDataRepository := repository.NewMarketDataRepository(db)
	// TODO: Use the repository when application behavior is added.
	_ = marketDataRepository
}
