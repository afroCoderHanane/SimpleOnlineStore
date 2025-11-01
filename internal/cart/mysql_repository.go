package cart

import (
	"context"
	"database/sql"
)

// MySQLRepository implements Repository backed by MySQL.
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository creates a repository using the provided sql.DB instance.
func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// CreateCart inserts a new shopping cart for the given customer.
func (r *MySQLRepository) CreateCart(ctx context.Context, customerID int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, "INSERT INTO shopping_carts (customer_id, status) VALUES (?, 'open')", customerID)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

// UpsertCartItem creates or updates an item entry within the shopping cart.
func (r *MySQLRepository) UpsertCartItem(ctx context.Context, cartID, productID int64, quantity int32) error {
	if err := r.ensureCartExists(ctx, cartID); err != nil {
		return err
	}
	if err := r.ensureProductExists(ctx, productID); err != nil {
		return err
	}

	_, err := r.db.ExecContext(ctx, `
        INSERT INTO shopping_cart_items (shopping_cart_id, product_id, quantity)
        VALUES (?, ?, ?)
        ON DUPLICATE KEY UPDATE quantity = VALUES(quantity)
    `, cartID, productID, quantity)
	return err
}

// RemoveCartItem deletes an item from the shopping cart.
func (r *MySQLRepository) RemoveCartItem(ctx context.Context, cartID, productID int64) error {
	if err := r.ensureCartExists(ctx, cartID); err != nil {
		return err
	}

	res, err := r.db.ExecContext(ctx, "DELETE FROM shopping_cart_items WHERE shopping_cart_id = ? AND product_id = ?", cartID, productID)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrCartItemNotFound
	}

	return nil
}

// GetCartWithItems fetches the shopping cart with its item details.
func (r *MySQLRepository) GetCartWithItems(ctx context.Context, cartID int64) (*Cart, error) {
	rows, err := r.db.QueryContext(ctx, `
        SELECT c.id, c.customer_id, c.status,
               i.product_id, i.quantity,
               p.name, p.price
        FROM shopping_carts c
        LEFT JOIN shopping_cart_items i ON c.id = i.shopping_cart_id
        LEFT JOIN products p ON i.product_id = p.id
        WHERE c.id = ?
    `, cartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cart *Cart
	for rows.Next() {
		var (
			id         int64
			customerID int64
			status     string
			productID  sql.NullInt64
			quantity   sql.NullInt64
			name       sql.NullString
			price      sql.NullFloat64
		)

		if err := rows.Scan(&id, &customerID, &status, &productID, &quantity, &name, &price); err != nil {
			return nil, err
		}

		if cart == nil {
			cart = &Cart{
				ID:         id,
				CustomerID: customerID,
				Status:     status,
				Items:      make([]Item, 0),
			}
		}

		if productID.Valid {
			cart.Items = append(cart.Items, Item{
				ProductID:   productID.Int64,
				Quantity:    int32(quantity.Int64),
				ProductName: name.String,
				Price:       price.Float64,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if cart == nil {
		return nil, ErrCartNotFound
	}

	return cart, nil
}

// CheckoutCart converts a shopping cart into an order and marks the cart as checked out.
func (r *MySQLRepository) CheckoutCart(ctx context.Context, cartID int64, total float64) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var status string
	if err = tx.QueryRowContext(ctx, "SELECT status FROM shopping_carts WHERE id = ? FOR UPDATE", cartID).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			err = ErrCartNotFound
		}
		return 0, err
	}

	if status != "open" {
		err = ErrCartAlreadyClosed
		return 0, err
	}

	result, execErr := tx.ExecContext(ctx, "INSERT INTO orders (shopping_cart_id, total_amount) VALUES (?, ?)", cartID, total)
	if execErr != nil {
		err = execErr
		return 0, err
	}

	orderID, execErr := result.LastInsertId()
	if execErr != nil {
		err = execErr
		return 0, err
	}

	if _, execErr = tx.ExecContext(ctx, "UPDATE shopping_carts SET status = 'checked_out' WHERE id = ?", cartID); execErr != nil {
		err = execErr
		return 0, err
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return 0, commitErr
	}

	return orderID, nil
}

func (r *MySQLRepository) ensureCartExists(ctx context.Context, cartID int64) error {
	var exists bool
	if err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM shopping_carts WHERE id = ?)", cartID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrCartNotFound
	}
	return nil
}

func (r *MySQLRepository) ensureProductExists(ctx context.Context, productID int64) error {
	var exists bool
	if err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM products WHERE id = ?)", productID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrProductNotFound
	}
	return nil
}
