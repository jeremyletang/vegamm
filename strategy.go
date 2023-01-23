package main

import "github.com/jeremyletang/vega-go-sdk/wallet"

type Strategy struct {
	w        *wallet.Client
	vega     *VegaStore
	refPrice *BinanceRP
}

func NewStrategy(
	w *wallet.Client,
	vega *VegaStore,
	refPrice *BinanceRP,
) *Strategy {
	return &Strategy{
		w:        w,
		vega:     vega,
		refPrice: refPrice,
	}
}

func (s *Strategy) Run() {
	// do some cool stuff now.
}
