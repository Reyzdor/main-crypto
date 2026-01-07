package bot

import (
	"log"
	"os"

	"main-crypto/pkg/db"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5"
)

func Conn(conn *pgx.Conn) {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("BOT_TOKEN"))
	if err != nil {
		log.Fatal("Error creating bot:", err)
	}

	u := tgbotapi.NewUpdate(0)
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			db.AddUser(
				conn,
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
