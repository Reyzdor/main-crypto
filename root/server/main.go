package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"main-crypto/pkg/bot"
	"main-crypto/pkg/db"
	"net/http"
	"os"
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

var cachedPrices = map[string]float64{
	"BTC": 0,
	"ETH": 0,
	"SOL": 0,
}
var muCached sync.RWMutex

func main() {
	_ = godotenv.Load()

	port = os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	conn := db.ConnToDB()
	defer dbConn.Close(context.Background())

	go bot.Conn(conn)

	go wsToCoin("BTCUSDC", "BTC")
	go wsToCoin("ETHUSDC", "ETH")
	go wsToCoin("SOLUSDC", "SOL")

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

func coinGecko() (map[string]float64, error) {
	url := "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin,ethereum,solana&vs_currencies=usd"

	client := &http.Client{}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CoinGecko returned status %d", resp.StatusCode)
	}

	var result map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	prices := make(map[string]float64)
	prices["BTC"] = result["bitcoin"]["usd"]
	prices["ETH"] = result["ethereum"]["usd"]
	prices["SOL"] = result["solana"]["usd"]

	return prices, nil
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

	http.HandleFunc("/api/prices", func(w http.ResponseWriter, r *http.Request) {
		muCached.RLock()
		defer muCached.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(cachedPrices)
	})

	go func() {
		for {
			newPrices, err := coinGecko()
			if err != nil {
				log.Println("CoinGecko error:", err)
			} else {
				muCached.Lock()
				cachedPrices = newPrices
				muCached.Unlock()
			}
			time.Sleep(1 * time.Minute)
		}
	}()

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

func wsToCoin(symbol string, coinID string) {
	url := "wss://stream.bybit.com/v5/public/spot"

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Println("Conn err:", err)
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
			log.Println("Conn read:", err)
			break
		}

		var ticker TickerMsg
		if err := json.Unmarshal(msg, &ticker); err != nil {
			continue
		}

		if ticker.Data.LastPrice != "" {
			mu.Lock()
			prices[coinID] = ticker.Data.LastPrice
			mu.Unlock()
		}
	}
}
