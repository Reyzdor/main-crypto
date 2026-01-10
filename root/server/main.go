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
	json.NewEncoder(w).Encode(coins)
}

func bybit24hPrice(symbol string) (float64, error) {
	end := time.Now().Add(-24 * time.Hour)
	start := end.Add(-1 * time.Minute)

	url := fmt.Sprintf(
		"https://api.bybit.com/v5/market/kline?category=spot&symbol=%s&interval=1&start=%d&end=%d",
		symbol,
		start.UnixMilli(),
		end.UnixMilli(),
	)

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Bybit returned status %d", resp.StatusCode)
	}

	var res struct {
		Result struct {
			List [][]string `json:"list"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, err
	}

	if len(res.Result.List) == 0 {
		return 0, fmt.Errorf("no kline data")
	}

	price, _ := strconv.ParseFloat(res.Result.List[0][4], 64)
	return price, nil
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

	http.HandleFunc("/api/24h", func(w http.ResponseWriter, r *http.Request) {
		mu24h.RLock()
		defer mu24h.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(prices24hAgo)
	})

	go func() {
		for {
			for _, sym := range []struct {
				coin, symbol string
			}{
				{"BTC", "BTCUSDC"},
				{"ETH", "ETHUSDC"},
				{"SOL", "SOLUSDC"},
			} {
				price, err := bybit24hPrice(sym.symbol)
				if err != nil {
					log.Println("Bybit 24h error:", sym.coin, err)
					continue
				}
				mu24h.Lock()
				prices24hAgo[sym.coin] = price
				mu24h.Unlock()
			}
			time.Sleep(10 * time.Minute)
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
