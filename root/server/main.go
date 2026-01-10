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
	url := fmt.Sprintf(
		"https://api.bybit.com/v5/market/tickers?category=spot&symbol=%s",
		symbol,
	)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Bybit returned status %d", resp.StatusCode)
	}

	var res struct {
		Result struct {
			List []struct {
				PrevPrice24h string `json:"prevPrice24h"`
			} `json:"list"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, err
	}

	if len(res.Result.List) == 0 {
		return 0, fmt.Errorf("empty ticker list")
	}

	price, err := strconv.ParseFloat(res.Result.List[0].PrevPrice24h, 64)
	if err != nil {
		return 0, err
	}

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
				{"BTC", "BTCUSDT"},
				{"ETH", "ETHUSDT"},
				{"SOL", "SOLUSDT"},
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
