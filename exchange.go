// Package fastme package implements limit matching engine.
// To process order you need to implement following interfaces
package fastme

import "context"

// Asset contains name of the asset
type Asset string

// Volume contains total price and quantity done for average price calculation
type Volume struct {
	Price    Value
	Quantity Value
}

// Value calcultes math operations
type Value interface {
	// Add is an "+" operation
	Add(Value) Value

	// Sub is an "-" operation
	Sub(Value) Value

	// Mul is an "*" operation
	Mul(Value) Value

	// Cmp returns 1 if self > given, -1 if self < given and 0 if self == given
	Cmp(Value) int

	// Sign returns 1 if Value > 0, -1 if Value < 0 and 0 if Value == 0
	Sign() int

	// Hash returns any string representation of the Value
	Hash() string
}

// Wallet describes interface for asset exchange operations
type Wallet interface {
	// Balance returns current wallet balance for given asset
	Balance(context.Context, Asset) Value

	// UpdateBalance calls by matching engine to update wallet balance
	UpdateBalance(context.Context, Asset, Value)

	// InOrder returns amount of asset in order (optional)
	InOrder(context.Context, Asset) Value

	// UpdateInOrder calls by matching engine to inform about freezed amount in order (optional)
	UpdateInOrder(context.Context, Asset, Value)
}

// Order is the extensible interface responsible for containig information about order
type Order interface {
	// ID returns any uinique string for order
	ID() string

	// Owner returns wallet to debit or credit asset on exchange process
	Owner() Wallet

	// Sell returns true if order for selling, true otherwise
	Sell() bool

	// Price retuns order price
	Price() Value

	// Quantity returns current order quantity
	Quantity() Value

	// UpdateQuantity calls by matching engine to set new order quantity
	UpdateQuantity(Value)
}

// EventListener informs subscriber to some matching changes
type EventListener interface {
	OnIncomingOrderPartial(context.Context, Order, Volume)
	OnIncomingOrderDone(context.Context, Order, Volume)
	OnIncomingOrderPlaced(context.Context, Order)

	OnExistingOrderPartial(context.Context, Order, Volume)
	OnExistingOrderDone(context.Context, Order, Volume)
	OnExistingOrderCanceled(context.Context, Order)

	OnBalanceChanged(context.Context, Wallet, Asset, Value)
	OnInOrderChanged(context.Context, Wallet, Asset, Value)
}

// FeeHandler responsible for fee calculations and fee wallet processing
type FeeHandler interface {
	// HandleFeeMaker calls by  matching engine and provide data to correct output value for fee processing
	HandleFeeMaker(context.Context, Order, Asset, Value) (out Value)

	// HandleFeeTaker calls by  matching engine and provide data to correct output value for fee processing
	HandleFeeTaker(context.Context, Order, Asset, Value) (out Value)
}
