package main

import (
	"context"
	"log"
	"sync"
	"time"

	"code.vegaprotocol.io/vega/libs/ptr"
	apipb "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	vegapb "code.vegaprotocol.io/vega/protos/vega"
	"golang.org/x/exp/maps"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

type VegaStore struct {
	mu sync.RWMutex

	// the market our bot is trading on
	market *vegapb.Market
	// the market our bot is trading on
	marketData *vegapb.MarketData
	// the liquidityProvision for this pubkey
	liquidityProvision *vegapb.LiquidityProvision
	// our pubkey accounts
	// map[type+asset+market]Account
	accounts map[string]*apipb.AccountBalance
	// our party orders
	// map[orderId]Order
	orders map[string]*vegapb.Order
	// position of our party for the given market
	position *vegapb.Position
	// assets
	assets map[string]*vegapb.Asset
}

func NewVegaStore() *VegaStore {
	return &VegaStore{
		accounts: map[string]*apipb.AccountBalance{},
		orders:   map[string]*vegapb.Order{},
		assets:   map[string]*vegapb.Asset{},
	}
}

func (v *VegaStore) SetAsset(asset *vegapb.Asset) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.assets[asset.Id] = asset
}

func (v *VegaStore) GetAsset(assetID string) *vegapb.Asset {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.assets[assetID]
}

func (v *VegaStore) GetAssets() []*vegapb.Asset {
	v.mu.Lock()
	defer v.mu.Unlock()
	return maps.Values(v.assets)
}

func (v *VegaStore) SetMarket(market *vegapb.Market) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.market = market
}

func (v *VegaStore) SetLiquidityProvision(lp *vegapb.LiquidityProvision) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	v.liquidityProvision = lp
}

func (v *VegaStore) GetMarket() *vegapb.Market {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.market
}

func (v *VegaStore) SetMarketData(marketData *vegapb.MarketData) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.marketData = marketData
}

func (v *VegaStore) GetMarketData() *vegapb.MarketData {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.marketData
}

func (v *VegaStore) SetPosition(position *vegapb.Position) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.position = position
}

func (v *VegaStore) GetPosition() *vegapb.Position {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.position
}

func (v *VegaStore) SetOrders(orders []*vegapb.Order) {
	v.mu.Lock()
	defer v.mu.Unlock()

	for _, o := range orders {
		if o.Status != vegapb.Order_STATUS_ACTIVE {
			delete(v.orders, o.Id)
			continue
		}

		v.orders[o.Id] = o
	}
}

func (v *VegaStore) GetOrder(id string) *vegapb.Order {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.orders[id]
}

func (v *VegaStore) GetOrders() []*vegapb.Order {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return maps.Values(v.orders)
}

func (v *VegaStore) SetAccounts(accounts []*apipb.AccountBalance) {
	v.mu.Lock()
	defer v.mu.Unlock()

	for _, a := range accounts {
		v.accounts[a.Type.String()+a.Asset+a.MarketId] = a
	}
}

func (v *VegaStore) GetAccount(market, asset string, typ vegapb.AccountType) *apipb.AccountBalance {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.accounts[typ.String()+asset+market]
}

func (v *VegaStore) GetAccounts() []*apipb.AccountBalance {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return maps.Values(v.accounts)
}

func (v *VegaStore) GetLiquidityProvison() *vegapb.LiquidityProvision {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return proto.Clone(v.liquidityProvision).(*vegapb.LiquidityProvision)
}

type vegaAPI struct {
	config *Config
	store  *VegaStore
	svc    apipb.TradingDataServiceClient
}

func VegaAPI(config *Config, store *VegaStore) {
	conn, err := grpc.Dial(config.VegaGRPCURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("could not open connection with vega node: %v", err)
	}

	svc := apipb.NewTradingDataServiceClient(conn)

	api := &vegaAPI{
		config: config,
		svc:    svc,
		store:  store,
	}

	// now populate initial data
	api.loadMarket()
	api.loadMarketData()
	api.loadAccounts()
	api.loadOrders()
	api.loadPosition()
	api.loadAssets()
	api.loadLP()

	go func() {
		// then we start our streams
		go api.streamMarketData()
		go api.streamAccounts()
		go api.streamOrders()
		go api.streamPosition()
		go api.streamLP()
	}()

	return
}

func (v *vegaAPI) loadLP() {
	resp, err := v.svc.ListLiquidityProvisions(context.Background(), &apipb.ListLiquidityProvisionsRequest{
		MarketId: ptr.From(v.config.VegaMarket),
		PartyId:  ptr.From(v.config.WalletPubkey),
		Live:     ptr.From(true),
	})

	if err != nil {
		log.Printf("could not load liquidity provision: %v", err)
	}

	if len(resp.LiquidityProvisions.Edges) > 0 {
		v.store.SetLiquidityProvision(resp.LiquidityProvisions.Edges[0].Node)
	}
}

func (v *vegaAPI) streamLP() {
	for range time.NewTicker(5 * time.Second).C {
		resp, err := v.svc.ListLiquidityProvisions(context.Background(), &apipb.ListLiquidityProvisionsRequest{
			MarketId: ptr.From(v.config.VegaMarket),
			PartyId:  ptr.From(v.config.WalletPubkey),
			Live:     ptr.From(true),
		})
		if err != nil {
			log.Printf("could not load liquidity provision: %v", err)
		}

		if len(resp.LiquidityProvisions.Edges) > 0 {
			v.store.SetLiquidityProvision(resp.LiquidityProvisions.Edges[0].Node)
		}
	}
}

func (v *vegaAPI) streamMarketData() {
	stream, err := v.svc.ObserveMarketsData(context.Background(), &apipb.ObserveMarketsDataRequest{MarketIds: []string{v.config.VegaMarket}})
	if err != nil {
		log.Fatalf("could not start market data stream: %v", err)
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			log.Fatalf("could not recv market data: %v", err)
		}

		for _, md := range resp.MarketData {
			v.store.SetMarketData(md)
		}
	}
}

func (v *vegaAPI) streamPosition() {
	stream, err := v.svc.ObservePositions(context.Background(), &apipb.ObservePositionsRequest{MarketId: ptr.From(v.config.VegaMarket), PartyId: ptr.From(v.config.WalletPubkey)})
	if err != nil {
		log.Fatalf("could not start market data stream: %v", err)
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			log.Fatalf("could not recv market data: %v", err)
		}

		switch r := resp.Response.(type) {
		case *apipb.ObservePositionsResponse_Snapshot:
			v.store.SetPosition(r.Snapshot.Positions[0])
		case *apipb.ObservePositionsResponse_Updates:
			v.store.SetPosition(r.Updates.Positions[0])
		}
	}
}

func (v *vegaAPI) streamOrders() {
	stream, err := v.svc.ObserveOrders(context.Background(), &apipb.ObserveOrdersRequest{MarketIds: []string{v.config.VegaMarket}, PartyIds: []string{v.config.WalletPubkey}})
	if err != nil {
		log.Fatalf("could not start market data stream: %v", err)
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			log.Fatalf("could not recv market data: %v", err)
		}

		switch r := resp.Response.(type) {
		case *apipb.ObserveOrdersResponse_Snapshot:
			v.store.SetOrders(r.Snapshot.Orders)
		case *apipb.ObserveOrdersResponse_Updates:
			v.store.SetOrders(r.Updates.Orders)
		}
	}
}

func (v *vegaAPI) streamAccounts() {
	stream, err := v.svc.ObserveAccounts(context.Background(), &apipb.ObserveAccountsRequest{PartyId: v.config.WalletPubkey})
	if err != nil {
		log.Fatalf("could not start market data stream: %v", err)
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			log.Fatalf("could not recv market data: %v", err)
		}

		switch r := resp.Response.(type) {
		case *apipb.ObserveAccountsResponse_Snapshot:
			v.store.SetAccounts(r.Snapshot.Accounts)
		case *apipb.ObserveAccountsResponse_Updates:
			v.store.SetAccounts(r.Updates.Accounts)
		}
	}
}

func (v *vegaAPI) loadMarket() {
	resp, err := v.svc.GetMarket(context.Background(), &apipb.GetMarketRequest{MarketId: v.config.VegaMarket})
	if err != nil {
		log.Fatalf("couldn't load the vega market: %v", err)
	}

	v.store.SetMarket(resp.Market)
}

func (v *vegaAPI) loadMarketData() {
	resp, err := v.svc.GetLatestMarketData(context.Background(), &apipb.GetLatestMarketDataRequest{MarketId: v.config.VegaMarket})
	if err != nil {
		log.Fatalf("couldn't load the vega market: %v", err)
	}

	v.store.SetMarketData(resp.MarketData)
}

func (v *vegaAPI) loadAssets() {
	resp, err := v.svc.ListAssets(context.Background(), &apipb.ListAssetsRequest{})
	if err != nil {
		log.Fatalf("couldn't load the vega market: %v", err)
	}

	for _, a := range resp.Assets.Edges {
		v.store.SetAsset(a.Node)
	}
}

func (v *vegaAPI) loadAccounts() {
	resp, err := v.svc.ListAccounts(context.Background(), &apipb.ListAccountsRequest{Filter: &apipb.AccountFilter{PartyIds: []string{v.config.WalletPubkey}}})
	if err != nil {
		log.Fatalf("couldn't load the vega market: %v", err)
	}

	accounts := []*apipb.AccountBalance{}
	for _, a := range resp.Accounts.Edges {
		accounts = append(accounts, a.Node)
	}

	v.store.SetAccounts(accounts)
}

func (v *vegaAPI) loadOrders() {
	resp, err := v.svc.ListOrders(context.Background(), &apipb.ListOrdersRequest{Filter: &apipb.OrderFilter{PartyIds: []string{v.config.WalletPubkey}, MarketIds: []string{v.config.VegaMarket}, LiveOnly: ptr.From(true)}})
	if err != nil {
		log.Fatalf("couldn't load the vega market: %v", err)
	}

	orders := []*vegapb.Order{}
	for _, o := range resp.Orders.Edges {
		orders = append(orders, o.Node)
	}

	v.store.SetOrders(orders)
}

func (v *vegaAPI) loadPosition() {
	resp, err := v.svc.ListPositions(context.Background(), &apipb.ListPositionsRequest{PartyId: v.config.WalletPubkey, MarketId: v.config.VegaMarket})
	if err != nil {
		log.Fatalf("couldn't load the vega market: %v", err)
	}

	if len(resp.Positions.Edges) > 1 {
		log.Fatalf("invalid number of positions loaded: %v", len(resp.Positions.Edges))
	}

	if len(resp.Positions.Edges) == 1 {
		v.store.SetPosition(resp.Positions.Edges[0].Node)
	}
}
