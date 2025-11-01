package cart

import "context"

// Repository defines the storage operations required by the shopping cart handlers.
type Repository interface {
	CreateCart(ctx context.Context, customerID int64) (int64, error)
	UpsertCartItem(ctx context.Context, cartID, productID int64, quantity int32) error
	RemoveCartItem(ctx context.Context, cartID, productID int64) error
	GetCartWithItems(ctx context.Context, cartID int64) (*Cart, error)
	CheckoutCart(ctx context.Context, cartID int64, total float64) (int64, error)
}
