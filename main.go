package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/mux"
)

// Product represents the product model based on OpenAPI schema
type Product struct {
	ID          int32   `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Stock       int32   `json:"stock"`
	Category    string  `json:"category,omitempty"`
	ImageURL    string  `json:"imageUrl,omitempty"`
}

// Error represents the error response model
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ProductStore handles in-memory storage with thread safety
type ProductStore struct {
	mu       sync.RWMutex
	products map[int32]*Product
	nextID   int32
}

// NewProductStore creates a new product store
func NewProductStore() *ProductStore {
	return &ProductStore{
		products: make(map[int32]*Product),
		nextID:   1,
	}
}

// GetProduct retrieves a product by ID (thread-safe read)
func (s *ProductStore) GetProduct(id int32) (*Product, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	product, exists := s.products[id]
	return product, exists
}

// AddOrUpdateProduct adds or updates product details (thread-safe write)
func (s *ProductStore) AddOrUpdateProduct(id int32, product *Product) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Check if product exists
	if _, exists := s.products[id]; !exists {
		return false
	}
	
	// Update the product, preserving the ID
	product.ID = id
	s.products[id] = product
	return true
}

// CreateProduct creates a new product (for initial data seeding)
func (s *ProductStore) CreateProduct(product *Product) *Product {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	product.ID = s.nextID
	s.products[s.nextID] = product
	s.nextID++
	return product
}

// Server represents the HTTP server
type Server struct {
	store *ProductStore
}

// NewServer creates a new server instance
func NewServer() *Server {
	server := &Server{
		store: NewProductStore(),
	}
	// Seed some initial products for testing
	server.seedData()
	return server
}

// seedData adds initial products for testing
func (s *Server) seedData() {
	products := []*Product{
		{Name: "Laptop", Description: "High-performance laptop", Price: 999.99, Stock: 10, Category: "Electronics"},
		{Name: "Mouse", Description: "Wireless mouse", Price: 29.99, Stock: 50, Category: "Electronics"},
		{Name: "Keyboard", Description: "Mechanical keyboard", Price: 79.99, Stock: 30, Category: "Electronics"},
	}
	
	for _, p := range products {
		s.store.CreateProduct(p)
	}
}

// HandleGetProduct handles GET /products/{productId}
func (s *Server) HandleGetProduct(w http.ResponseWriter, r *http.Request) {
	// Extract productId from path
	vars := mux.Vars(r)
	productIDStr := vars["productId"]
	
	// Parse and validate productId
	productID64, err := strconv.ParseInt(productIDStr, 10, 32)
	if err != nil || productID64 < 1 {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid product ID format")
		return
	}
	productID := int32(productID64)
	
	// Retrieve product from store
	product, exists := s.store.GetProduct(productID)
	if !exists {
		writeErrorResponse(w, http.StatusNotFound, fmt.Sprintf("Product with ID %d not found", productID))
		return
	}
	
	// Return successful response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(product); err != nil {
		log.Printf("Error encoding product response: %v", err)
	}
}

// HandleAddProductDetails handles POST /products/{productId}/details
func (s *Server) HandleAddProductDetails(w http.ResponseWriter, r *http.Request) {
	// Extract productId from path
	vars := mux.Vars(r)
	productIDStr := vars["productId"]
	
	// Parse and validate productId
	productID64, err := strconv.ParseInt(productIDStr, 10, 32)
	if err != nil || productID64 < 1 {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid product ID format")
		return
	}
	productID := int32(productID64)
	
	// Parse request body
	var product Product
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // Strict parsing
	if err := decoder.Decode(&product); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}
	
	// Validate required fields
	if product.Name == "" || product.Price < 0 || product.Stock < 0 {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid product data: name is required, price and stock must be non-negative")
		return
	}
	
	// Update product in store
	if !s.store.AddOrUpdateProduct(productID, &product) {
		writeErrorResponse(w, http.StatusNotFound, fmt.Sprintf("Product with ID %d not found", productID))
		return
	}
	
	// Return 204 No Content on success
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
	// Create server
	server := NewServer()
	
	// Setup routes
	router := mux.NewRouter()
	
	// Apply middleware
	router.Use(LoggingMiddleware)
	router.Use(RecoveryMiddleware)
	
	// Product endpoints
	router.HandleFunc("/products/{productId:[0-9]+}", server.HandleGetProduct).Methods("GET")
	router.HandleFunc("/products/{productId:[0-9]+}/details", server.HandleAddProductDetails).Methods("POST")
	
	// Health check endpoint (useful for ECS)
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")
	
	// Start server
	port := "8080"
	log.Printf("Starting server on port %s", port)
	log.Printf("Initial products seeded: 3 products available (IDs: 1, 2, 3)")
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}