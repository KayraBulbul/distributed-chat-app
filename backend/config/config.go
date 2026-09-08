package config

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/KayraBulbul/distributed-chat-app/backend/internal/database"
	"github.com/joho/godotenv"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Config struct {
	Queries *database.Queries
}

func CreateCfg() (Config, error) {
	_ = godotenv.Load()

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		return Config{}, fmt.Errorf("DB_URL is required")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return Config{}, err
	}

	dbQueries := database.New(db)

	cfg := Config{
		Queries: dbQueries,
	}

	return cfg, nil
}
