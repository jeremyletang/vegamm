package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

// BinannceRP stores the current reference prices for a given market on binance.
type BinanceRP struct {
	market string

	mu  sync.RWMutex
	bid decimal.Decimal
	ask decimal.Decimal
}

func NewBinanceRP(mkt string) *BinanceRP {
	return &BinanceRP{
		market: mkt,
	}
}

func (b *BinanceRP) Set(bid, ask decimal.Decimal) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bid, b.ask = bid, ask
}

func (b *BinanceRP) Get() (bid, ask decimal.Decimal) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.bid.Copy(), b.ask.Copy()
}

// BinanceAPI a simple routine to listen to prices updates for a market on binance.
func BinanceAPI(config *Config, store *BinanceRP) {
	c, _, err := websocket.DefaultDialer.Dial(config.BinanceWSURL, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	request := struct {
		ID     uint     `json:"id"`
		Method string   `json:"method"`
		Params []string `json:"params"`
	}{
		ID:     1,
		Method: "SUBSCRIBE",
		Params: []string{fmt.Sprintf("%s@ticker", strings.ToLower(config.BinanceMarket))},
	}

	out, _ := json.Marshal(request)
	log.Printf("requesting binance market information: %#v", string(out))
	if err := c.WriteMessage(websocket.TextMessage, out); err != nil {
		log.Fatalf("could not write on binance websocket: %v", err)
	}

	// first message is confirmations, just discard
	_, _, err = c.ReadMessage()
	if err != nil {
		log.Fatalf("could not read from binance websocket: %v", err)
	}

	response := struct {
		E        string          `json:"e"`
		AskPrice decimal.Decimal `json:"a"`
		BidPrice decimal.Decimal `json:"b"`
		// these 3 fields are lame and unused,
		// just here to make the json decoded less
		// confused as it seems to not be able to
		// differentiate between caps and non caps field
		// if not used explicitly...
		NotE uint64 `json:"E"`
		NotA string `json:"A"`
		NotB string `json:"B"`
	}{}

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Fatalf("could not read from binance websocket: %v", err)
		}

		err = json.Unmarshal(message, &response)
		if err != nil {
			log.Fatalf("could not unmarshal binance response: %v - %v", err, string(message))
		}

		if response.E != "24hrTicker" {
			log.Fatalf("unknown event received: %v", string(message))
		}

		store.Set(response.BidPrice.Copy(), response.AskPrice.Copy())
	}
}
