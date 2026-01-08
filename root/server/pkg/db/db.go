package db

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

func ConnToDB() *pgx.Conn {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		panic("DB_DSN is not set in environment variables")
	}

	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		panic(fmt.Sprintf("Connection error: %v", err))
	}
	return conn
}
