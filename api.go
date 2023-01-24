package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	apipb "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	vegapb "code.vegaprotocol.io/vega/protos/vega"
	"github.com/shopspring/decimal"
)

type State struct {
	Position   *vegapb.Position
	Market     *vegapb.Market
	MarketData *vegapb.MarketData
	BestBid    decimal.Decimal
	BestAsk    decimal.Decimal
	Orders     []*vegapb.Order
	Accounts   []*apipb.AccountBalance
	Assets     []*vegapb.Asset
}

func StartAPI(config *Config, vega *VegaStore, refPrice *BinanceRP) {
	http.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		bid, ask := refPrice.Get()
		state := State{
			Position:   vega.GetPosition(),
			Market:     vega.GetMarket(),
			MarketData: vega.GetMarketData(),
			Orders:     vega.GetOrders(),
			Accounts:   vega.GetAccounts(),
			BestBid:    bid,
			BestAsk:    ask,
			Assets:     vega.GetAssets(),
		}

		out, _ := json.Marshal(&state)
		fmt.Fprintf(w, "%v", string(out))
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
