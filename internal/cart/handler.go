package cart

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

// Handler exposes HTTP handlers for shopping cart operations.
type Handler struct {
	repo Repository
}

// NewHandler constructs a cart handler backed by the provided repository.
func NewHandler(repo Repository) *Handler {
	return &Handler{repo: repo}
}

// RegisterRoutes wires the shopping cart handlers to the given router.
func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/shopping-carts", h.HandleCreateCart).Methods(http.MethodPost)
	r.HandleFunc("/shopping-carts/{shoppingCartId:[0-9]+}/items", h.HandleAddCartItem).Methods(http.MethodPost)
	r.HandleFunc("/shopping-carts/{shoppingCartId:[0-9]+}/checkout", h.HandleCheckout).Methods(http.MethodPost)
}

type createCartRequest struct {
	CustomerID int64 `json:"customer_id"`
}

type createCartResponse struct {
	ShoppingCartID int64 `json:"shopping_cart_id"`
}

type addCartItemRequest struct {
	ProductID int64 `json:"product_id"`
	Quantity  int32 `json:"quantity"`
}

type orderResponse struct {
	OrderID int64 `json:"order_id"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

var errInvalidCartID = errors.New("invalid shoppingCartId path parameter")

// HandleCreateCart handles POST /shopping-carts requests.
func (h *Handler) HandleCreateCart(w http.ResponseWriter, r *http.Request) {
	var req createCartRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.CustomerID < 1 {
		writeError(w, http.StatusBadRequest, "customer_id must be a positive integer")
		return
	}

	cartID, err := h.repo.CreateCart(r.Context(), req.CustomerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, createCartResponse{ShoppingCartID: cartID})
}

// HandleAddCartItem handles POST /shopping-carts/{id}/items.
func (h *Handler) HandleAddCartItem(w http.ResponseWriter, r *http.Request) {
	cartID, err := parseCartID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req addCartItemRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.ProductID < 1 {
		writeError(w, http.StatusBadRequest, "product_id must be a positive integer")
		return
	}

	if req.Quantity < 0 {
		writeError(w, http.StatusBadRequest, "quantity must be greater than or equal to zero")
		return
	}

	if req.Quantity == 0 {
		err = h.repo.RemoveCartItem(r.Context(), cartID, req.ProductID)
		if err != nil {
			switch err {
			case ErrCartNotFound, ErrCartItemNotFound:
				writeError(w, http.StatusNotFound, err.Error())
			default:
				writeError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	err = h.repo.UpsertCartItem(r.Context(), cartID, req.ProductID, req.Quantity)
	if err != nil {
		switch err {
		case ErrCartNotFound, ErrProductNotFound:
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleCheckout handles POST /shopping-carts/{id}/checkout.
func (h *Handler) HandleCheckout(w http.ResponseWriter, r *http.Request) {
	cartID, err := parseCartID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	cart, err := h.repo.GetCartWithItems(r.Context(), cartID)
	if err != nil {
		switch err {
		case ErrCartNotFound:
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	if cart.Status != "open" {
		writeError(w, http.StatusBadRequest, ErrCartAlreadyClosed.Error())
		return
	}

	if len(cart.Items) == 0 {
		writeError(w, http.StatusBadRequest, ErrCartEmpty.Error())
		return
	}

	orderID, err := h.repo.CheckoutCart(r.Context(), cart.ID, cart.Total())
	if err != nil {
		switch err {
		case ErrCartNotFound:
			writeError(w, http.StatusNotFound, err.Error())
		case ErrCartAlreadyClosed, ErrCartEmpty:
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, orderResponse{OrderID: orderID})
}

func parseCartID(r *http.Request) (int64, error) {
	vars := mux.Vars(r)
	value, ok := vars["shoppingCartId"]
	if !ok {
		return 0, errInvalidCartID
	}

	cartID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || cartID < 1 {
		return 0, errInvalidCartID
	}

	return cartID, nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, apiError{Code: status, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
