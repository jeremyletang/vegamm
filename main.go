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
	defaultWalletURL    = "http://127.0.0.1:1789"
	defaultVegaGRPCURL  = "n11.testnet.vega.xyz:3007"
	defaultBinanceWSURL = "wss://stream.binance.com:443/ws"
)

var (
	vegaGRPCURL   string
	walletURL     string
	walletToken   string
	walletPubkey  string
	binanceWSURL  string
	vegaMarket    string
	binanceMarket string
)

func init() {
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

	// err = w.SendTransaction(
	// 	context.Background(),
	// 	config.WalletPubkey,
	// 	&walletpb.SubmitTransactionRequest{
	// 		Command: &walletpb.SubmitTransactionRequest_VoteSubmission{
	// 			VoteSubmission: &commandspb.VoteSubmission{
	// 				ProposalId: "90e71c52b2f40db78efc24abe4217382993868cd24e45b3dd17147be4afaf884",
	// 				Value:      vega.Vote_VALUE_NO,
	// 			},
	// 		},
	// 	},
	// )

	if err != nil {
		log.Fatalf("error: %v", err)
	}

	// share binance reference price for the given market
	binanceRefPrice := NewBinanceRP(config.BinanceMarket)

	// listening to the binance data
	go BinanceAPI(config, binanceRefPrice)

	// start the vega API stuff
	vegaStore := NewVegaStore()
	go VegaAPI(config, vegaStore)

	strategy := NewStrategy(w, vegaStore, binanceRefPrice)
	go strategy.Run()

	// just waiting for users to close
	gracefulStop := make(chan os.Signal, 1)
	signal.Notify(gracefulStop, syscall.SIGTERM, syscall.SIGINT)
	<-gracefulStop

	log.Print("closing on user request.")
}
