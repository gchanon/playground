package orders

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Line struct {
	SKU       string `json:"sku"`
	Quantity  int    `json:"quantity"`
	UnitMinor int64  `json:"unit_minor"`
}

type Order struct {
	ID        string            `json:"id"`
	Customer  string            `json:"customer"`
	Currency  string            `json:"currency"`
	Lines     []Line            `json:"lines"`
	Labels    map[string]string `json:"labels"`
	reserved  map[string]int
	createdAt time.Time
}

func NewOrder(id, customer, currency string) *Order {
	return &Order{
		ID:        id,
		Customer:  customer,
		Currency:  currency,
		Labels:    make(map[string]string),
		reserved:  make(map[string]int),
		createdAt: time.Now(),
	}
}

func LoadDraft(raw []byte) (*Order, error) {
	var o Order
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, fmt.Errorf("decode draft: %w", err)
	}
	if o.ID == "" {
		return nil, errors.New("draft is missing an order id")
	}
	if o.Currency == "" {
		o.Currency = "THB"
	}
	if o.Labels == nil {
		o.Labels = make(map[string]string)
	}
	if o.reserved == nil {
		o.reserved = make(map[string]int)
	}
	o.createdAt = time.Now()
	return &o, nil
}

func (o *Order) AddLine(l Line) error {
	if l.Quantity <= 0 {
		return fmt.Errorf("add line %s: quantity must be positive", l.SKU)
	}
	o.Lines = append(o.Lines, l)
	return nil
}

func (o *Order) available(sku string) int {
	total := 0
	for _, l := range o.Lines {
		if l.SKU == sku {
			total += l.Quantity
		}
	}
	return total - o.reserved[sku]
}

func (o *Order) Reserve(sku string, qty int) error {
	if qty <= 0 {
		return fmt.Errorf("reserve %s: quantity must be positive", sku)
	}
	if free := o.available(sku); free < qty {
		return fmt.Errorf("reserve %s: want %d, only %d free", sku, qty, free)
	}
	o.reserved[sku] += qty
	return nil
}

func (o *Order) Release(sku string, qty int) error {
	held := o.reserved[sku]
	if qty > held {
		return fmt.Errorf("release %s: holding %d, asked for %d", sku, held, qty)
	}
	o.reserved[sku] = held - qty
	return nil
}

func (o *Order) ReservedCount(sku string) int {
	return o.reserved[sku]
}

func (o *Order) TotalMinor() int64 {
	var total int64
	for _, l := range o.Lines {
		total += l.UnitMinor * int64(l.Quantity)
	}
	return total
}

type Repository struct {
	mu     sync.RWMutex
	orders map[string]*Order
}

func NewRepository() *Repository {
	return &Repository{orders: make(map[string]*Order)}
}

func (r *Repository) Save(o *Order) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.orders[o.ID] = o
}

func (r *Repository) Get(id string) (*Order, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	o, ok := r.orders[id]
	return o, ok
}
