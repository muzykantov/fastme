package fastme

import (
	"context"
	"strconv"
	"testing"
	"time"
)

// -----------------------------------------------------------

type tFloat64 float64

// Add is an "+" operation
func (t tFloat64) Add(n Value) Value {
	return t + t.checkNil(n)
}

// Sub is an "-" operation
func (t tFloat64) Sub(n Value) Value {
	return t - t.checkNil(n)
}

// Mul is an "*" operation
func (t tFloat64) Mul(n Value) Value {
	return t * t.checkNil(n)
}

// Cmp returns 1 if self > given, -1 if self < given and 0 if self == given
func (t tFloat64) Cmp(n Value) int {
	num := t.checkNil(n)
	switch {
	case t > num:
		return 1

	case t < num:
		return -1

	}

	return 0
}

// Sign returns 1 if Number > 0, -1 if Number < 0 and 0 if number == 0
func (t tFloat64) Sign() int {
	switch {
	case t < 0:
		return -1

	case t > 0:
		return 1

	}

	return 0
}

// Hash returns any string representation of the Number
func (t tFloat64) Hash() string {
	return strconv.FormatFloat(float64(t), 'f', -1, 64)
}

func (t tFloat64) checkNil(v Value) tFloat64 {
	if v != nil {
		return v.(tFloat64)
	}

	return 0
}

// -----------------------------------------------------------

type tWallet struct {
	balance map[Asset]tFloat64
	inOrder map[Asset]tFloat64
}

func newWallet() *tWallet {
	return &tWallet{
		balance: make(map[Asset]tFloat64),
		inOrder: make(map[Asset]tFloat64),
	}
}

// Balance returns current wallet balance for given asset
func (t *tWallet) Balance(ctx context.Context, a Asset) Value {
	if balance, ok := t.balance[a]; ok {
		return balance
	}

	return tFloat64(0)
}

// UpdateBalance calls by matching engine to update wallet balance
func (t *tWallet) UpdateBalance(ctx context.Context, a Asset, v Value) {
	if v.(tFloat64) == 0 {
		delete(t.balance, a)
		return
	}

	t.balance[a] = v.(tFloat64)
}

// InOrder returns amount of asset in order (optional)
func (t *tWallet) InOrder(ctx context.Context, a Asset) Value {
	if inOrder, ok := t.inOrder[a]; ok {
		return inOrder
	}

	return tFloat64(0)
}

// UpdateInOrder calls by matching engine to inform about freezed monet in order (optional)
func (t *tWallet) UpdateInOrder(ctx context.Context, a Asset, v Value) {
	if v.(tFloat64) == 0 {
		delete(t.inOrder, a)
		return
	}

	t.inOrder[a] = v.(tFloat64)
}

// -----------------------------------------------------------

type tOrder struct {
	id       string
	owner    *tWallet
	quantity tFloat64
	price    tFloat64
	sell     bool
}

func newOrder(id string, owner *tWallet, sell bool, qty float64, price float64) *tOrder {
	return &tOrder{
		id:       id,
		owner:    owner,
		sell:     sell,
		quantity: tFloat64(qty),
		price:    tFloat64(price),
	}
}

// ID returns any uinique string for order
func (t *tOrder) ID() string {
	return t.id
}

// Owner returns wallet id to debit or credit asset on exchange process
func (t *tOrder) Owner() Wallet {
	return t.owner
}

// Sell returns true if order for selling, true otherwise
func (t *tOrder) Sell() bool {
	return t.sell
}

// Price returns order price
func (t *tOrder) Price() Value {
	return t.price
}

// Quantity returns current order quantity
func (t *tOrder) Quantity() Value {
	return t.quantity
}

// UpdateQuantity calls by matching engine to set new order quantity
func (t *tOrder) UpdateQuantity(v Value) {
	t.quantity = v.(tFloat64)
}

// -----------------------------------------------------------

type tEventListener struct {
	incoming  Volume
	existing  Volume
	done      uint64
	priceDone tFloat64
	qtyDone   tFloat64
	partial   Order
}

func newEventListener() *tEventListener {
	return &tEventListener{}
}

func (t *tEventListener) OnIncomingOrderPartial(ctx context.Context, o Order, v Volume) {
	t.partial = o
}

func (t *tEventListener) OnIncomingOrderDone(ctx context.Context, o Order, v Volume) {
	t.done++
}

func (t *tEventListener) OnIncomingOrderPlaced(context.Context, Order) {

}

func (t *tEventListener) OnExistingOrderPartial(ctx context.Context, o Order, v Volume) {
	t.priceDone = t.priceDone.Add(v.Price).(tFloat64)
	t.qtyDone = t.qtyDone.Add(v.Quantity).(tFloat64)
	t.partial = o
}

func (t *tEventListener) OnExistingOrderDone(ctx context.Context, o Order, v Volume) {
	t.priceDone = t.priceDone.Add(v.Price).(tFloat64)
	t.qtyDone = t.qtyDone.Add(v.Quantity).(tFloat64)
	t.done++
}

func (t *tEventListener) OnExistingOrderCanceled(context.Context, Order) {

}

func (t *tEventListener) OnBalanceChanged(context.Context, Wallet, Asset, Value) {

}

func (t *tEventListener) OnInOrderChanged(context.Context, Wallet, Asset, Value) {

}

func walletBalance(w *tWallet, a Asset) float64 {
	return float64(w.Balance(context.Background(), a).(tFloat64))
}

func updateWalletBalance(w *tWallet, a Asset, value float64) {
	w.UpdateBalance(context.Background(), a, tFloat64(value))
}

func walletInOrder(w *tWallet, a Asset) float64 {
	return float64(w.InOrder(context.Background(), a).(tFloat64))
}

func updateWalletInOrder(w *tWallet, a Asset, value float64) {
	w.UpdateInOrder(context.Background(), a, tFloat64(value))
}

// -----------------------------------------------------------

func assertErr(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

func TestAddToOrderBookSellAndCancel(t *testing.T) {
	var (
		asset1, asset2 = Asset("apples"), Asset("dollars")
		wallet1        = newWallet()

		engine = NewEngine(asset1, asset2)

		order1 = newOrder(
			"1",
			wallet1,
			true,
			1,
			10,
		)
	)

	updateWalletBalance(wallet1, asset1, 10)

	if err := engine.PlaceOrder(context.Background(), nil, order1); err != nil {
		t.Fatal(err)
	}

	if walletBalance(wallet1, asset1) != 9 {
		t.Fatal("invalid result")
	}

	if walletInOrder(wallet1, asset1) != 1 {
		t.Fatal("invalid result")
	}

	engine.CancelOrder(context.Background(), nil, order1)

	if walletBalance(wallet1, asset1) != 10 {
		t.Fatal("invalid result")
	}

	if walletInOrder(wallet1, asset1) != 0 {
		t.Fatal("invalid result")
	}
}

func TestAddToOrderBookBuyAndCancel(t *testing.T) {
	var (
		asset1, asset2 = Asset("apples"), Asset("dollars")
		wallet1        = newWallet()

		engine = NewEngine(asset1, asset2)

		order1 = newOrder(
			"1",
			wallet1,
			false,
			1,
			10,
		)
	)

	updateWalletBalance(wallet1, asset2, 100)

	if err := engine.PlaceOrder(context.Background(), nil, order1); err != nil {
		t.Fatal(err)
	}

	if walletBalance(wallet1, asset2) != 90 {
		t.Fatal("invalid result")
	}

	if walletInOrder(wallet1, asset2) != 10 {
		t.Fatal("invalid result")
	}

	engine.CancelOrder(context.Background(), nil, order1)

	if walletBalance(wallet1, asset2) != 100 {
		t.Fatal("invalid result")
	}

	if walletInOrder(wallet1, asset2) != 0 {
		t.Fatal("invalid result")
	}
}

func TestMarketSell(t *testing.T) {
	var (
		processor                 = newEventListener()
		asset1, asset2            = Asset("apples"), Asset("dollars")
		wallet1, wallet2, wallet3 = newWallet(), newWallet(), newWallet()

		engine = NewEngine(asset1, asset2)

		order1 = newOrder(
			"1",
			wallet1,
			false,
			1,
			10,
		)
		order2 = newOrder(
			"2",
			wallet2,
			false,
			1,
			20,
		)
		order3 = newOrder(
			"3",
			wallet3,
			true,
			2,
			0,
		)
	)

	updateWalletBalance(wallet1, asset2, 10)
	updateWalletBalance(wallet2, asset2, 20)
	updateWalletBalance(wallet3, asset1, 2)

	if err := engine.PlaceOrder(context.Background(), processor, order1); err != nil {
		t.Fatal(err)
	}
	if err := engine.PlaceOrder(context.Background(), processor, order2); err != nil {
		t.Fatal(err)
	}
	if err := engine.PlaceOrder(context.Background(), processor, order3); err != nil {
		t.Fatal(err)
	}

	if processor.done != 3 {
		t.Fatalf("invalid result")
	}
	if processor.priceDone != 30 {
		t.Fatalf("invalid result")
	}
	if processor.qtyDone != 2 {
		t.Fatalf("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset1) != 1 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet2, asset1) != 1 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet3, asset1) != 0 {
		t.Fatal("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset2) != 0 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet2, asset2) != 0 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet3, asset2) != 30 {
		t.Fatal("invalid result")
	}
}

func TestMarketBuy(t *testing.T) {
	var (
		processor                 = newEventListener()
		asset1, asset2            = Asset("apples"), Asset("dollars")
		wallet1, wallet2, wallet3 = newWallet(), newWallet(), newWallet()

		engine = NewEngine(asset1, asset2)

		order1 = newOrder(
			"1",
			wallet1,
			true,
			1,
			10,
		)
		order2 = newOrder(
			"2",
			wallet2,
			true,
			1,
			20,
		)
		order3 = newOrder(
			"3",
			wallet3,
			false,
			2,
			0,
		)
	)

	updateWalletBalance(wallet1, asset1, 1)
	updateWalletBalance(wallet2, asset1, 1)
	updateWalletBalance(wallet3, asset2, 30)

	if err := engine.PlaceOrder(context.Background(), processor, order1); err != nil {
		t.Fatal(err)
	}
	if err := engine.PlaceOrder(context.Background(), processor, order2); err != nil {
		t.Fatal(err)
	}
	if err := engine.PlaceOrder(context.Background(), processor, order3); err != nil {
		t.Fatal(err)
	}

	if processor.done != 3 {
		t.Fatalf("invalid result")
	}
	if processor.priceDone != 30 {
		t.Fatalf("invalid result")
	}
	if processor.qtyDone != 2 {
		t.Fatalf("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset2) != 10 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet2, asset2) != 20 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet3, asset2) != 0 {
		t.Fatal("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset1) != 0 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet2, asset1) != 0 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet3, asset1) != 2 {
		t.Fatal("invalid result")
	}
}

func TestMarketSellWithPartial(t *testing.T) {
	var (
		processor                 = newEventListener()
		asset1, asset2            = Asset("apples"), Asset("dollars")
		wallet1, wallet2, wallet3 = newWallet(), newWallet(), newWallet()

		engine = NewEngine(asset1, asset2)

		order1 = newOrder(
			"1",
			wallet1,
			false,
			2,
			10,
		)
		order2 = newOrder(
			"2",
			wallet2,
			false,
			1,
			20,
		)
		order3 = newOrder(
			"3",
			wallet3,
			true,
			2,
			0,
		)
	)

	updateWalletBalance(wallet1, asset2, 20)
	updateWalletBalance(wallet2, asset2, 20)
	updateWalletBalance(wallet3, asset1, 2)

	if err := engine.PlaceOrder(context.Background(), processor, order1); err != nil {
		t.Fatal(err)
	}
	if err := engine.PlaceOrder(context.Background(), processor, order2); err != nil {
		t.Fatal(err)
	}
	if err := engine.PlaceOrder(context.Background(), processor, order3); err != nil {
		t.Fatal(err)
	}

	if processor.done != 2 {
		t.Fatalf("invalid result")
	}
	if processor.partial.Quantity().(tFloat64) != 1 {
		t.Fatalf("invalid result")
	}
	if processor.priceDone != 30 {
		t.Fatalf("invalid result")
	}
	if processor.qtyDone != 2 {
		t.Fatalf("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset1) != 1 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet2, asset1) != 1 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet3, asset1) != 0 {
		t.Fatal("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset2) != 0 {
		t.Fatal("invalid result")
	}
	if walletInOrder(wallet1, asset2) != 10 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet2, asset2) != 0 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet3, asset2) != 30 {
		t.Fatal("invalid result")
	}
}

func TestMarketBuyWithPartial(t *testing.T) {
	var (
		processor                 = newEventListener()
		asset1, asset2            = Asset("apples"), Asset("dollars")
		wallet1, wallet2, wallet3 = newWallet(), newWallet(), newWallet()

		engine = NewEngine(asset1, asset2)

		order1 = newOrder(
			"1",
			wallet1,
			true,
			1,
			10,
		)
		order2 = newOrder(
			"2",
			wallet2,
			true,
			2,
			20,
		)
		order3 = newOrder(
			"3",
			wallet3,
			false,
			2,
			0,
		)
	)

	updateWalletBalance(wallet1, asset1, 1)
	updateWalletBalance(wallet2, asset1, 2)
	updateWalletBalance(wallet3, asset2, 30)

	if err := engine.PlaceOrder(context.Background(), processor, order1); err != nil {
		t.Fatal(err)
	}
	if err := engine.PlaceOrder(context.Background(), processor, order2); err != nil {
		t.Fatal(err)
	}
	if err := engine.PlaceOrder(context.Background(), processor, order3); err != nil {
		t.Fatal(err)
	}

	if processor.done != 2 {
		t.Fatalf("invalid result")
	}
	if processor.partial.Quantity().(tFloat64) != 1 {
		t.Fatalf("invalid result")
	}
	if processor.priceDone != 30 {
		t.Fatalf("invalid result")
	}
	if processor.qtyDone != 2 {
		t.Fatalf("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset2) != 10 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet2, asset2) != 20 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet3, asset2) != 0 {
		t.Fatal("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset1) != 0 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet2, asset1) != 0 {
		t.Fatal("invalid result")
	}
	if walletInOrder(wallet2, asset1) != 1 {
		t.Fatal("invalid result")
	}
	if walletBalance(wallet3, asset1) != 2 {
		t.Fatal("invalid result")
	}
}

func TestLimitSell(t *testing.T) {
	var (
		processor                 = newEventListener()
		asset1, asset2            = Asset("apples"), Asset("dollars")
		wallet1, wallet2, wallet3 = newWallet(), newWallet(), newWallet()

		engine = NewEngine(asset1, asset2)

		order1 = newOrder(
			"1",
			wallet1,
			false,
			1,
			10,
		)
		order2 = newOrder(
			"2",
			wallet2,
			false,
			1,
			20,
		)
		order3 = newOrder(
			"3",
			wallet3,
			true,
			3,
			5,
		)
	)

	updateWalletBalance(wallet1, asset2, 10)
	updateWalletBalance(wallet2, asset2, 20)
	updateWalletBalance(wallet3, asset1, 3)

	if err := engine.PlaceOrder(context.Background(), processor, order1); err != nil {
		t.Fatal(err)
	}
	if err := engine.PlaceOrder(context.Background(), processor, order2); err != nil {
		t.Fatal(err)
	}
	if err := engine.PlaceOrder(context.Background(), processor, order3); err != nil {
		t.Fatal(err)
	}

	if processor.partial.ID() != "3" ||
		processor.partial.Quantity().(tFloat64) != 1 ||
		processor.done != 2 ||
		engine.asks.prices["5"].volume.(tFloat64) != 1 {
		t.Fatalf("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset1) != 1 ||
		walletBalance(wallet2, asset1) != 1 ||
		walletBalance(wallet3, asset1) != 0 ||
		walletInOrder(wallet3, asset1) != 1 {
		t.Fatal("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset2) != 0 ||
		walletBalance(wallet2, asset2) != 0 ||
		walletBalance(wallet3, asset2) != 30 {
		t.Fatal("invalid result")
	}
}

func TestLimitBuy(t *testing.T) {
	var (
		processor                 = newEventListener()
		asset1, asset2            = Asset("apples"), Asset("dollars")
		wallet1, wallet2, wallet3 = newWallet(), newWallet(), newWallet()

		engine = NewEngine(asset1, asset2)

		order1 = newOrder(
			"1",
			wallet1,
			true,
			1,
			10,
		)
		order2 = newOrder(
			"2",
			wallet2,
			true,
			1,
			20,
		)
		order3 = newOrder(
			"3",
			wallet3,
			false,
			3,
			20,
		)
	)

	updateWalletBalance(wallet1, asset1, 1)
	updateWalletBalance(wallet2, asset1, 1)
	updateWalletBalance(wallet3, asset2, 60)

	assertErr(t, engine.PlaceOrder(context.Background(), processor, order1))
	assertErr(t, engine.PlaceOrder(context.Background(), processor, order2))
	assertErr(t, engine.PlaceOrder(context.Background(), processor, order3))

	if processor.partial.ID() != "3" ||
		processor.partial.Quantity().(tFloat64) != 1 ||
		processor.done != 2 ||
		processor.priceDone != 30 ||
		processor.qtyDone != 2 ||
		engine.bids.prices["20"].volume.(tFloat64) != 1 {
		t.Fatalf("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset2) != 10 ||
		walletBalance(wallet2, asset2) != 20 ||
		walletBalance(wallet3, asset2) != 10 ||
		walletInOrder(wallet3, asset2) != 20 {
		t.Fatal("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset1) != 0 ||
		walletBalance(wallet2, asset1) != 0 ||
		walletBalance(wallet3, asset1) != 2 {
		t.Fatal("invalid result")
	}
}

func TestLimitSellWithSelfDone(t *testing.T) {
	var (
		processor                 = newEventListener()
		asset1, asset2            = Asset("apples"), Asset("dollars")
		wallet1, wallet2, wallet3 = newWallet(), newWallet(), newWallet()

		engine = NewEngine(asset1, asset2)

		order1 = newOrder(
			"1",
			wallet1,
			false,
			2,
			10,
		)
		order2 = newOrder(
			"2",
			wallet2,
			false,
			1,
			20,
		)
		order3 = newOrder(
			"3",
			wallet3,
			true,
			2,
			5,
		)
	)

	updateWalletBalance(wallet1, asset2, 20)
	updateWalletBalance(wallet2, asset2, 20)
	updateWalletBalance(wallet3, asset1, 2)

	assertErr(t, engine.PlaceOrder(context.Background(), processor, order1))
	assertErr(t, engine.PlaceOrder(context.Background(), processor, order2))
	assertErr(t, engine.PlaceOrder(context.Background(), processor, order3))

	if processor.partial.ID() != "1" ||
		processor.partial.Quantity().(tFloat64) != 1 ||
		processor.done != 2 ||
		processor.priceDone != 30 ||
		processor.qtyDone != 2 ||
		engine.bids.prices["10"].volume.(tFloat64) != 1 ||
		//---------------
		walletBalance(wallet1, asset1) != 1 ||
		walletBalance(wallet2, asset1) != 1 ||
		walletBalance(wallet3, asset1) != 0 ||
		//---------------
		walletBalance(wallet1, asset2) != 0 ||
		walletInOrder(wallet1, asset2) != 10 ||
		walletBalance(wallet2, asset2) != 0 ||
		walletBalance(wallet3, asset2) != 30 {
		t.Fatalf("invalid result")
	}
}

func TestLimitBuyWithSelfDone(t *testing.T) {
	var (
		processor                 = newEventListener()
		asset1, asset2            = Asset("apples"), Asset("dollars")
		wallet1, wallet2, wallet3 = newWallet(), newWallet(), newWallet()

		engine = NewEngine(asset1, asset2)

		order1 = newOrder(
			"1",
			wallet1,
			true,
			1,
			10,
		)
		order2 = newOrder(
			"2",
			wallet2,
			true,
			2,
			20,
		)
		order3 = newOrder(
			"3",
			wallet3,
			false,
			2,
			30,
		)
	)

	updateWalletBalance(wallet1, asset1, 1)
	updateWalletBalance(wallet2, asset1, 2)
	updateWalletBalance(wallet3, asset2, 60)

	assertErr(t, engine.PlaceOrder(context.Background(), processor, order1))
	assertErr(t, engine.PlaceOrder(context.Background(), processor, order2))
	assertErr(t, engine.PlaceOrder(context.Background(), processor, order3))

	if processor.partial.ID() != "2" ||
		processor.partial.Quantity().(tFloat64) != 1 ||
		processor.done != 2 ||
		processor.priceDone != 30 ||
		processor.qtyDone != 2 ||
		engine.asks.prices["20"].volume.(tFloat64) != 1 {
		t.Fatalf("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset2) != 10 ||
		walletBalance(wallet2, asset2) != 20 ||
		walletBalance(wallet3, asset2) != 30 {
		t.Fatal("invalid result")
	}

	// --------------------------------------

	if walletBalance(wallet1, asset1) != 0 ||
		walletBalance(wallet2, asset1) != 0 ||
		walletInOrder(wallet2, asset1) != 1 ||
		walletBalance(wallet3, asset1) != 2 {
		t.Fatal("invalid result")
	}
}

func BenchmarkOrderProcessung(b *testing.B) {
	var (
		asset1, asset2 = Asset("apples"), Asset("dollars")
		ctx            = context.Background()
		wallet1        = newWallet()
		wallet2        = newWallet()
		wallet3        = newWallet()
		wallet4        = newWallet()
	)

	start := time.Now()
	for i := 0; i < b.N; i++ {
		var (
			engine = NewEngine(asset1, asset2)
		)

		wallet1.UpdateBalance(ctx, asset1, tFloat64(50))
		wallet2.UpdateBalance(ctx, asset2, tFloat64(1500))

		// 50 transactions
		for i := 100; i >= 60; i = i - 10 {
			for j := 0; j < 10; j++ {
				if err := engine.PlaceOrder(context.Background(), nil,
					newOrder(
						strconv.Itoa(i)+"-"+strconv.Itoa(j),
						wallet1,
						true,
						1,
						float64(i),
					)); err != nil {
					b.Fatal(err)
				}
			}
		}

		// 50 transactions
		for i := 50; i >= 10; i = i - 10 {
			for j := 0; j < 10; j++ {
				if err := engine.PlaceOrder(context.Background(), nil,
					newOrder(
						strconv.Itoa(i)+"-"+strconv.Itoa(j),
						wallet2,
						false,
						1,
						float64(i),
					)); err != nil {
					b.Fatal(err)
				}
			}
		}

		// 1 transaction
		wallet3.UpdateBalance(ctx, asset1, tFloat64(50))
		if err := engine.PlaceOrder(context.Background(), nil,
			newOrder(
				"sellMarket",
				wallet3,
				true,
				50,
				0,
			)); err != nil {
			b.Fatal(err)
		}

		// 1 transaction
		wallet4.UpdateBalance(ctx, asset2, tFloat64(4000))
		if err := engine.PlaceOrder(context.Background(), nil,
			newOrder(
				"buyMarket",
				wallet4,
				false,
				50,
				0,
			)); err != nil {
			b.Fatal(err)
		}
	}

	elapsed := time.Since(start)
	b.Logf("N: %d, Elapsed: %v, TPS: %f", b.N, elapsed, float64(b.N*54)/elapsed.Seconds())

}

func TestSetFeeHandler(t *testing.T) {
	var (
		asset1, asset2 = Asset("apples"), Asset("dollars")
		engine         = NewEngine(asset1, asset2)
	)
	engine.SetFeeHandler(&emptyFeeHandler{})
}

func TestPlaceOrderErrors(t *testing.T) {
	var (
		processor                 = newEventListener()
		asset1, asset2            = Asset("apples"), Asset("dollars")
		wallet1, wallet2, wallet3 = newWallet(), newWallet(), newWallet()

		engine = NewEngine(asset1, asset2)

		order1 = newOrder(
			"1",
			wallet1,
			false,
			-1,
			10,
		)
		order2 = newOrder(
			"2",
			wallet2,
			false,
			1,
			-20,
		)
		order3 = newOrder(
			"3",
			wallet3,
			true,
			2,
			100,
		)
		order4 = newOrder(
			"4",
			wallet3,
			true,
			2,
			0,
		)
		order5 = newOrder(
			"5",
			wallet1,
			false,
			2,
			0,
		)
		order6 = newOrder(
			"6",
			wallet1,
			false,
			5,
			2,
		)
		order7 = newOrder(
			"7",
			wallet3,
			true,
			3,
			0,
		)
	)

	updateWalletBalance(wallet1, asset2, 10)
	updateWalletBalance(wallet2, asset2, 20)
	updateWalletBalance(wallet3, asset1, 2)

	if err := engine.PlaceOrder(context.Background(), processor, order1); err == nil {
		t.Fatal()
	}
	if err := engine.PlaceOrder(context.Background(), processor, order2); err == nil {
		t.Fatal()
	}

	if err := engine.PlaceOrder(context.Background(), processor, order3); err != nil {
		t.Fatal(err)
	}
	if err := engine.PlaceOrder(context.Background(), processor, order3); err == nil {
		t.Fatal()
	}
	if err := engine.PlaceOrder(context.Background(), processor, order5); err == nil {
		t.Fatal()
	}

	engine.CancelOrder(context.Background(), processor, order3)

	if err := engine.PlaceOrder(context.Background(), processor, order4); err == nil {
		t.Fatal()
	}

	if err := engine.PlaceOrder(context.Background(), processor, order6); err != nil {
		t.Fatal(err)
	}
	if err := engine.PlaceOrder(context.Background(), processor, order7); err == nil {
		t.Fatal()
	}
}

func TestMiscFunctions(t *testing.T) {
	var (
		processor        = newEventListener()
		asset1, asset2   = Asset("apples"), Asset("dollars")
		wallet1, wallet2 = newWallet(), newWallet()

		engine = NewEngine(asset1, asset2)

		order1 = newOrder(
			"1",
			wallet1,
			true,
			1,
			20,
		)
		order2 = newOrder(
			"2",
			wallet2,
			false,
			1,
			10,
		)
		order3 = newOrder(
			"3",
			wallet2,
			false,
			1,
			10,
		)
	)

	updateWalletBalance(wallet1, asset1, 2)
	updateWalletBalance(wallet2, asset2, 20)

	if err := engine.PlaceOrder(context.Background(), processor, order1); err != nil {
		t.Fatal(err)
	}
	if err := engine.PlaceOrder(context.Background(), processor, order2); err != nil {
		t.Fatal(err)
	}

	t.Log(engine.Quantity(true, tFloat64(10.0)))
	t.Log(engine.Price(true, tFloat64(1.0)))
	t.Log(engine.Quantity(false, tFloat64(10.0)))
	t.Log(engine.Price(false, tFloat64(1.0)))
	t.Log(engine.Spread())
	t.Log(engine.Orders())
	t.Log(engine.FindOrder("1"))
	t.Log(engine.FindOrder("10"))
	engine.OrderBook(func(asks bool, price, volume Value, len int) { t.Log(asks, price, volume, len) })
	engine.pull(context.Background(), order3)
	engine.PushOrder(context.Background(), order1)
	l := emptyListener{}
	l.OnIncomingOrderPartial(context.Background(), &tOrder{}, Volume{})
	l.OnIncomingOrderDone(context.Background(), &tOrder{}, Volume{})
	l.OnIncomingOrderPlaced(context.Background(), &tOrder{})
	l.OnExistingOrderPartial(context.Background(), &tOrder{}, Volume{})
	l.OnExistingOrderDone(context.Background(), &tOrder{}, Volume{})
	l.OnExistingOrderCanceled(context.Background(), &tOrder{})
	l.OnBalanceChanged(context.Background(), &tWallet{}, asset1, tFloat64(0.0))
	l.OnInOrderChanged(context.Background(), &tWallet{}, asset1, tFloat64(0.0))
}

func newWithIntComparator() *rbTree {
	return &rbTree{comp: func(a, b interface{}) int {
		aAsserted := a.(int)
		bAsserted := b.(int)
		switch {
		case aAsserted > bAsserted:
			return 1
		case aAsserted < bAsserted:
			return -1
		default:
			return 0
		}
	}}
}

func newWithStringComparator() *rbTree {
	return &rbTree{comp: func(a, b interface{}) int {
		s1 := a.(string)
		s2 := b.(string)
		min := len(s2)
		if len(s1) < len(s2) {
			min = len(s1)
		}
		diff := 0
		for i := 0; i < min && diff == 0; i++ {
			diff = int(s1[i]) - int(s2[i])
		}
		if diff == 0 {
			diff = len(s1) - len(s2)
		}
		if diff < 0 {
			return -1
		}
		if diff > 0 {
			return 1
		}
		return 0
	}}
}

func TestRedBlackTreePut(t *testing.T) {
	tree := newWithIntComparator()
	tree.put(5, "e")
	tree.put(6, "f")
	tree.put(7, "g")
	tree.put(3, "c")
	tree.put(4, "d")
	tree.put(1, "x")
	tree.put(2, "b")
	tree.put(1, "a") //overwrite

	tree = newWithIntComparator()
	tree.put(1, "a")
	tree.put(5, "e")
	tree.put(6, "f")
	tree.put(7, "g")
	tree.put(3, "c")
	tree.put(4, "d")
	tree.put(1, "x") // overwrite
	tree.put(2, "b")

	tree = newWithIntComparator()
	tree.put(5, "e")
	tree.put(6, "f")
	tree.put(7, "g")
	tree.put(3, "c")
	tree.put(4, "d")
	tree.put(1, "x")
	tree.put(2, "b")

	tree = newWithIntComparator()
	tree.put(5, "e")
	tree.put(6, "f")
	tree.put(7, "g")
	tree.put(3, "c")
	tree.put(4, "d")
	tree.put(1, "x")
	tree.put(2, "b")
	tree.put(1, "a") //overwrite

	tree = newWithIntComparator()
	tree.put(5, "e")
	tree.put(6, "f")
	tree.put(7, "g")
	tree.put(3, "c")
	tree.put(4, "d")
	tree.put(1, "x")
	tree.put(2, "b")
	tree.put(1, "a") //overwrite

	tree = newWithIntComparator()
	tree.put(13, 5)
	tree.put(8, 3)
	tree.put(17, 7)
	tree.put(1, 1)
	tree.put(11, 4)
	tree.put(15, 6)
	tree.put(25, 9)
	tree.put(6, 2)
	tree.put(22, 8)
	tree.put(27, 10)

	tree = newWithStringComparator()
	tree.put("c", "3")
	tree.put("b", "2")
	tree.put("a", "1")
}
