# Библиотека fastme (fast matching engine)

[![Go Report Card](https://goreportcard.com/badge/github.com/newity/fastme)](https://goreportcard.com/report/github.com/newity/fastme)

## Предназначение

Библиотека решает задачу пересчета биржевого стакана для обмена активов. Классический биржевой стакан состоит из лимитных заявок (ордеров) на покупку или продажу (обмен) какого-то товара на другой товар, как правило валюту.

Отличительная особенность библиотеки в том, что:
- Нет заранее определенных уровней цены
- Нет ограничения на используемые типы числовых значений
- При работе в рамках целых чисел достигается производительность до 500к транзакций в секунду
- Содержит в себе все необходимые зависимости
- Есть поддержка списания комиссий типа maker и taker для работы на криптовалюиных активах



## Описание интерфейсов и типов данных

Все математические операции пересчета биржевого стакана рассчитаны на работу в системе бизнес-логики, поэтому сам matching engine работает с входными интерфейсами и уведомляет бизнес-логику о произошедших событиях в процессе обмена товаров. А та в свою очередь принимает решение, что делать с этими данными (напрмиер списать средства с соответствующего кошелька). Для корректной работы биржевого стакана требуется реализовать интерфейсы, описанные в файле ```echange.go```


### Asset

```Go
type Asset string
```

Описывает имя некоторого обменного актива. Matching engine уведомляет бизнес-логику о произошедшем событии с товаром и типом Asset а не string для наглядности

### Volume

```Go
type Volume struct {
	Price    Value
	Quantity Value
}
```

При совершении сделки или закрытии позиции биржевой стакан также уведомляет о прошедшем объеме сделки типом Volume. В нем содержится значение суммы и количество обменного актива.


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

Для пересчета и хранения чисел в заявках биржевого стакана ему требуется реализация определенных математические операций. Остановиться на каком-то определенном типе данных (например int.Big) очевидно неудобно из-за последующего жесткого ограничения на связанный тип. Поэтому появился тип ```Value```, в который можно завернуть этот интерфейс (пример в файле ```engine_test.go```). 

#### Add, Sub и Mul
Должны возвращать новый объект а не измененный текущий.

#### Cmp и Sign
Должны возвращать знак числа уметь сравнить два числа, как указано в помощи.

#### Hash
Не обязательно должна возвращать хеш числа. Достаточно для конкретного одного и того-же числа возвращать уникальную строку. В большинстве случаев подойдет преобразование числа в строку. Эта функция используется для хранения ценовых уровней и добавления заявок в очередь по определенной цене. Этого интерфейса достаточно для реализации всей математики обмена. 


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

В ходе совершения сделки биржевой стакан пользуется интефейсом ```Wallet``` для того, чтобы корректно зачислить или списать средства в обменных кошельках. В кошельке присутствует обязательный баланс ```Balance``` в котором содержится текущий баланс кошелька, а также ```InOrder``` баланс, обозначающий количество актива в биржевом стакане.

Функции вызываются движком во время просчёта заявки. __Контекст__ передается сквозным от функций выставления и отмены ордера, поэтому в него можно поместить например объекты записи базы данных.

Функции помеченные как optional могут быть заглушками и не влияют на работу движка, однако полезны для учета количества актива в стакане при рассчете биржевой бизнес-логики.

#### Balance и InOrder 
Должны возвратить текущий баланс кошелька для дальнейших операций пересчёта.

#### UpdateBalance и UpdateInOrder 
Должны обновить баланс кошелька при совершении сделки.


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

Основная обменная единица - заявка. Лимитные заявки хранятся в оперативной памяти стакана и удаляются из него по мере завершения указанного количества. Для работы движка необходимо реализовать указанные методы.

#### ID
Должен вернуть уникальный идентификатор заявки. В случае повтора заявка будет отклонена.

#### Owner
Должен возвратить кошелек, которому принадлежит эта заявка. Метод можно реализовать различными путями, например через менеджера кошельков при создании заявки. Или в заявке держать указатель на кошелек и возвращать его.

#### Sell
Возвращает true, если заявка на продажу.

#### Price
Возвращает желаемую лимитную стоимость по которой готов купить или продать актив. Если стоимость равна нулю, то ордер исполнится полностью по текущей рыночной цене (рыночный ордер) если в кошельке достаточно средств для покупки или продажи.

#### Quantity
Должно вернуть оставшееся количество в неисполненной заявке.

#### UpdateQuantity
Вызывается движком при пересчете и совершении сделки, уведомляя бизнес логику о том, какое текущее количество актива осталось.


### FeeHandler

```Go
type FeeHandler interface {
	// HandleFeeMaker calls by  matching engine and provide data to correct output value for fee processing
	HandleFeeMaker(context.Context, Order, Asset, Value) (out Value)

	// HandleFeeTaker calls by  matching engine and provide data to correct output value for fee processing
	HandleFeeTaker(context.Context, Order, Asset, Value) (out Value)
}
```

Вызывается во время совершения сделки с целью списания комиссии. В текущей версии комиссия списывается пост обработкой. Например продано 1 единица товара, а на баланс зачислится 0.9 единиц в связи с особенностью применения движка. Параметр опциональный. Обработчик делится на Maker - маркет мейкер. То есть лимитный ордер. И Taker - рыночный ордер (или рыночная часть лимитного).

#### HandleFeeMaker, HandleFeeTaker
Контекст сковзной, на вход поступает информация по совершаемой сделке по определенному активу. Функция должна вернуть inValue - feeValue в out значение.


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

Интерфейс участвует в операциях выставления и снятия заявки. Служит для уведомления вышестоящей бизнес-логики обо всех произошедших событиях в ходе обработки ордера. Различают 2 типа ордера - входящий и существующий. Существующий - это лимитный ордер (с его остатком), находящийся в биржевом стакане. 

#### OnIncomingOrderPartial
Вызывается при частичном исполнении входящего ордера.

#### OnIncomingOrderDone
Вызывается при завершении входящего ордера.

#### OnIncomingOrderPlaced
Вызывается когда входящий ордер (или остаток от частичного исполнения) встал в очередь биржевого стакана.

#### OnExistingOrderPartial
Вызывается при частичном исполнении лимитного ордера.

#### OnExistingOrderDone
Вызывается при завершении лимитного ордера.

#### OnExistingOrderCanceled
Вызывается при отмене лимитного ордера.

#### OnBalanceChanged
Вызывается при измнении баланса на кошельке в результате обработки заявки. Уведомляет получателя о новом балансе.

#### OnInOrderChanged
Вызывается при измнении баланса в ордерах на кошельке в результате обработки заявки. Уведомляет получателя о новом значении количества актива в стакане.


## Функционал

Для управления биржевым стаканом реализованы основные функции обработки входящих ордеров.

#### func NewEngine(base, quote Asset) *Engine
Инициализирует экземпляр matching engine. Выделяет память под данные.

#### func (e *Engine) SetFeeHandler(h FeeHandler)
Устанавливает обработчик комиссии.

#### func (e *Engine) PlaceOrder(ctx context.Context, listener EventListener, o Order) (err error)
Обрабатывает входящий ордер. Пересчитывает состояние биржевого стакана. Возвращает следующие ошибки в случае невозможности обработать ордер:
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
Удаляет указанный ордер из биржевого стакана.

#### func (e *Engine) PushOrder(ctx context.Context, o Order)
Ставит ордер в очередь без применения математического пересчета. Используется при восстановлении стакана из базы данных.

#### func (e *Engine) Quantity(sell bool, priceLim Value) Value
Возвращает объем актива до определенной цены.

#### func (e *Engine) Price(sell bool, quantity Value) (Value, error)
Возвращает рыночную стоимость для указанного количества актива.

#### func (e *Engine) Spread() (bestAsk, bestBid Value)
Возвращает значение ценового спреда в стакане. nil если по одному или двум направлениям заявок нет.

#### func (e *Engine) FindOrder(id string) (Order, error)
Возвращает ордер по его идентификатору или ошибку ```ErrOrderNotFound```

#### func (e *Engine) Orders() (orders []Order)
Возвращает список лимитных ордеров, находящихся в стакане.

#### func (e *Engine) OrderBook(iter func(asks bool, price, volume Value, len int))
Итерирует ценовые уровни, возвращая информацию о цене, объеме заявок и длинне очереди.


## Алгоритм работы

В структуре движка присутствует связанный список ордеров и дерево поиска оптимальной цены для вставки или размещения. 