package cart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound indicates that the requested record does not exist.
var ErrNotFound = errors.New("not found")

// Repository exposes data access helpers for products, carts, and cart items.
type Repository struct {
	db *sql.DB
}

// Product represents the product record stored in MySQL.
type Product struct {
	ID          int32   `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Stock       int32   `json:"stock"`
	Category    string  `json:"category,omitempty"`
	ImageURL    string  `json:"imageUrl,omitempty"`
}

// Cart represents a cart with its line items.
type Cart struct {
	ID        int64      `json:"id"`
	UserID    string     `json:"userId"`
	Status    string     `json:"status"`
	Items     []CartItem `json:"items"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// CartItem represents a product stored in a cart.
type CartItem struct {
	ID        int64     `json:"id"`
	CartID    int64     `json:"cartId"`
	ProductID int32     `json:"productId"`
	Quantity  int32     `json:"quantity"`
	Price     float64   `json:"price"`
	Product   *Product  `json:"product,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NewRepository constructs a new Repository backed by the provided database handle.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// GetProduct fetches a product by ID.
func (r *Repository) GetProduct(ctx context.Context, id int32) (*Product, error) {
	const query = `
SELECT id, name, description, price, stock, category, image_url
FROM products
WHERE id = ?`
	var p Product
	if err := r.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock, &p.Category, &p.ImageURL,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("select product: %w", err)
	}
	return &p, nil
}

// UpdateProduct updates the details for a product.
func (r *Repository) UpdateProduct(ctx context.Context, id int32, p *Product) error {
	const query = `
UPDATE products
SET name = ?, description = ?, price = ?, stock = ?, category = ?, image_url = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`
	res, err := r.db.ExecContext(ctx, query, p.Name, p.Description, p.Price, p.Stock, p.Category, p.ImageURL, id)
	if err != nil {
		return fmt.Errorf("update product: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateCart creates a new cart for a user.
func (r *Repository) CreateCart(ctx context.Context, userID string) (*Cart, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	res, err := tx.ExecContext(ctx, `INSERT INTO carts (user_id, status) VALUES (?, 'open')`, userID)
	if err != nil {
		return nil, fmt.Errorf("insert cart: %w", err)
	}
	cartID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("cart id: %w", err)
	}

	cart, err := getCartTx(ctx, tx, cartID)
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit cart: %w", err)
	}
	return cart, nil
}

// GetCart fetches a cart and all of its items by ID.
func (r *Repository) GetCart(ctx context.Context, cartID int64) (*Cart, error) {
	cart, err := r.loadCart(ctx, cartID)
	if err != nil {
		return nil, err
	}
	return cart, nil
}

// AddOrUpdateCartItem inserts or updates a line item, using a transaction to guarantee atomicity.
func (r *Repository) AddOrUpdateCartItem(ctx context.Context, cartID int64, productID int32, quantity int32) error {
	if quantity < 1 {
		return fmt.Errorf("quantity must be positive")
	}

	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := ensureCartExists(ctx, tx, cartID); err != nil {
			return err
		}

		var price float64
		if err := tx.QueryRowContext(ctx, `SELECT price FROM products WHERE id = ? FOR UPDATE`, productID).Scan(&price); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("product %d: %w", productID, ErrNotFound)
			}
			return fmt.Errorf("lookup product price: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO cart_items (cart_id, product_id, quantity, price)
VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE quantity = VALUES(quantity), price = VALUES(price), updated_at = CURRENT_TIMESTAMP`, cartID, productID, quantity, price); err != nil {
			return fmt.Errorf("upsert cart item: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `UPDATE carts SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, cartID); err != nil {
			return fmt.Errorf("touch cart: %w", err)
		}
		return nil
	})
}

// RemoveCartItem removes a line item from a cart.
func (r *Repository) RemoveCartItem(ctx context.Context, cartID int64, productID int32) error {
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := ensureCartExists(ctx, tx, cartID); err != nil {
			return err
		}

		res, err := tx.ExecContext(ctx, `DELETE FROM cart_items WHERE cart_id = ? AND product_id = ?`, cartID, productID)
		if err != nil {
			return fmt.Errorf("delete cart item: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("delete rows affected: %w", err)
		}
		if affected == 0 {
			return ErrNotFound
		}

		if _, err := tx.ExecContext(ctx, `UPDATE carts SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, cartID); err != nil {
			return fmt.Errorf("touch cart: %w", err)
		}
		return nil
	})
}

// ClearCart removes all items from a cart atomically.
func (r *Repository) ClearCart(ctx context.Context, cartID int64) error {
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := ensureCartExists(ctx, tx, cartID); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM cart_items WHERE cart_id = ?`, cartID); err != nil {
			return fmt.Errorf("clear cart: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `UPDATE carts SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, cartID); err != nil {
			return fmt.Errorf("touch cart: %w", err)
		}
		return nil
	})
}

func (r *Repository) loadCart(ctx context.Context, cartID int64) (*Cart, error) {
	return getCart(ctx, r.db, cartID)
}

func getCart(ctx context.Context, db queryer, cartID int64) (*Cart, error) {
	cart := Cart{}
	const cartQuery = `
SELECT id, user_id, status, created_at, updated_at
FROM carts
WHERE id = ?`
	if err := db.QueryRowContext(ctx, cartQuery, cartID).Scan(&cart.ID, &cart.UserID, &cart.Status, &cart.CreatedAt, &cart.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("select cart: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
SELECT ci.id, ci.cart_id, ci.product_id, ci.quantity, ci.price, ci.created_at, ci.updated_at,
       p.id, p.name, p.description, p.price, p.stock, p.category, p.image_url
FROM cart_items ci
JOIN products p ON p.id = ci.product_id
WHERE ci.cart_id = ?
ORDER BY ci.id`, cartID)
	if err != nil {
		return nil, fmt.Errorf("select cart items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item CartItem
		var prod Product
		if err := rows.Scan(
			&item.ID, &item.CartID, &item.ProductID, &item.Quantity, &item.Price, &item.CreatedAt, &item.UpdatedAt,
			&prod.ID, &prod.Name, &prod.Description, &prod.Price, &prod.Stock, &prod.Category, &prod.ImageURL,
		); err != nil {
			return nil, fmt.Errorf("scan cart item: %w", err)
		}
		item.Product = &prod
		cart.Items = append(cart.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cart items: %w", err)
	}
	return &cart, nil
}

func getCartTx(ctx context.Context, tx *sql.Tx, cartID int64) (*Cart, error) {
	return getCart(ctx, tx, cartID)
}

func ensureCartExists(ctx context.Context, tx *sql.Tx, cartID int64) error {
	if err := tx.QueryRowContext(ctx, `SELECT id FROM carts WHERE id = ? FOR UPDATE`, cartID).Scan(new(int64)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cart %d: %w", cartID, ErrNotFound)
		}
		return fmt.Errorf("ensure cart: %w", err)
	}
	return nil
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func withTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
