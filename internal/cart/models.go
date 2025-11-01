package cart

import "errors"

// Domain errors returned by the repository and handlers.
var (
	ErrCartNotFound      = errors.New("shopping cart not found")
	ErrProductNotFound   = errors.New("product not found")
	ErrCartItemNotFound  = errors.New("shopping cart item not found")
	ErrCartEmpty         = errors.New("shopping cart is empty")
	ErrCartAlreadyClosed = errors.New("shopping cart is not open")
)

// Cart represents a shopping cart with its items.
type Cart struct {
	ID         int64
	CustomerID int64
	Status     string
	Items      []Item
}

// Item represents a product stored in a shopping cart.
type Item struct {
	ProductID   int64
	Quantity    int32
	ProductName string
	Price       float64
}

// Total computes the aggregated price of all items.
func (c Cart) Total() float64 {
	var total float64
	for _, item := range c.Items {
		total += float64(item.Quantity) * item.Price
	}
	return total
}
