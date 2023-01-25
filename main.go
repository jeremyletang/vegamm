package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	wallet "github.com/jeremyletang/vega-go-sdk/wallet"
)

const (
	defaultAppPort      = 8080
	defaultWalletURL    = "http://127.0.0.1:1789"
	defaultVegaGRPCURL  = "n11.testnet.vega.xyz:3007"
	defaultBinanceWSURL = "wss://stream.binance.com:443/ws"
)

var (
	appPort       uint
	vegaGRPCURL   string
	walletURL     string
	walletToken   string
	walletPubkey  string
	binanceWSURL  string
	vegaMarket    string
	binanceMarket string
)

func init() {
	flag.UintVar(&appPort, "port", defaultAppPort, "port of the http API")
	flag.StringVar(&vegaGRPCURL, "vega-grpc-url", defaultVegaGRPCURL, "a vega grpc server")
	flag.StringVar(&walletURL, "wallet-url", defaultWalletURL, "a vega wallet service address")
	flag.StringVar(&walletToken, "wallet-token", "", "a vega wallet token (for info see vega wallet token-api -h)")
	flag.StringVar(&walletPubkey, "wallet-pubkey", "", "a vega public key")
	flag.StringVar(&binanceWSURL, "binance-ws-url", defaultBinanceWSURL, "binance websocket url")
	flag.StringVar(&vegaMarket, "vega-market", "", "a vega market id")
	flag.StringVar(&binanceMarket, "binance-market", "", "a binance market symbol")
}

func main() {
	config := parseFlags()

	// connect to the wallet
	w, err := wallet.NewClient(config.WalletURL, config.WalletToken)
	if err != nil {
		log.Fatalf("could not connect to the wallet: %v", err)
	}

	// share binance reference price for the given market
	binanceRefPrice := NewBinanceRP(config.BinanceMarket)

	// listening to the binance data
	go BinanceAPI(config, binanceRefPrice)

	// start the vega API stuff
	vegaStore := NewVegaStore()
	go VegaAPI(config, vegaStore)

	// start the strategy
	go RunStrategy(config, w, vegaStore, binanceRefPrice)

	// start the state API
	go StartAPI(config, vegaStore, binanceRefPrice)

	// just waiting for users to close
	gracefulStop := make(chan os.Signal, 1)
	signal.Notify(gracefulStop, syscall.SIGTERM, syscall.SIGINT)
	<-gracefulStop

	log.Print("closing on user request.")
}
