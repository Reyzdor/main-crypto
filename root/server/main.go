package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"main-crypto/pkg/bot"
	"main-crypto/pkg/db"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

var (
	dbConn *pgx.Conn
	port   string

	prices = make(map[string]string)
	mu     sync.RWMutex

	prices24hAgo = map[string]float64{
		"BTC": 0,
		"ETH": 0,
		"SOL": 0,
	}
	mu24h sync.RWMutex
)

func main() {
	_ = godotenv.Load()

	port = os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	conn := db.ConnToDB()
	defer conn.Close(context.Background())

	go bot.Conn(conn)

	go wsToCoin("BTCUSDT", "BTC")
	go wsToCoin("ETHUSDT", "ETH")
	go wsToCoin("SOLUSDT", "SOL")

	go runHTTP()

	select {}
}

func runHTTP() {
	http.HandleFunc("/price", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		jsonResponse(w, prices)
	})

	http.HandleFunc("/api/24h", func(w http.ResponseWriter, r *http.Request) {
		mu24h.RLock()
		defer mu24h.RUnlock()
		jsonResponse(w, prices24hAgo)
	})

	go update24hPrices()

	log.Println("Server running on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(data)
}

func update24hPrices() {
	for {
		for _, s := range []struct {
			coin, symbol string
		}{
			{"BTC", "BTCUSDT"},
			{"ETH", "ETHUSDT"},
			{"SOL", "SOLUSDT"},
		} {
			price, err := bybitPrev24hPrice(s.symbol)
			if err != nil {
				log.Println("Bybit 24h error:", s.coin, err)
				continue
			}
			mu24h.Lock()
			prices24hAgo[s.coin] = price
			mu24h.Unlock()
		}
		time.Sleep(10 * time.Minute)
	}
}

func bybitPrev24hPrice(symbol string) (float64, error) {
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

	return strconv.ParseFloat(res.Result.List[0].PrevPrice24h, 64)
}

type TickerMsg struct {
	Data struct {
		LastPrice string `json:"lastPrice"`
	} `json:"data"`
}

func wsToCoin(symbol, coinID string) {
	conn, _, err := websocket.DefaultDialer.Dial(
		"wss://stream.bybit.com/v5/public/spot",
		nil,
	)
	if err != nil {
		log.Println("WS error:", err)
		return
	}
	defer conn.Close()

	sub := map[string]any{
		"op":   "subscribe",
		"args": []string{"tickers." + symbol},
	}

	msg, _ := json.Marshal(sub)
	_ = conn.WriteMessage(websocket.TextMessage, msg)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("WS read:", err)
			return
		}

		var t TickerMsg
		if json.Unmarshal(msg, &t) == nil && t.Data.LastPrice != "" {
			mu.Lock()
			prices[coinID] = t.Data.LastPrice
			mu.Unlock()
		}
	}
}
