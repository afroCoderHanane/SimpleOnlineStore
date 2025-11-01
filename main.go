package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	"store/internal/cart"
	"store/internal/db"
)

// Error represents the error response model
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Server represents the HTTP server
type Server struct {
	repo *cart.Repository
}

// NewServer creates a new server instance
func NewServer(repo *cart.Repository) *Server {
	return &Server{repo: repo}
}

// HandleGetProduct handles GET /products/{productId}
func (s *Server) HandleGetProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	productIDStr := vars["productId"]

	productID64, err := strconv.ParseInt(productIDStr, 10, 32)
	if err != nil || productID64 < 1 {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid product ID format")
		return
	}
	productID := int32(productID64)

	product, err := s.repo.GetProduct(r.Context(), productID)
	if err != nil {
		if errors.Is(err, cart.ErrNotFound) {
			writeErrorResponse(w, http.StatusNotFound, fmt.Sprintf("Product with ID %d not found", productID))
			return
		}
		log.Printf("Error retrieving product %d: %v", productID, err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(product); err != nil {
		log.Printf("Error encoding product response: %v", err)
	}
}

// HandleAddProductDetails handles POST /products/{productId}/details
func (s *Server) HandleAddProductDetails(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	productIDStr := vars["productId"]

	productID64, err := strconv.ParseInt(productIDStr, 10, 32)
	if err != nil || productID64 < 1 {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid product ID format")
		return
	}
	productID := int32(productID64)

	var product cart.Product
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&product); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	if product.Name == "" || product.Price < 0 || product.Stock < 0 {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid product data: name is required, price and stock must be non-negative")
		return
	}

	if err := s.repo.UpdateProduct(r.Context(), productID, &product); err != nil {
		if errors.Is(err, cart.ErrNotFound) {
			writeErrorResponse(w, http.StatusNotFound, fmt.Sprintf("Product with ID %d not found", productID))
			return
		}
		log.Printf("Error updating product %d: %v", productID, err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeErrorResponse writes an error response
func writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResponse := Error{
		Code:    statusCode,
		Message: message,
	}

	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		log.Printf("Error encoding error response: %v", err)
	}
}

// LoggingMiddleware logs all incoming requests
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] %s %s", r.Method, r.RequestURI, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

// RecoveryMiddleware handles panics gracefully
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic recovered: %v", err)
				writeErrorResponse(w, http.StatusInternalServerError, "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func main() {
	ctx := context.Background()

	mysqlDB, err := db.NewMySQLDB(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize MySQL connection: %v", err)
	}
	defer mysqlDB.Close()

	repository := cart.NewRepository(mysqlDB)
	server := NewServer(repository)

	router := mux.NewRouter()
	router.Use(LoggingMiddleware)
	router.Use(RecoveryMiddleware)

	router.HandleFunc("/products/{productId:[0-9]+}", server.HandleGetProduct).Methods("GET")
	router.HandleFunc("/products/{productId:[0-9]+}/details", server.HandleAddProductDetails).Methods("POST")

	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}).Methods("GET")

	port := "8080"
	log.Printf("Starting server on port %s", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
