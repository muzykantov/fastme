# fastme (fast matching engine) library

## Design

The library solves the task of recalculating the exchange order book. A classic matching engine consists of limit orders (orders) for buying or selling (exchanging) some commodity for another commodity, usually currency.

The distinctive feature of the library is that:
- No predetermined price levels
- There is no restriction on the types of numeric values used
- When working within integers, performance is achieved up to 500k transactions per second
- Contains all necessary dependencies
- There is support for write-off of commissions like maker and taker for working on cryptocurrency assets

## Description of interfaces and data types

All mathematical recalculation operations of the order book are designed to work in the system of business logic, so the engine itself works with the input interfaces and notifies the business logic about the events that occurred during the exchange of goods. BL decides what to do with this data (e.g., write off funds from the corresponding wallet). For the correct operation of the exchange it is necessary to implement the interfaces described in the file ```echange.go``.


### Asset

```Go
type Asset string
```

Describes the name of some exchange asset. Matching engine notifies the business logic of the event with the product and Asset type, not string, for clarity.


### Volume

```Go
type Volume struct {
	Price    Value
	Quantity Value
}
```

When making a transaction or closing a position, the matching engine also notifies about the volume of the transaction of Volume type. It contains the value of the amount and quantity of the exchange asset.


### Value

```Go
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
```

For recalculation and storage of numbers in the order of the order book it is necessary to implement certain mathematical operations. To dwell on a certain type of data (eg int.Big) is obviously inconvenient because of the subsequent severe restriction on the related type. Therefore, the type ```Value``` has appeared, in which you can wrap this interface (example in the file ```engine_test.go``).


#### Add, Sub и Mul
Must return a new object, not a changed current one.

#### Cmp и Sign
Must return the sign of a number to be able to compare the two numbers, as indicated in the help.

#### Hash
Does not have to return the hash number. It is enough to return a unique string for a specific single number. In most cases, the conversion of a number to a string will do. This function is used to store price levels and add orders to the queue at a specific price. This interface is enough to implement all the mathematics of exchange.


### Wallet

```Go
type Wallet interface {
	// Balance returns current wallet balance for given asset
	Balance(context.Context, Asset) Value

	// UpdateBalance calls by matching engine to update wallet balance
	UpdateBalance(context.Context, Asset, Value)

	// InOrder returns amount of asset in order
	InOrder(context.Context, Asset) Value

	// UpdateInOrder calls by matching engine to inform about freezed amount in order
	UpdateInOrder(context.Context, Asset, Value)
}
```


In the course of the transaction, the matching engine uses the interface ```Wallet``` in order to correctly transfer or write off funds in exchange wallets. In a wallet there is an obligatory balance ```Balance```, which contains the current balance of a wallet, and also ```InOrder``` balance, which denotes the quantity of an asset in an order book.

Functions are called by the engine when calculating the request. __Context__ is passed through from the functions of placing and canceling an order, so it is possible to place, for example, database record objects.

Functions marked as optional may be plugged and do not affect the operation of the engine, but are useful for taking into account the number of assets in the order book when calculating the exchange business logic.


#### Balance и InOrder 
Must return the current balance of the wallet for further recalculation operations.

#### UpdateBalance и UpdateInOrder 
Must update the balance of the wallet when the transaction.


### Order

```Go
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
```

The main unit of exchange is an order. Limit requests are stored in the RAM of the matching engine and are deleted from it as soon as the specified quantity is completed. For engine operation it is necessary to implement the specified methods.

#### ID
Must return the unique identifier of the order. In case of repetition, the request will be rejected.

#### Owner
Must return wallet, which owns this order. This method can be implemented in various ways, for example, through the wallet manager when creating an order. Or keep a pointer to the wallet in the request and return it.

#### Sell
Returns true if the order for sale.

#### Price
Returns the desired limit value at which you are willing to buy or sell the asset. If the value is zero, the order will be fully executed at the current market price (market order) if there are sufficient funds in your wallet to buy or sell.

#### Quantity
Must return the remaining amount in the unfulfilled order.

#### UpdateQuantity
It is called by the engine when recalculating and executing a transaction, notifying the business logic about the current amount of the asset remaining.


### FeeHandler

```Go
type FeeHandler interface {
	// HandleFeeMaker calls by  matching engine and provide data to correct output value for fee processing
	HandleFeeMaker(context.Context, Order, Asset, Value) (out Value)

	// HandleFeeTaker calls by  matching engine and provide data to correct output value for fee processing
	HandleFeeTaker(context.Context, Order, Asset, Value) (out Value)
}
```

Called at the time of the transaction in order to write off the commission. In the current version, the commission is written off by processing. For example, 1 unit of goods has been sold and 0.9 units will be credited to the balance due to the specifics of the engine. This parameter is optional. The handler is divided into Maker - market maker. That is a limit order. And Taker - a market order (or a market part of a limit order).

#### HandleFeeMaker, HandleFeeTaker
The context of a constraint, information on a certain asset transaction is received at the entrance. The function should return inValue - feeValue to out value.


## EventListener

```Go
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
```

The interface takes part in the operations of placing and withdrawing an order. It is used to notify the higher business logic about all events that occurred during order processing. There are 2 order types - incoming and existing. Existing order is a limit order (with it's balance), which is located in the order book. 

#### OnIncomingOrderPartial
Called at partial execution of an incoming order.

#### OnIncomingOrderDone
Called when an incoming order is completed.

#### OnIncomingOrderPlaced
Called when the incoming order (or the balance of partial execution) has entered the queue of the matching engine.

#### OnExistingOrderPartial
Called at partial limit order execution.

#### OnExistingOrderDone
Called when the limit order is completed.

#### OnExistingOrderCanceled
Called when the limit order is canceled.

#### OnBalanceChanged
Called when the balance on your wallet changes as a result of processing the order. Notifies the recipient about the new balance.

#### OnInOrderChanged
Called in case of change of balance in orders on your wallet as a result of processing the order. Notifies the recipient of the new value of the amount of asset in the order book.

## Functionality

The main functions of processing incoming orders have been implemented to manage the order book.


#### func NewEngine(base, quote Asset) *Engine
Initializes the matching engine. Allocates memory for data.

#### func (e *Engine) SetFeeHandler(h FeeHandler)
Sets the fee handler.

#### func (e *Engine) PlaceOrder(ctx context.Context, listener EventListener, o Order) (err error)
Processes incoming order. Recalculates the state of the order book. Returns the following errors in case of impossibility to process the order:
```Go
var (
	ErrInvalidQuantity      = errors.New("Quantity could not be less or equal zero")
	ErrInvalidPrice         = errors.New("Price could not be less zero")
	ErrInsufficientQuantity = errors.New("Insufficient quantity to calculate market price")
	ErrInsufficientFunds    = errors.New("Insufficient funds to process order")
	ErrOrderExists          = errors.New("Order with given ID already exists")
	ErrOrderNotFound        = errors.New("Order with given ID not found")
)
```

#### func (e *Engine) CancelOrder(ctx context.Context, listener EventListener, o Order)
Removes the specified order from the order book.

#### func (e *Engine) PushOrder(ctx context.Context, o Order)
Puts the order in a queue without applying mathematical recalculation. It is used to restore the glass from the database.

#### func (e *Engine) Quantity(sell bool, priceLim Value) Value
Returns the volume of the asset to a certain price.

#### func (e *Engine) Price(sell bool, quantity Value) (Value, error)
Returns the market value for the specified amount of asset.

#### func (e *Engine) Spread() (bestAsk, bestBid Value)
Returns the value of the price spread in the order book. nil if there are no bids in one or two directions.

#### func (e *Engine) FindOrder(id string) (Order, error)
Returns an order by its identifier or error  ```ErrOrderNotFound```

#### func (e *Engine) Orders() (orders []Order)
Returns the list of limit orders that are in the order book.

#### func (e *Engine) OrderBook(iter func(asks bool, price, volume Value, len int))
Iterates price levels by returning information about price, order volume and queue length.


## Work algorithm

The engine structure contains a linked list of orders and a search tree for the optimal price to insert or place.