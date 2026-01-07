package main

import (
	"context"
	"encoding/json"
	"log"
	"main-crypto/pkg/bot"
	"main-crypto/pkg/db"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

var dbConn *pgx.Conn

var lastPrice string = "loading..."
var mu sync.RWMutex

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file:", err)
	}

	conn := db.ConnToDB()

	defer dbConn.Close(context.Background())

	go bot.Conn(conn)

	go wsToBtc()

	go runHTTP()

	select {}

}

func runHTTP() {
	http.HandleFunc("/price", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		price := lastPrice
		mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(map[string]string{"btcPrice": price})
	})

	http.ListenAndServe(":8080", nil)
}

type TickerMsg struct {
	Topic string `json:"topic"`
	Data  struct {
		LastPrice string `json:"lastPrice"`
	} `json:"data"`
}

func wsToBtc() {
	url := "wss://stream.bybit.com/v5/public/spot"

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Println("Conn err:", err)
	}

	defer conn.Close()

	sub := map[string]interface{}{
		"op": "subscribe",
		"args": []string{
			"tickers.BTCUSDC",
		},
	}

	msg, _ := json.Marshal(sub)
	conn.WriteMessage(websocket.TextMessage, msg)

	for {
		_, msg, err := conn.ReadMessage()

		if err != nil {
			log.Println("Conn read:", err)
			continue
		}

		var ticker TickerMsg

		if err := json.Unmarshal(msg, &ticker); err != nil {
			continue
		}

		if ticker.Data.LastPrice != "" {
			mu.Lock()
			lastPrice = ticker.Data.LastPrice
			mu.Unlock()
		}
	}
}
