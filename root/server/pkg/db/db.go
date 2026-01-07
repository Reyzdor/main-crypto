package db

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

func ConnToDB() *pgx.Conn {
	dsn := os.Getenv("DB_DSN")
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		panic(fmt.Sprintf("Connection error: %v", err))
	}
	return conn
}
