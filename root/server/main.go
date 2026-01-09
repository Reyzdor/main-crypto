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

	conn := db.ConnToDB()
	defer dbConn.Close(context.Background())

	go bot.Conn(conn)
	go wsToBtc()

	go runHTTP()

	select {}
}

type Coin struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func coinsHandler(w http.ResponseWriter, r *http.Request) {
	coins := []Coin{
		{ID: "btc", Name: "Bitcoin"},
		{ID: "eth", Name: "Ethereum"},
		{ID: "sol", Name: "Solana"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(coins); err != nil {
		log.Println("Error encoding coins:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
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

	http.HandleFunc("/coins", coinsHandler)

	log.Println("Server running on port:", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
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
		return
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
			break
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
