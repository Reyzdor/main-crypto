package main

import (
	"context"
	"encoding/json"
	"log"
	"main-crypto/pkg/bot"
	"main-crypto/pkg/db"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

var dbConn *pgx.Conn
var port string

var prices = make(map[string]string)
var mu sync.RWMutex

var prices24hAgo = map[string]float64{
	"BTC": 0,
	"ETH": 0,
	"SOL": 0,
}
var mu24h sync.RWMutex

func main() {
	_ = godotenv.Load()

	port = os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	conn := db.ConnToDB()
	defer dbConn.Close(context.Background())

	go bot.Conn(conn)

	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		updateBasePrices()
		for range ticker.C {
			updateBasePrices()
		}
	}()

	go wsToCoin("BTCUSDT", "BTC")
	go wsToCoin("ETHUSDT", "ETH")
	go wsToCoin("SOLUSDT", "SOL")

	go runHTTP()

	select {}
}

type Coin struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type BasePrice struct {
	Price float64   `json:"price"`
	Time  time.Time `json:"time"`
}

func initBasePrices() {
	for _, c := range []string{"BTC", "ETH", "SOL"} {
		if _, err := loadBasePrice(c); err != nil {
			log.Println("Base price not found for", c)
		}
	}
}

func updateBasePrices() {
	for _, sym := range []struct {
		coin, symbol string
	}{
		{"BTC", "BTCUSDT"},
		{"ETH", "ETHUSDT"},
		{"SOL", "SOLUSDT"},
	} {
		mu.RLock()
		lastPriceStr := prices[sym.coin]
		mu.RUnlock()

		if lastPriceStr == "" {
			continue
		}

		price, err := strconv.ParseFloat(lastPriceStr, 64)
		if err != nil {
			log.Println("Error parsing price for", sym.coin, err)
			continue
		}

		setBasePrice(sym.coin, price)
		log.Println("Updated base price for", sym.coin, "=", price)
	}
}

func setBasePrice(symbol string, price float64) {
	data := BasePrice{
		Price: price,
		Time:  time.Now(),
	}
	b, _ := json.Marshal(data)
	os.WriteFile(symbol+"_base.json", b, 0644)
}

func loadBasePrice(symbol string) (BasePrice, error) {
	b, err := os.ReadFile(symbol + "_base.json")
	if err != nil {
		return BasePrice{}, err
	}
	var bp BasePrice
	json.Unmarshal(b, &bp)
	return bp, nil
}

func coinsHandler(w http.ResponseWriter, r *http.Request) {
	coins := []Coin{
		{ID: "btc", Name: "Bitcoin"},
		{ID: "eth", Name: "Ethereum"},
		{ID: "sol", Name: "Solana"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(coins)
}

func runHTTP() {
	http.HandleFunc("/price", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(prices)
	})

	http.HandleFunc("/coins", coinsHandler)

	http.HandleFunc("/api/base-price", func(w http.ResponseWriter, r *http.Request) {
		result := map[string]BasePrice{}
		for _, c := range []string{"BTC", "ETH", "SOL"} {
			bp, err := loadBasePrice(c)
			if err != nil {
				log.Println("Base price not found for", c)
				continue
			}
			result[c] = bp
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(result)
	})

	log.Println("Server running on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

type TickerMsg struct {
	Topic string `json:"topic"`
	Data  struct {
		LastPrice string `json:"lastPrice"`
	} `json:"data"`
}

func wsToCoin(symbol string, coinID string) {
	url := "wss://stream.bybit.com/v5/public/spot"

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Println("WS error:", err)
		return
	}
	defer conn.Close()

	sub := map[string]interface{}{
		"op":   "subscribe",
		"args": []string{"tickers." + symbol},
	}

	msg, _ := json.Marshal(sub)
	conn.WriteMessage(websocket.TextMessage, msg)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("WS read:", err)
			return
		}

		var ticker TickerMsg
		if json.Unmarshal(msg, &ticker) == nil && ticker.Data.LastPrice != "" {
			mu.Lock()
			prices[coinID] = ticker.Data.LastPrice
			mu.Unlock()
		}
	}
}
