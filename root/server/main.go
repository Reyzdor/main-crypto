package main

import (
	"context"
	"encoding/json"
	"log"
	"main-crypto/pkg/bot"
	"main-crypto/pkg/db"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

var dbConn *pgx.Conn
var port string

var lastPrice string = "loading..."
var mu sync.RWMutex

func main() {
	_ = godotenv.Load()

	port = os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbConn = db.ConnToDB()

	defer dbConn.Close(context.Background())

	go bot.Conn(dbConn)

	go wsToBtc()

	go runHTTP()

	select {}

}

type Coin struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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

	http.HandleFunc("/coins", func(w http.ResponseWriter, r *http.Request) {
		coins := []Coin{
			{ID: "btc", Name: "Bitcoin"},
			{ID: "eth", Name: "Ethereum"},
			{ID: "sol", Name: "Solana"},
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(coins)
	})

	log.Println("Server running on port:", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal(err)
	}
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
