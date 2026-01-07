package db

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"
)

func AddUser(conn *pgx.Conn, username string, tgID int64, firstName string, lastName string) {
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
