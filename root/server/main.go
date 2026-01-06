package main

import (
	"context"
	"fmt"
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

var dbConn *pgx.Conn

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file:", err)
	}

	dbConn = connToDB()
	defer dbConn.Close(context.Background())

	conn()
}

func connToDB() *pgx.Conn {
	dsn := os.Getenv("DB_DSN")
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		panic(fmt.Sprintf("Connection error: %v", err))
	}
	return conn
}

func addUser(conn *pgx.Conn, username string, tgID int64, firstName string, lastName string) {
	_, err := conn.Exec(
		context.Background(),
		`INSERT INTO users_tg (username, tg_id, first_name, last_name)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (tg_id) DO NOTHING`,
		username, tgID, firstName, lastName,
	)
	if err != nil {
		log.Println("Connection error: ", err)
	}
}

func conn() {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("BOT_TOKEN"))
	if err != nil {
		log.Fatal("Error creating bot:", err)
	}

	u := tgbotapi.NewUpdate(0)
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			addUser(
				dbConn,
				update.Message.From.UserName,
				update.Message.From.ID,
				update.Message.From.FirstName,
				update.Message.From.LastName,
			)
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Send(msg)
	}
}
