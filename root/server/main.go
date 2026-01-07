package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"

	"main-crypto/pkg/bot"
	"main-crypto/pkg/db"
)

var dbConn *pgx.Conn

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file:", err)
	}

	conn := db.ConnToDB()

	defer dbConn.Close(context.Background())

	bot.Conn(conn)
}
