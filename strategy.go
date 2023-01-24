package main

import (
	"context"
	"log"
	"time"

	vegapb "code.vegaprotocol.io/vega/protos/vega"
	commandspb "code.vegaprotocol.io/vega/protos/vega/commands/v1"
	walletpb "code.vegaprotocol.io/vega/protos/vega/wallet/v1"
	"github.com/jeremyletang/vega-go-sdk/wallet"
	"github.com/shopspring/decimal"
)

func RunStrategy(
	config *Config,
	w *wallet.Client,
	vega *VegaStore,
	refPrice *BinanceRP,
) {
	var (
		pubkey = config.WalletPubkey
		mktid  = config.VegaMarket
	)

	for range time.NewTicker(5 * time.Second).C {
		log.Printf("executing trading strategy...")
		if mkt := vega.GetMarket(); mkt != nil {
			asset := vega.GetAsset(
				mkt.GetTradableInstrument().
					GetInstrument().
					GetFuture().
					GetSettlementAsset(),
			)

			d := newDecimals(mkt, asset)

			log.Printf("updating quotes for %v", mkt.GetTradableInstrument().GetInstrument().GetName())
			bestBid, bestAsk := refPrice.Get()
			log.Printf("new reference prices: bestBid(%v), bestAsk(%v)", bestBid, bestAsk)
			openVol, aep := volumeAndAverageEntryPrice(d, mkt, vega.GetPosition())
			balance := getPubkeyBalance(vega, pubkey, asset.Id, int64(asset.Details.Decimals))
			log.Printf("pubkey balance: %v", balance)
			bidVol := balance.Mul(decimal.NewFromFloat(0.5)).Sub(openVol.Mul(aep))
			offerVol := balance.Mul(decimal.NewFromFloat(0.5)).Add(openVol.Mul(aep))
			notionalExposure := openVol.Mul(aep).Abs()
			log.Printf("openvolume(%v), entryPrice(%v), notionalExposure(%v)",
				openVol, aep, notionalExposure,
			)
			log.Printf("bidVolume(%v), offerVolume(%v)", bidVol, offerVol)

			batch := commandspb.BatchMarketInstructions{
				Cancellations: []*commandspb.OrderCancellation{
					{
						MarketId: mktid,
					},
				},
				Submissions: append(
					getOrderSubmission(d, bestBid, vegapb.Side_SIDE_BUY, mktid, bidVol),
					getOrderSubmission(d, bestAsk, vegapb.Side_SIDE_SELL, mktid, offerVol)...),
			}

			err := w.SendTransaction(
				context.Background(), pubkey, &walletpb.SubmitTransactionRequest{
					Command: &walletpb.SubmitTransactionRequest_BatchMarketInstructions{
						BatchMarketInstructions: &batch,
					},
				},
			)
			if err != nil {
				log.Printf("error submitting batch: %v", err)
			}

			log.Printf("batch submission: %v", batch.String())
		}
	}
}

func getOrderSubmission(
	d decimals,
	refPrice decimal.Decimal,
	side vegapb.Side,
	mktid string,
	targetVolume decimal.Decimal,
) []*commandspb.OrderSubmission {
	size := targetVolume.Div(decimal.NewFromInt(5).Mul(refPrice))
	orders := []*commandspb.OrderSubmission{}

	priceF := func(i int) decimal.Decimal {
		return refPrice.Mul(
			decimal.NewFromInt(1).Sub(
				decimal.NewFromInt(int64(i)).Mul(decimal.NewFromFloat(0.002)),
			),
		)
	}
	if side == vegapb.Side_SIDE_SELL {
		priceF = func(i int) decimal.Decimal {
			return refPrice.Mul(
				decimal.NewFromInt(1).Add(
					decimal.NewFromInt(int64(i)).Mul(decimal.NewFromFloat(0.002)),
				),
			)
		}
	}

	for i := 1; i <= 5; i++ {
		orders = append(orders, &commandspb.OrderSubmission{
			MarketId:    mktid,
			Price:       d.ToMarketPricePrecision(priceF(i)).BigInt().String(),
			Size:        d.ToMarketPositionPrecision(size).BigInt().Uint64(),
			Side:        vegapb.Side_SIDE_BUY,
			TimeInForce: vegapb.Order_TIME_IN_FORCE_GTC,
			Type:        vegapb.Order_TYPE_LIMIT,
			Reference:   "VEGA_GO_MM_SIMPLE",
		})
	}

	return orders
}

func getPubkeyBalance(
	vega *VegaStore,
	pubkey, asset string,
	decimalPlaces int64,
) (d decimal.Decimal) {
	for _, a := range vega.GetAccounts() {
		if a.Asset != asset || a.Owner != pubkey {
			continue
		}

		balance, _ := decimal.NewFromString(a.Balance)
		d = d.Add(balance)
	}

	return d.Div(decimal.NewFromFloat(10).Pow(decimal.NewFromInt(decimalPlaces)))
}

func volumeAndAverageEntryPrice(
	d decimals, mkt *vegapb.Market, pos *vegapb.Position,
) (vol, aep decimal.Decimal) {
	if pos == nil {
		return
	}

	vol = decimal.NewFromInt(pos.OpenVolume)
	aep, _ = decimal.NewFromString(pos.AverageEntryPrice)

	return d.FromMarketPositionPrecision(vol), d.FromMarketPricePrecision(aep)
}

type decimals struct {
	positionFactor decimal.Decimal
	priceFactor    decimal.Decimal
}

func newDecimals(mkt *vegapb.Market, asset *vegapb.Asset) decimals {
	return decimals{
		positionFactor: decimal.NewFromFloat(10).Pow(decimal.NewFromInt(mkt.PositionDecimalPlaces)),
		priceFactor:    decimal.NewFromFloat(10).Pow(decimal.NewFromInt(int64(mkt.DecimalPlaces))),
	}
}

func (d decimals) FromMarketPricePrecision(price decimal.Decimal) decimal.Decimal {
	return price.Div(d.priceFactor)
}

func (d decimals) FromMarketPositionPrecision(pos decimal.Decimal) decimal.Decimal {
	return pos.Div(d.positionFactor)
}

func (d decimals) ToMarketPricePrecision(price decimal.Decimal) decimal.Decimal {
	return price.Mul(d.priceFactor)
}

func (d decimals) ToMarketPositionPrecision(pos decimal.Decimal) decimal.Decimal {
	return pos.Mul(d.positionFactor)
}
