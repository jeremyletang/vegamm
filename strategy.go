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
		lpFee  = decimal.RequireFromString(config.LPFee)
	)

	// first we cleanup the current state
	// we cancel all existing orders
	clearAllOrders(w, pubkey, mktid)

	assetBalance := getAssetBalance(vega, pubkey, mktid)

	// our commitment amount will be 1/10 of the balance
	commitmentAmount := assetBalance.Div(decimal.NewFromInt(9))

	// then get / create a new liquidity provision
	err := getOrCreateLPSubmission(w, vega, pubkey, mktid, commitmentAmount, lpFee)
	if err != nil {
		log.Fatalf("couldn't get or submit liquidity order: %v", err)
	}

	for range time.NewTicker(5 * time.Second).C {
		log.Printf("executing trading strategy...")
		if mkt := vega.GetMarket(); mkt != nil {
			var assetId string
			if future := mkt.GetTradableInstrument().
				GetInstrument().
				GetFuture(); future != nil {
				assetId = future.GetSettlementAsset()
			} else if perps := mkt.GetTradableInstrument().
				GetInstrument().
				GetPerpetual(); perps != nil {
				assetId = perps.GetSettlementAsset()
			}

			printCurrentSLAStats(vega, pubkey)

			asset := vega.GetAsset(assetId)

			d := newDecimals(mkt, asset)

			log.Printf("updating quotes for %v", mkt.GetTradableInstrument().GetInstrument().GetName())
			bestBid, bestAsk := refPrice.Get()
			log.Printf("new reference prices: bestBid(%v), bestAsk(%v)", bestBid, bestAsk)
			openVol, aep := volumeAndAverageEntryPrice(d, mkt, vega.GetPosition())
			balance := getPubkeyBalance(vega, pubkey, asset.Id, int64(asset.Details.Decimals))
			log.Printf("pubkey balance: %v", balance)
			bidVol := balance.Mul(decimal.NewFromFloat(0.9)).Sub(openVol.Mul(aep))
			offerVol := balance.Mul(decimal.NewFromFloat(0.9)).Add(openVol.Mul(aep))
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
					getOrderSubmission(d, bestAsk, vegapb.Side_SIDE_SELL, mktid, offerVol)...,
				),
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

func printCurrentSLAStats(
	vega *VegaStore,
	pubkey string,
) {
	for _, v := range vega.GetMarketData().LiquidityProviderSla {
		if v.Party == pubkey {
			log.Printf("current SLA stats: %v", v.String())
			return
		}
	}

	log.Printf("no SLA stats available yet...")
}

func getAssetBalance(
	vega *VegaStore,
	pubkey, market string,
) decimal.Decimal {
	var assetBalance decimal.Decimal
	var assetSym string
	if mkt := vega.GetMarket(); mkt != nil {
		var assetId string
		if future := mkt.GetTradableInstrument().
			GetInstrument().
			GetFuture(); future != nil {
			assetId = future.GetSettlementAsset()
		} else if perps := mkt.GetTradableInstrument().
			GetInstrument().
			GetPerpetual(); perps != nil {
			assetId = perps.GetSettlementAsset()
		}

		asset := vega.GetAsset(assetId)

		assetSym = asset.Details.Symbol

		log.Printf("market is using asset %v", asset.Details.Symbol)

		accounts := vega.GetAccounts()
		// get all balances for the party which
		// are either general + asset ID or
		// market ID.
		for _, acc := range accounts {
			if acc.Asset == asset.Id && acc.Type == vegapb.AccountType_ACCOUNT_TYPE_GENERAL {
				if len(acc.Balance) > 0 {
					assetBalance = assetBalance.Add(decimal.RequireFromString(acc.Balance))
				}
			} else if acc.MarketId == market {
				if len(acc.Balance) > 0 {
					assetBalance = assetBalance.Add(decimal.RequireFromString(acc.Balance))
				}
			}
		}

	}

	if assetBalance.IsZero() {
		log.Fatalf("no balance for asset %v, please deposit funds first", assetSym)
	}

	return assetBalance
}

func getOrCreateLPSubmission(
	w *wallet.Client,
	vega *VegaStore,
	pubkey, market string,
	commitmentAmount decimal.Decimal,
	lpFee decimal.Decimal,
) error {
	lp := vega.GetLiquidityProvison()
	switch lp {
	case nil:
		return submitNewLP(w, vega, pubkey, market, commitmentAmount, lpFee)
	default:
		return maybeAmendLP(w, vega, pubkey, market, commitmentAmount, lp, lpFee)
	}
}

func submitNewLP(
	w *wallet.Client,
	vega *VegaStore,
	pubkey, market string,
	commitmentAmount decimal.Decimal,
	lpFee decimal.Decimal,
) error {
	log.Printf("party have no liquidity provision, try submitting one with a commitment of %v", commitmentAmount.String())

	lp := &commandspb.LiquidityProvisionSubmission{
		MarketId:         market,
		CommitmentAmount: commitmentAmount.Truncate(0).String(),
		Fee:              lpFee.String(),
	}

	err := w.SendTransaction(
		context.Background(), pubkey, &walletpb.SubmitTransactionRequest{
			Command: &walletpb.SubmitTransactionRequest_LiquidityProvisionSubmission{
				LiquidityProvisionSubmission: lp,
			},
		},
	)
	if err != nil {
		log.Printf("error submitting liquidity submission: %v", err)
		return err
	}

	log.Printf("submission submitted successfully: %v", lp.String())

	return nil
}

func maybeAmendLP(
	w *wallet.Client,
	vega *VegaStore,
	pubkey, market string,
	commitmentAmount decimal.Decimal,
	existingLP *vegapb.LiquidityProvision,
	lpFee decimal.Decimal,
) error {
	currentCommitment := decimal.RequireFromString(existingLP.CommitmentAmount)
	currentLPFee := decimal.RequireFromString(existingLP.Fee)

	ok := false

	if currentLPFee.Equal(lpFee) {
		log.Printf("expected commitment == to current commitment, nothing to do: (%v)", lpFee.String())
		ok = true
	}

	if commitmentAmount.Truncate(0).Equal(currentCommitment.Truncate(0)) {
		log.Printf("expected commitment == to current commitment, nothing to do: (%v)", commitmentAmount.Truncate(0).String())
		if ok {
			// fee and amounts are ok, nothing to do
			return nil
		}
	}

	if commitmentAmount.GreaterThan(currentCommitment) {
		log.Printf("current commitment (%v) smaller than expected commitmed (%v), increasing commitment", currentCommitment.String(), commitmentAmount.String())
	}

	if currentCommitment.GreaterThan(commitmentAmount) {
		log.Printf("current commitment (%v) greater than expected commitmed (%v), releasing commitment", currentCommitment.String(), commitmentAmount.String())
	}

	lpAmend := &commandspb.LiquidityProvisionAmendment{
		MarketId:         existingLP.MarketId,
		CommitmentAmount: commitmentAmount.Truncate(0).String(),
		Fee:              lpFee.String(),
	}

	err := w.SendTransaction(
		context.Background(), pubkey, &walletpb.SubmitTransactionRequest{
			Command: &walletpb.SubmitTransactionRequest_LiquidityProvisionAmendment{
				LiquidityProvisionAmendment: lpAmend,
			},
		},
	)
	if err != nil {
		log.Printf("error submitting liquidity submission: %v", err)
		return err
	}

	log.Printf("amendment submitted successfully: %v", lpAmend.String())

	return nil
}

func clearAllOrders(
	w *wallet.Client,
	pubkey, market string,
) {
	batch := commandspb.BatchMarketInstructions{
		Cancellations: []*commandspb.OrderCancellation{
			{
				MarketId: market,
			},
		},
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
			Side:        side,
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
