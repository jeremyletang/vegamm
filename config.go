package main

import (
	"flag"
	"log"
	"os"
)

type Config struct {
	VegaGRPCURL   string
	WalletURL     string
	BinanceWSURL  string
	WalletToken   string
	WalletPubkey  string
	VegaMarket    string
	BinanceMarket string
}

func parseFlags() *Config {
	flag.Parse()

	if vegaGRPCURL = getSetting(vegaGRPCURL, os.Getenv("VEGAMM_VEGA_GRPC_URL")); len(vegaGRPCURL) <= 0 {
		vegaGRPCURL = defaultVegaGRPCURL
	}

	if walletURL = getSetting(walletURL, os.Getenv("VEGAMM_WALLET_URL")); len(walletURL) <= 0 {
		walletURL = defaultWalletURL
	}

	if binanceWSURL = getSetting(binanceWSURL, os.Getenv("VEGAMM_BINANCE_WS_URL")); len(binanceWSURL) <= 0 {
		binanceWSURL = defaultBinanceWSURL
	}

	if walletToken = getSetting(walletToken, os.Getenv("VEGAMM_WALLET_TOKEN")); len(walletToken) <= 0 {
		log.Fatal("error: -wallet-token flag is required")
	}

	if walletPubkey = getSetting(walletPubkey, os.Getenv("VEGAMM_WALLET_PUBKEY")); len(walletPubkey) <= 0 {
		log.Fatal("error: -wallet-pubkey flag is required")
	}

	if vegaMarket = getSetting(vegaMarket, os.Getenv("VEGAMM_VEGA_MARKET")); len(vegaMarket) <= 0 {
		log.Fatal("error: -vega-market flag is required")
	}

	if binanceMarket = getSetting(binanceMarket, os.Getenv("VEGAMM_BINANCE_MARKET")); len(binanceMarket) <= 0 {
		log.Fatal("error: -binance-market flag is required")
	}

	return &Config{
		VegaGRPCURL:   vegaGRPCURL,
		WalletURL:     walletURL,
		BinanceWSURL:  binanceWSURL,
		WalletToken:   walletToken,
		WalletPubkey:  walletPubkey,
		VegaMarket:    vegaMarket,
		BinanceMarket: binanceMarket,
	}
}

func getSetting(flag, env string) string {
	if len(flag) <= 0 {
		return env
	}

	return flag
}
