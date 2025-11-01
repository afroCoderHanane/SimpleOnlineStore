package cart

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

type mockRepository struct {
	createCartFn     func(ctx context.Context, customerID int64) (int64, error)
	upsertItemFn     func(ctx context.Context, cartID, productID int64, quantity int32) error
	removeItemFn     func(ctx context.Context, cartID, productID int64) error
	getCartWithItems func(ctx context.Context, cartID int64) (*Cart, error)
	checkoutCartFn   func(ctx context.Context, cartID int64, total float64) (int64, error)
}

func (m *mockRepository) CreateCart(ctx context.Context, customerID int64) (int64, error) {
	if m.createCartFn == nil {
		return 0, nil
	}
	return m.createCartFn(ctx, customerID)
}

func (m *mockRepository) UpsertCartItem(ctx context.Context, cartID, productID int64, quantity int32) error {
	if m.upsertItemFn == nil {
		return nil
	}
	return m.upsertItemFn(ctx, cartID, productID, quantity)
}

func (m *mockRepository) RemoveCartItem(ctx context.Context, cartID, productID int64) error {
	if m.removeItemFn == nil {
		return nil
	}
	return m.removeItemFn(ctx, cartID, productID)
}

func (m *mockRepository) GetCartWithItems(ctx context.Context, cartID int64) (*Cart, error) {
	if m.getCartWithItems == nil {
		return nil, nil
	}
	return m.getCartWithItems(ctx, cartID)
}

func (m *mockRepository) CheckoutCart(ctx context.Context, cartID int64, total float64) (int64, error) {
	if m.checkoutCartFn == nil {
		return 0, nil
	}
	return m.checkoutCartFn(ctx, cartID, total)
}

func TestHandleCreateCart_Success(t *testing.T) {
	repo := &mockRepository{
		createCartFn: func(ctx context.Context, customerID int64) (int64, error) {
			require.Equal(t, int64(99), customerID)
			return 42, nil
		},
	}

	handler := NewHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts", bytes.NewBufferString(`{"customer_id":99}`))
	rec := httptest.NewRecorder()

	handler.HandleCreateCart(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp createCartResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, int64(42), resp.ShoppingCartID)
}

func TestHandleCreateCart_InvalidBody(t *testing.T) {
	handler := NewHandler(&mockRepository{
		createCartFn: func(ctx context.Context, customerID int64) (int64, error) {
			return 0, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts", bytes.NewBufferString(`{"customer":1}`))
	rec := httptest.NewRecorder()

	handler.HandleCreateCart(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleCreateCart_RepositoryError(t *testing.T) {
	handler := NewHandler(&mockRepository{
		createCartFn: func(ctx context.Context, customerID int64) (int64, error) {
			return 0, assertErr
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts", bytes.NewBufferString(`{"customer_id":1}`))
	rec := httptest.NewRecorder()

	handler.HandleCreateCart(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

var assertErr = &customError{msg: "boom"}

type customError struct{ msg string }

func (e *customError) Error() string { return e.msg }

func TestHandleAddCartItem_Success(t *testing.T) {
	called := false
	handler := NewHandler(&mockRepository{
		upsertItemFn: func(ctx context.Context, cartID, productID int64, quantity int32) error {
			called = true
			require.Equal(t, int64(12), cartID)
			require.Equal(t, int64(55), productID)
			require.Equal(t, int32(3), quantity)
			return nil
		},
		removeItemFn: func(ctx context.Context, cartID, productID int64) error {
			t.Fatalf("remove should not be called")
			return nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts/12/items", bytes.NewBufferString(`{"product_id":55,"quantity":3}`))
	req = mux.SetURLVars(req, map[string]string{"shoppingCartId": "12"})
	rec := httptest.NewRecorder()

	handler.HandleAddCartItem(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.True(t, called)
}

func TestHandleAddCartItem_Remove(t *testing.T) {
	handler := NewHandler(&mockRepository{
		removeItemFn: func(ctx context.Context, cartID, productID int64) error {
			require.Equal(t, int64(7), cartID)
			require.Equal(t, int64(11), productID)
			return nil
		},
		upsertItemFn: func(ctx context.Context, cartID, productID int64, quantity int32) error {
			t.Fatalf("upsert should not be called")
			return nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts/7/items", bytes.NewBufferString(`{"product_id":11,"quantity":0}`))
	req = mux.SetURLVars(req, map[string]string{"shoppingCartId": "7"})
	rec := httptest.NewRecorder()

	handler.HandleAddCartItem(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandleAddCartItem_ValidationError(t *testing.T) {
	handler := NewHandler(&mockRepository{})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts/abc/items", bytes.NewBufferString(`{"product_id":11,"quantity":1}`))
	rec := httptest.NewRecorder()

	handler.HandleAddCartItem(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleAddCartItem_NotFound(t *testing.T) {
	handler := NewHandler(&mockRepository{
		upsertItemFn: func(ctx context.Context, cartID, productID int64, quantity int32) error {
			return ErrCartNotFound
		},
		removeItemFn: func(ctx context.Context, cartID, productID int64) error {
			return nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts/3/items", bytes.NewBufferString(`{"product_id":11,"quantity":2}`))
	req = mux.SetURLVars(req, map[string]string{"shoppingCartId": "3"})
	rec := httptest.NewRecorder()

	handler.HandleAddCartItem(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleAddCartItem_ProductNotFound(t *testing.T) {
	handler := NewHandler(&mockRepository{
		upsertItemFn: func(ctx context.Context, cartID, productID int64, quantity int32) error {
			return ErrProductNotFound
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts/3/items", bytes.NewBufferString(`{"product_id":11,"quantity":2}`))
	req = mux.SetURLVars(req, map[string]string{"shoppingCartId": "3"})
	rec := httptest.NewRecorder()

	handler.HandleAddCartItem(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleAddCartItem_RemoveNotFound(t *testing.T) {
	handler := NewHandler(&mockRepository{
		removeItemFn: func(ctx context.Context, cartID, productID int64) error {
			return ErrCartItemNotFound
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts/3/items", bytes.NewBufferString(`{"product_id":11,"quantity":0}`))
	req = mux.SetURLVars(req, map[string]string{"shoppingCartId": "3"})
	rec := httptest.NewRecorder()

	handler.HandleAddCartItem(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleCheckout_Success(t *testing.T) {
	handler := NewHandler(&mockRepository{
		getCartWithItems: func(ctx context.Context, cartID int64) (*Cart, error) {
                        return &Cart{
                                ID:     cartID,
                                Status: "open",
                                Items: []Item{{
                                        ProductID: 1,
                                        Quantity:  2,
                                        Product: &Product{
                                                ID:    1,
                                                Price: 5.5,
                                        },
                                }},
                        }, nil
                },
		checkoutCartFn: func(ctx context.Context, cartID int64, total float64) (int64, error) {
			require.Equal(t, float64(11), total)
			return 1001, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts/5/checkout", nil)
	req = mux.SetURLVars(req, map[string]string{"shoppingCartId": "5"})
	rec := httptest.NewRecorder()

	handler.HandleCheckout(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp orderResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, int64(1001), resp.OrderID)
}

func TestHandleCheckout_CartNotFound(t *testing.T) {
	handler := NewHandler(&mockRepository{
		getCartWithItems: func(ctx context.Context, cartID int64) (*Cart, error) {
			return nil, ErrCartNotFound
		},
		checkoutCartFn: func(ctx context.Context, cartID int64, total float64) (int64, error) {
			return 0, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts/5/checkout", nil)
	req = mux.SetURLVars(req, map[string]string{"shoppingCartId": "5"})
	rec := httptest.NewRecorder()

	handler.HandleCheckout(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleCheckout_EmptyCart(t *testing.T) {
	handler := NewHandler(&mockRepository{
		getCartWithItems: func(ctx context.Context, cartID int64) (*Cart, error) {
			return &Cart{ID: cartID, Status: "open", Items: []Item{}}, nil
		},
		checkoutCartFn: func(ctx context.Context, cartID int64, total float64) (int64, error) {
			t.Fatalf("checkout should not be called")
			return 0, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts/5/checkout", nil)
	req = mux.SetURLVars(req, map[string]string{"shoppingCartId": "5"})
	rec := httptest.NewRecorder()

	handler.HandleCheckout(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleCheckout_AlreadyClosed(t *testing.T) {
	handler := NewHandler(&mockRepository{
		getCartWithItems: func(ctx context.Context, cartID int64) (*Cart, error) {
                        return &Cart{ID: cartID, Status: "checked_out", Items: []Item{{
                                ProductID: 1,
                                Quantity:  1,
                                Product: &Product{
                                        ID:    1,
                                        Price: 10,
                                },
                        }}}, nil
		},
		checkoutCartFn: func(ctx context.Context, cartID int64, total float64) (int64, error) {
			t.Fatalf("checkout should not be called")
			return 0, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts/5/checkout", nil)
	req = mux.SetURLVars(req, map[string]string{"shoppingCartId": "5"})
	rec := httptest.NewRecorder()

	handler.HandleCheckout(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleCheckout_RepositoryError(t *testing.T) {
	handler := NewHandler(&mockRepository{
		getCartWithItems: func(ctx context.Context, cartID int64) (*Cart, error) {
                        return &Cart{ID: cartID, Status: "open", Items: []Item{{
                                ProductID: 1,
                                Quantity:  1,
                                Product: &Product{
                                        ID:    1,
                                        Price: 10,
                                },
                        }}}, nil
		},
		checkoutCartFn: func(ctx context.Context, cartID int64, total float64) (int64, error) {
			return 0, assertErr
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts/5/checkout", nil)
	req = mux.SetURLVars(req, map[string]string{"shoppingCartId": "5"})
	rec := httptest.NewRecorder()

	handler.HandleCheckout(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleCheckout_RepositoryConflict(t *testing.T) {
	handler := NewHandler(&mockRepository{
		getCartWithItems: func(ctx context.Context, cartID int64) (*Cart, error) {
                        return &Cart{ID: cartID, Status: "open", Items: []Item{{
                                ProductID: 1,
                                Quantity:  1,
                                Product: &Product{
                                        ID:    1,
                                        Price: 10,
                                },
                        }}}, nil
		},
		checkoutCartFn: func(ctx context.Context, cartID int64, total float64) (int64, error) {
			return 0, ErrCartAlreadyClosed
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/shopping-carts/5/checkout", nil)
	rec := httptest.NewRecorder()

	handler.HandleCheckout(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
