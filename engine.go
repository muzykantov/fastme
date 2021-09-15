package fastme

import (
	"container/list"
	"context"
	"errors"
	"sync"
)

// Fast matching engine errors
var (
	//lint:ignore ST1005 for backward compatibility
	ErrInvalidQuantity = errors.New("Invalid order quantity")

	//lint:ignore ST1005 for backward compatibility
	ErrInvalidPrice = errors.New("Invalid order price")

	//lint:ignore ST1005 for backward compatibility
	ErrInvalidOrder = errors.New("Invalid order format")

	//lint:ignore ST1005 for backward compatibility
	ErrInsufficientQuantity = errors.New("Insufficient quantity to calculate market price")

	//lint:ignore ST1005 for backward compatibility
	ErrInsufficientFunds = errors.New("Insufficient funds to process order")

	ErrOrderExists = errors.New("Order with given ID already exists")

	ErrOrderNotFound = errors.New("Order with given ID not found")
)

// Engine implements fast matching engine
type Engine struct {
	base       Asset
	quote      Asset
	orders     map[string]*list.Element // OrderID() -> *list.Element.Value.(Order)
	asks       *side
	bids       *side
	feeHandler FeeHandler
	m          sync.Mutex
}

// NewEngine creates fast matching engine implementation
func NewEngine(base, quote Asset) *Engine {
	return &Engine{
		base:   base,
		quote:  quote,
		orders: make(map[string]*list.Element),
		asks:   newSide(),
		bids:   newSide(),
	}
}

// NewEngineWithFeeHandler creates fast matching engine implementation
func NewEngineWithFeeHandler(base, quote Asset, h FeeHandler) (me *Engine) {
	me = NewEngine(base, quote)
	me.SetFeeHandler(h)
	return
}

// ----------------------------------------------------------
// Matching engine implementation
// ----------------------------------------------------------

// SetFeeHandler updates fee handlers
func (e *Engine) SetFeeHandler(h FeeHandler) {
	e.m.Lock()
	e.feeHandler = h
	e.m.Unlock()
}

// CanPlace calculates balance and retuns an error if is not enought money
// to place an order with given params
func (e *Engine) CanPlace(
	ctx context.Context,
	w Wallet,
	sell bool,
	quantity, price Value,
) error {
	if quantity == nil || quantity.Sign() <= 0 {
		return ErrInvalidQuantity
	}

	if price == nil || price.Sign() < 0 {
		return ErrInvalidPrice
	}

	var (
		marketPrice Value
		err         error
	)
	if price.Sign() == 0 {
		if marketPrice, err = e.price(sell, quantity); err != nil {
			return err
		}
	} else {
		marketPrice = price.Mul(quantity)
	}

	if sell {
		if w == nil || w.Balance(ctx, e.base).Cmp(quantity) < 0 {
			return ErrInsufficientFunds
		}
	} else {
		if w == nil || w.Balance(ctx, e.quote).Cmp(marketPrice) < 0 {
			return ErrInsufficientFunds
		}
	}

	return nil
}

// PlaceOrder order adds the order to the order book and solves exchange task
func (e *Engine) PlaceOrder(
	ctx context.Context,
	listener EventListener,
	o Order,
) (err error) {
	e.m.Lock()
	defer e.m.Unlock()

	if listener == nil {
		listener = emptyListenerValue
	}

	if e.feeHandler == nil {
		e.feeHandler = emptyFeeHandlerValue
	}

	if _, ok := e.orders[o.ID()]; ok {
		return ErrOrderExists
	}

	if err := e.CanPlace(
		ctx,
		o.Owner(),
		o.Sell(),
		o.Quantity(),
		o.Price(),
	); err != nil {
		return err
	}

	var (
		next    func() *queue
		compare func(Value) bool
	)

	if o.Sell() {
		next = e.bids.maxPrice
		compare = func(n Value) bool {
			return o.Price().Cmp(n) <= 0
		}
	} else {
		next = e.asks.minPrice
		compare = func(n Value) bool {
			return o.Price().Cmp(n) >= 0
		}
	}

	if o.Price().Sign() == 0 {
		compare = func(Value) bool { return true }
	}

	// Side processing
	bestPriceQueue := next()
	for bestPriceQueue != nil &&
		o.Quantity().Sign() > 0 &&
		compare(bestPriceQueue.price) {

		// Queue processing
		for bestPriceQueue.orders.Len() > 0 &&
			o.Quantity().Sign() > 0 {
			var (
				makerEl = bestPriceQueue.orders.Front()
				maker   = makerEl.Value.(Order)
				taker   = o

				makerQty = maker.Quantity()
				takerQty = taker.Quantity()
				volume   Volume
			)

			// Matching
			switch taker.Quantity().Cmp(maker.Quantity()) {
			case 0: // taker qty == maker qty
				e.pull(ctx, maker)
				volume = Volume{
					Price:    makerQty.Mul(maker.Price()),
					Quantity: makerQty,
				}

				maker.UpdateQuantity(makerQty.Sub(makerQty))
				taker.UpdateQuantity(takerQty.Sub(takerQty))
				listener.OnExistingOrderDone(ctx, maker, volume)
				listener.OnIncomingOrderDone(ctx, taker, volume)

			case 1: // taker qty > maker qty
				e.pull(ctx, maker)
				volume = Volume{
					Price:    makerQty.Mul(maker.Price()),
					Quantity: makerQty,
				}

				maker.UpdateQuantity(makerQty.Sub(makerQty))
				taker.UpdateQuantity(takerQty.Sub(makerQty))
				listener.OnExistingOrderDone(ctx, maker, volume)
				listener.OnIncomingOrderPartial(ctx, taker, volume)

			case -1: // taker qty < maker qty
				volume = Volume{
					Price:    takerQty.Mul(maker.Price()),
					Quantity: takerQty,
				}

				bestPriceQueue.updateQuantity(
					ctx,
					makerEl,
					makerQty.Sub(takerQty),
				)
				taker.UpdateQuantity(takerQty.Sub(takerQty))
				listener.OnExistingOrderPartial(ctx, maker, volume)
				listener.OnIncomingOrderDone(ctx, taker, volume)
			}

			e.updateBalanceOnExchanged(
				ctx,
				listener,
				maker,
				volume,
				true,
			)

			e.updateBalanceOnExchanged(
				ctx,
				listener,
				taker,
				volume,
				false,
			)
		}

		bestPriceQueue = next()
	}

	if o.Quantity().Sign() > 0 {
		e.push(ctx, o)
		listener.OnIncomingOrderPlaced(ctx, o)
		e.updateBalanceOnPlaced(ctx, listener, o)
	}

	return nil
}

// ReplaceOrder replaces order at the same price level without queue loss
func (e *Engine) ReplaceOrder(
	ctx context.Context,
	listener EventListener,
	o, n Order,
) error {
	e.m.Lock()
	defer e.m.Unlock()

	orderEl, ok := e.orders[o.ID()]
	if !ok {
		return ErrOrderNotFound
	}

	o, ok = orderEl.Value.(Order)
	if !ok {
		return ErrInvalidOrder
	}

	if o.Owner() != n.Owner() {
		return ErrInvalidOrder
	}

	if o.Sell() != n.Sell() {
		return ErrInvalidOrder
	}

	if o.Price().Cmp(n.Price()) != 0 {
		return ErrInvalidOrder
	}

	if n.Quantity().Sign() <= 0 {
		return ErrInvalidQuantity
	}

	if listener == nil {
		listener = emptyListenerValue
	}

	var (
		wallet     = o.Owner()
		asset      Asset
		newBalance Value
		newInOrder Value
		oldValue   Value
		newValue   Value
		orderSide  *side
	)

	if o.Sell() {
		orderSide = e.asks
		asset = e.base
		oldValue = o.Quantity()
		newValue = n.Quantity()
	} else {
		orderSide = e.bids
		asset = e.quote
		oldValue = o.Price().Mul(o.Quantity())
		newValue = n.Price().Mul(n.Quantity())
	}

	newBalance = oldValue.
		Sub(newValue).
		Add(wallet.Balance(ctx, asset))

	if newBalance.Sign() < 0 {
		return ErrInsufficientFunds
	}

	queue, ok := orderSide.prices[n.Price().Hash()]
	if !ok {
		return ErrInvalidPrice
	}

	newInOrder = newValue.
		Sub(oldValue).
		Add(wallet.InOrder(ctx, asset))

	orderEl.Value = n

	delete(e.orders, o.ID())
	e.orders[n.ID()] = orderEl

	queue.volume = n.Quantity().
		Sub(o.Quantity()).
		Add(queue.volume)

	wallet.UpdateBalance(ctx, asset, newBalance)
	listener.OnBalanceChanged(ctx, wallet, asset, newBalance)

	wallet.UpdateInOrder(ctx, asset, newInOrder)
	listener.OnInOrderChanged(ctx, wallet, asset, newInOrder)

	return nil
}

// CancelOrder removes order from the order book and refund assets to the owner
func (e *Engine) CancelOrder(
	ctx context.Context,
	listener EventListener,
	o Order,
) {
	e.m.Lock()
	defer e.m.Unlock()

	if listener == nil {
		listener = emptyListenerValue
	}

	e.pull(ctx, o)

	var (
		wallet = o.Owner()
		value  Value
		asset  Asset
	)

	if o.Sell() {
		value = o.Quantity()
		asset = e.base
	} else {
		value = o.Quantity().Mul(o.Price())
		asset = e.quote
	}

	valBalance := value.Add(wallet.Balance(ctx, asset))
	wallet.UpdateBalance(ctx, asset, valBalance)
	listener.OnBalanceChanged(ctx, wallet, asset, valBalance)

	valInOrder := wallet.InOrder(ctx, asset).Sub(value)
	wallet.UpdateInOrder(ctx, asset, valInOrder)
	listener.OnInOrderChanged(ctx, wallet, asset, valInOrder)

	listener.OnExistingOrderCanceled(ctx, o)
}

// PushOrder puts the order to the queue without any calculations
func (e *Engine) PushOrder(ctx context.Context, o Order) {
	e.m.Lock()
	e.push(ctx, o)
	e.m.Unlock()
}

// Quantity returns quantity for price limit
func (e *Engine) Quantity(sell bool, priceLim Value) Value {
	e.m.Lock()
	defer e.m.Unlock()

	return e.quantity(sell, priceLim)
}

// Price returns market price of given quantity
func (e *Engine) Price(sell bool, quantity Value) (Value, error) {
	e.m.Lock()
	defer e.m.Unlock()

	return e.price(sell, quantity)
}

// Spread returns best bid and best ask
func (e *Engine) Spread() (bestAsk, bestBid Value) {
	e.m.Lock()
	defer e.m.Unlock()

	asksQueue := e.asks.minPrice()
	bidsQueue := e.bids.maxPrice()

	if asksQueue != nil {
		bestAsk = asksQueue.price
	}

	if bidsQueue != nil {
		bestBid = bidsQueue.price
	}

	return
}

// FindOrder returns order bygiven ID
func (e *Engine) FindOrder(id string) (Order, error) {
	e.m.Lock()
	defer e.m.Unlock()

	el, ok := e.orders[id]
	if !ok {
		return nil, ErrOrderNotFound
	}

	return el.Value.(Order), nil
}

// Orders returns all existing limit orders
func (e *Engine) Orders() (orders []Order) {
	e.m.Lock()
	defer e.m.Unlock()

	for _, order := range e.orders {
		orders = append(orders, order.Value.(Order))
	}

	return
}

// OrderBook returns information about volume and price for definite price level
func (e *Engine) OrderBook(iter func(asks bool, price, volume Value, len int)) {
	e.m.Lock()
	defer e.m.Unlock()

	level := e.asks.maxPrice()
	for level != nil {
		iter(true, level.price, level.volume, level.orders.Len())
		level = e.asks.lessThan(level.price)
	}

	level = e.bids.maxPrice()
	for level != nil {
		iter(false, level.price, level.volume, level.orders.Len())
		level = e.bids.lessThan(level.price)
	}
}

func (e *Engine) quantity(sell bool, priceLim Value) Value {
	var (
		level    *queue
		iter     func(Value) *queue
		quantity Value
	)

	if sell {
		level = e.bids.maxPrice()
		iter = e.bids.lessThan
	} else {
		level = e.asks.minPrice()
		iter = e.asks.greaterThan
	}

	for level != nil {
		if priceLim != nil &&
			((sell && level.price.Cmp(priceLim) < 0) ||
				(!sell && level.price.Cmp(priceLim) > 0)) {
			break
		}

		quantity = level.volume.Add(quantity)
		level = iter(level.price)
	}

	return quantity
}

func (e *Engine) price(sell bool, quantity Value) (Value, error) {
	var (
		level *queue
		iter  func(Value) *queue
		price Value
	)

	if sell {
		level = e.bids.maxPrice()
		iter = e.bids.lessThan
	} else {
		level = e.asks.minPrice()
		iter = e.asks.greaterThan
	}

	for quantity.Sign() > 0 && level != nil {
		if quantity.Cmp(level.volume) < 0 {
			return level.price.Mul(quantity).Add(price), nil
		}

		price = level.price.Mul(level.volume).Add(price)
		quantity = quantity.Sub(level.volume)
		level = iter(level.price)
	}

	if quantity.Sign() > 0 {
		return nil, ErrInsufficientQuantity
	}

	return price, nil
}

func (e *Engine) updateBalanceOnExchanged(
	ctx context.Context,
	listener EventListener,
	o Order,
	v Volume,
	isMaker bool,
) {
	var (
		wallet             = o.Owner()
		assetInc, assetDec Asset
		valueInc, valueDec Value
	)

	if o.Sell() {
		assetInc = e.quote
		assetDec = e.base
		valueInc = v.Price
		valueDec = v.Quantity
	} else {
		assetInc = e.base
		assetDec = e.quote
		valueInc = v.Quantity
		valueDec = v.Price
	}

	if isMaker {
		valueInc = e.feeHandler.HandleFeeMaker(ctx, o, assetInc, valueInc)
	} else {
		valueInc = e.feeHandler.HandleFeeTaker(ctx, o, assetInc, valueInc)
	}

	valBalance := valueInc.Add(wallet.Balance(ctx, assetInc))
	wallet.UpdateBalance(ctx, assetInc, valBalance)
	listener.OnBalanceChanged(ctx, wallet, assetInc, valBalance)

	if isMaker {
		valInOrder := wallet.InOrder(ctx, assetDec).Sub(valueDec)
		wallet.UpdateInOrder(ctx, assetDec, valInOrder)
		listener.OnInOrderChanged(ctx, wallet, assetDec, valInOrder)
	} else {
		valInOrder := wallet.Balance(ctx, assetDec).Sub(valueDec)
		wallet.UpdateBalance(ctx, assetDec, valInOrder)
		listener.OnBalanceChanged(ctx, wallet, assetDec, valInOrder)
	}
}

func (e *Engine) updateBalanceOnPlaced(
	ctx context.Context,
	listener EventListener,
	o Order,
) {
	var (
		wallet = o.Owner()
		asset  Asset
		value  Value
	)

	if o.Sell() {
		asset = e.base
		value = o.Quantity()
	} else {
		asset = e.quote
		value = o.Price().Mul(o.Quantity())
	}

	valBalance := wallet.Balance(ctx, asset).Sub(value)
	wallet.UpdateBalance(ctx, asset, valBalance)
	listener.OnBalanceChanged(ctx, wallet, asset, valBalance)

	valInOrder := value.Add(wallet.InOrder(ctx, asset))
	wallet.UpdateInOrder(ctx, asset, valInOrder)
	listener.OnInOrderChanged(ctx, wallet, asset, valInOrder)
}

func (e *Engine) push(ctx context.Context, o Order) {
	if o.Sell() {
		e.orders[o.ID()] = e.asks.append(ctx, o)
	} else {
		e.orders[o.ID()] = e.bids.append(ctx, o)
	}
}

func (e *Engine) pull(ctx context.Context, o Order) {
	el, ok := e.orders[o.ID()]
	if !ok {
		return
	}

	if el.Value.(Order).Sell() {
		e.asks.remove(ctx, el)
	} else {
		e.bids.remove(ctx, el)
	}

	delete(e.orders, o.ID())
}

// ----------------------------------------------------------
// Order side implementation
// ----------------------------------------------------------

type side struct {
	prices    map[string]*queue
	priceTree *rbTree
	numOrders int
	depth     int
}

func newSide() *side {
	return &side{
		priceTree: newRBTree(func(a, b interface{}) int {
			return a.(Value).Cmp(b.(Value))
		}),
		prices: make(map[string]*queue),
	}
}

func (s *side) append(ctx context.Context, o Order) *list.Element {
	p := o.Price()
	h := p.Hash()

	q, ok := s.prices[h]
	if !ok {
		q = newQueue(p)
		s.prices[h] = q
		s.priceTree.put(p, q)
		s.depth++
	}

	s.numOrders++
	return q.append(ctx, o)
}

func (s *side) remove(ctx context.Context, e *list.Element) (o Order) {
	p := e.Value.(Order).Price()
	h := p.Hash()

	q := s.prices[h]
	o = q.remove(ctx, e)

	if q.orders.Len() == 0 {
		delete(s.prices, h)
		s.priceTree.remove(p)
		s.depth--
	}

	s.numOrders--
	return
}

func (s *side) maxPrice() *queue {
	if s.depth > 0 {
		if value, found := s.priceTree.getMax(); found {
			return value.(*queue)
		}
	}
	return nil
}

func (s *side) minPrice() *queue {
	if s.depth > 0 {
		if value, found := s.priceTree.getMin(); found {
			return value.(*queue)
		}
	}
	return nil
}

func (s *side) greaterThan(price Value) *queue {
	tree := s.priceTree
	node := tree.root

	var ceiling *rbtNode
	for node != nil {
		if tree.comp(price, node.Key) < 0 {
			ceiling = node
			node = node.Left
		} else {
			node = node.Right
		}
	}

	if ceiling != nil {
		return ceiling.Value.(*queue)
	}

	return nil
}

func (s *side) lessThan(price Value) *queue {
	tree := s.priceTree
	node := tree.root

	var floor *rbtNode
	for node != nil {
		if tree.comp(price, node.Key) > 0 {
			floor = node
			node = node.Right
		} else {
			node = node.Left
		}
	}

	if floor != nil {
		return floor.Value.(*queue)
	}

	return nil
}

type emptyListener struct{}

func (l *emptyListener) OnIncomingOrderPartial(context.Context, Order, Volume)  {}
func (l *emptyListener) OnIncomingOrderDone(context.Context, Order, Volume)     {}
func (l *emptyListener) OnIncomingOrderPlaced(context.Context, Order)           {}
func (l *emptyListener) OnExistingOrderPartial(context.Context, Order, Volume)  {}
func (l *emptyListener) OnExistingOrderDone(context.Context, Order, Volume)     {}
func (l *emptyListener) OnExistingOrderCanceled(context.Context, Order)         {}
func (l *emptyListener) OnBalanceChanged(context.Context, Wallet, Asset, Value) {}
func (l *emptyListener) OnInOrderChanged(context.Context, Wallet, Asset, Value) {}

var emptyListenerValue = new(emptyListener)

type emptyFeeHandler struct{}

func (h *emptyFeeHandler) HandleFeeMaker(
	ctx context.Context,
	o Order,
	a Asset,
	in Value,
) (out Value) {
	return in
}

func (h *emptyFeeHandler) HandleFeeTaker(ctx context.Context,
	o Order,
	a Asset,
	in Value,
) (out Value) {
	return in
}

var emptyFeeHandlerValue = new(emptyFeeHandler)

// ----------------------------------------------------------
// Order queue implementation
// ----------------------------------------------------------

type queue struct {
	volume Value
	price  Value
	orders *list.List
}

func newQueue(price Value) *queue {
	return &queue{
		volume: nil,
		price:  price,
		orders: list.New(),
	}
}

func (q *queue) append(ctx context.Context, o Order) *list.Element {
	q.volume = o.Quantity().Add(q.volume)
	return q.orders.PushBack(o)
}

func (q *queue) remove(ctx context.Context, e *list.Element) Order {
	q.volume = q.volume.Sub(e.Value.(Order).Quantity())
	return q.orders.Remove(e).(Order)
}

func (q *queue) updateQuantity(ctx context.Context, e *list.Element, qty Value) Order {
	o := e.Value.(Order)
	q.volume = q.volume.Sub(o.Quantity()).Add(qty)
	o.UpdateQuantity(qty)
	return o
}

// ----------------------------------------------------------
// RedBlackTree implementation
// ----------------------------------------------------------

type color bool

const (
	black color = true
	red   color = false
)

// rbtNode is a single element within the tree
type rbtNode struct {
	Key    interface{}
	Value  interface{}
	color  color
	Left   *rbtNode
	Right  *rbtNode
	Parent *rbtNode
}

func (n *rbtNode) grandparent() *rbtNode {
	if n != nil && n.Parent != nil {
		return n.Parent.Parent
	}
	return nil
}

func (n *rbtNode) uncle() *rbtNode {
	if n == nil || n.Parent == nil || n.Parent.Parent == nil {
		return nil
	}
	return n.Parent.sibling()
}

func (n *rbtNode) sibling() *rbtNode {
	if n == nil || n.Parent == nil {
		return nil
	}
	if n == n.Parent.Left {
		return n.Parent.Right
	}
	return n.Parent.Left
}

func (n *rbtNode) maximumNode() *rbtNode {
	if n == nil {
		return nil
	}
	for n.Right != nil {
		n = n.Right
	}
	return n
}

// ----------------------------------------------------------

// comparator will make type assertion (see IntComparator for example),
// which will panic if a or b are not of the asserted type.
//
// Should return a number:
//    positive , if a > b
//    zero     , if a == b
//    negative , if a < b
type comparator func(a, b interface{}) int

// rbTree holds elements of the red-black tree
type rbTree struct {
	root *rbtNode
	comp comparator
	size int
}

// newRBTree instantiates a red-black tree with the custom comparator.
func newRBTree(comp comparator) *rbTree {
	return &rbTree{comp: comp}
}

// put inserts node into the tree.
// Key should adhere to the comparator's type assertion, otherwise method panics.
func (t *rbTree) put(key interface{}, value interface{}) {
	var insertedNode *rbtNode
	if t.root == nil {
		// Assert key is of comparator's type for initial tree
		t.comp(key, key)
		t.root = &rbtNode{Key: key, Value: value, color: red}
		insertedNode = t.root
	} else {
		node := t.root
		loop := true
		for loop {
			compare := t.comp(key, node.Key)
			switch {
			case compare == 0:
				node.Key = key
				node.Value = value
				return
			case compare < 0:
				if node.Left == nil {
					node.Left = &rbtNode{Key: key, Value: value, color: red}
					insertedNode = node.Left
					loop = false
				} else {
					node = node.Left
				}
			case compare > 0:
				if node.Right == nil {
					node.Right = &rbtNode{Key: key, Value: value, color: red}
					insertedNode = node.Right
					loop = false
				} else {
					node = node.Right
				}
			}
		}
		insertedNode.Parent = node
	}
	t.insertCase1(insertedNode)
	t.size++
}

// remove remove the node from the tree by key.
// Key should adhere to the comparator's type assertion, otherwise method panics.
func (t *rbTree) remove(key interface{}) {
	var child *rbtNode
	node := t.lookup(key)
	if node == nil {
		return
	}
	if node.Left != nil && node.Right != nil {
		pred := node.Left.maximumNode()
		node.Key = pred.Key
		node.Value = pred.Value
		node = pred
	}
	if node.Left == nil || node.Right == nil {
		if node.Right == nil {
			child = node.Left
		} else {
			child = node.Right
		}
		if node.color == black {
			node.color = nodeColor(child)
			t.deleteCase1(node)
		}
		t.replaceNode(node, child)
		if node.Parent == nil && child != nil {
			child.color = black
		}
	}
	t.size--
}

// getMin gets the min value and flag if found
func (t *rbTree) getMin() (value interface{}, found bool) {
	node, found := t.getMinFromNode(t.root)
	if node != nil {
		return node.Value, found
	}
	return nil, false
}

// getMax gets the max value and flag if found
func (t *rbTree) getMax() (value interface{}, found bool) {
	node, found := t.getMaxFromNode(t.root)
	if node != nil {
		return node.Value, found
	}
	return nil, false
}

func (t *rbTree) getMinFromNode(n *rbtNode) (foundNode *rbtNode, found bool) {
	if n == nil {
		return nil, false
	}
	if n.Left == nil {
		return n, true
	}
	return t.getMinFromNode(n.Left)
}

func (t *rbTree) getMaxFromNode(n *rbtNode) (foundNode *rbtNode, found bool) {
	if n == nil {
		return nil, false
	}
	if n.Right == nil {
		return n, true
	}
	return t.getMaxFromNode(n.Right)
}

func (t *rbTree) insertCase1(n *rbtNode) {
	if n.Parent == nil {
		n.color = black
	} else {
		t.insertCase2(n)
	}
}

func (t *rbTree) insertCase2(n *rbtNode) {
	if nodeColor(n.Parent) == black {
		return
	}
	t.insertCase3(n)
}

func (t *rbTree) insertCase3(n *rbtNode) {
	uncle := n.uncle()
	if nodeColor(uncle) == red {
		n.Parent.color = black
		uncle.color = black
		n.grandparent().color = red
		t.insertCase1(n.grandparent())
	} else {
		t.insertCase4(n)
	}
}

func (t *rbTree) insertCase4(n *rbtNode) {
	grandparent := n.grandparent()
	if n == n.Parent.Right && n.Parent == grandparent.Left {
		t.rotateLeft(n.Parent)
		n = n.Left
	} else if n == n.Parent.Left && n.Parent == grandparent.Right {
		t.rotateRight(n.Parent)
		n = n.Right
	}
	t.insertCase5(n)
}

func (t *rbTree) insertCase5(n *rbtNode) {
	n.Parent.color = black
	grandparent := n.grandparent()
	grandparent.color = red
	if n == n.Parent.Left && n.Parent == grandparent.Left {
		t.rotateRight(grandparent)
	} else if n == n.Parent.Right && n.Parent == grandparent.Right {
		t.rotateLeft(grandparent)
	}
}

func (t *rbTree) deleteCase1(n *rbtNode) {
	if n.Parent == nil {
		return
	}
	t.deleteCase2(n)
}

func (t *rbTree) deleteCase2(n *rbtNode) {
	sibling := n.sibling()
	if nodeColor(sibling) == red {
		n.Parent.color = red
		sibling.color = black
		if n == n.Parent.Left {
			t.rotateLeft(n.Parent)
		} else {
			t.rotateRight(n.Parent)
		}
	}
	t.deleteCase3(n)
}

func (t *rbTree) deleteCase3(n *rbtNode) {
	sibling := n.sibling()
	if nodeColor(n.Parent) == black &&
		nodeColor(sibling) == black &&
		nodeColor(sibling.Left) == black &&
		nodeColor(sibling.Right) == black {
		sibling.color = red
		t.deleteCase1(n.Parent)
	} else {
		t.deleteCase4(n)
	}
}

func (t *rbTree) deleteCase4(n *rbtNode) {
	sibling := n.sibling()
	if nodeColor(n.Parent) == red &&
		nodeColor(sibling) == black &&
		nodeColor(sibling.Left) == black &&
		nodeColor(sibling.Right) == black {
		sibling.color = red
		n.Parent.color = black
	} else {
		t.deleteCase5(n)
	}
}

func (t *rbTree) deleteCase5(n *rbtNode) {
	sibling := n.sibling()
	if n == n.Parent.Left &&
		nodeColor(sibling) == black &&
		nodeColor(sibling.Left) == red &&
		nodeColor(sibling.Right) == black {
		sibling.color = red
		sibling.Left.color = black
		t.rotateRight(sibling)
	} else if n == n.Parent.Right &&
		nodeColor(sibling) == black &&
		nodeColor(sibling.Right) == red &&
		nodeColor(sibling.Left) == black {
		sibling.color = red
		sibling.Right.color = black
		t.rotateLeft(sibling)
	}
	t.deleteCase6(n)
}

func (t *rbTree) deleteCase6(n *rbtNode) {
	sibling := n.sibling()
	sibling.color = nodeColor(n.Parent)
	n.Parent.color = black
	if n == n.Parent.Left && nodeColor(sibling.Right) == red {
		sibling.Right.color = black
		t.rotateLeft(n.Parent)
	} else if nodeColor(sibling.Left) == red {
		sibling.Left.color = black
		t.rotateRight(n.Parent)
	}
}

func (t *rbTree) rotateLeft(n *rbtNode) {
	right := n.Right
	t.replaceNode(n, right)
	n.Right = right.Left
	if right.Left != nil {
		right.Left.Parent = n
	}
	right.Left = n
	n.Parent = right
}

func (t *rbTree) rotateRight(n *rbtNode) {
	left := n.Left
	t.replaceNode(n, left)
	n.Left = left.Right
	if left.Right != nil {
		left.Right.Parent = n
	}
	left.Right = n
	n.Parent = left
}

func (t *rbTree) replaceNode(old *rbtNode, new *rbtNode) {
	if old.Parent == nil {
		t.root = new
	} else {
		if old == old.Parent.Left {
			old.Parent.Left = new
		} else {
			old.Parent.Right = new
		}
	}
	if new != nil {
		new.Parent = old.Parent
	}
}

func (t *rbTree) lookup(key interface{}) *rbtNode {
	node := t.root
	for node != nil {
		compare := t.comp(key, node.Key)
		switch {
		case compare == 0:
			return node
		case compare < 0:
			node = node.Left
		case compare > 0:
			node = node.Right
		}
	}
	return nil
}

func nodeColor(n *rbtNode) color {
	if n == nil {
		return black
	}
	return n.color
}
