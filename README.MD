# [Product API Service]
[DOC](https://docs.google.com/document/d/1gECje-Mzk39h4Me723CLzGsljHohsDY_Bd7xNSNbTFM/edit?usp=sharing)

A simple RESTful API service for managing product information, built with Go and deployable to AWS ECS using Terraform.

## Features

- GET product by ID
- POST/UPDATE product details
- In-memory storage with thread-safe operations
- Docker containerization
- AWS ECS deployment with Terraform
- Built-in health checks

## API Endpoints

### 1. Get Product by ID

**Endpoint:** `GET /products/{productId}`

**Description:** Retrieve a product's details using its unique identifier

**Parameters:**
- `productId` (path, required): Integer >= 1

**Response Codes:**
- `200`: Product found successfully
- `404`: Product not found
- `500`: Internal server error

**Example Request:**
```bash
curl -X GET http://localhost:8080/products/1
```

**Example Response (200):**
```json
{
  "id": 1,
  "name": "Laptop",
  "description": "High-performance laptop",
  "price": 999.99,
  "stock": 10,
  "category": "Electronics",
  "imageUrl": ""
}
```

**Example Response (404):**
```json
{
  "code": 404,
  "message": "Product with ID 99 not found"
}
```

### 2. Add/Update Product Details

**Endpoint:** `POST /products/{productId}/details`

**Description:** Add or update detailed information for a specific product

**Parameters:**
- `productId` (path, required): Integer >= 1

**Request Body:** Product object (JSON)

**Response Codes:**
- `204`: Product details added successfully (No Content)
- `400`: Invalid input data
- `404`: Product not found
- `500`: Internal server error

**Example Request:**
```bash
curl -X POST http://localhost:8080/products/1/details \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Gaming Laptop",
    "description": "High-end gaming laptop with RTX 4090",
    "price": 2499.99,
    "stock": 5,
    "category": "Gaming",
    "imageUrl": "https://example.com/laptop.jpg"
  }'
```

**Example Response (204):**
No content (success)

**Example Response (400):**
```json
{
  "code": 400,
  "message": "Invalid product data: name is required, price and stock must be non-negative"
}
```

### 3. Health Check

**Endpoint:** `GET /health`

**Description:** Check if the service is running

**Example Request:**
```bash
curl -X GET http://localhost:8080/health
```

**Response:** `OK`

## Local Development

### Prerequisites

- Go 1.21 or higher
- Docker
- Make (optional)

### Running Locally

1. **Install dependencies:**
```bash
go mod init product-api
go get github.com/gorilla/mux
```

2. **Create go.mod file:**
```go
module product-api

go 1.21

require github.com/gorilla/mux v1.8.1
```

3. **Run the server:**
```bash
go run main.go
```

The server will start on port 8080 with 3 pre-seeded products (IDs: 1, 2, 3).

### Building Docker Image

```bash
# Build for local testing
docker build -t product-api:latest .

# Build for AMD64 (ECS deployment)
docker buildx build --platform linux/amd64 -t product-api:latest .

# Run container
docker run -p 8080:8080 product-api:latest
```

## Testing with Postman

### Import Collection

Create a Postman collection with these requests:

1. **Get Product 1**
   - Method: GET
   - URL: `{{base_url}}/products/1`
   - Tests: Check status code 200, validate JSON schema

2. **Get Non-existent Product**
   - Method: GET
   - URL: `{{base_url}}/products/999`
   - Tests: Check status code 404

3. **Update Product Details**
   - Method: POST
   - URL: `{{base_url}}/products/1/details`
   - Headers: `Content-Type: application/json`
   - Body:
   ```json
   {
     "name": "Updated Product",
     "description": "New description",
     "price": 149.99,
     "stock": 20,
     "category": "Updated"
   }
   ```
   - Tests: Check status code 204

4. **Invalid Product Update**
   - Method: POST
   - URL: `{{base_url}}/products/1/details`
   - Headers: `Content-Type: application/json`
   - Body:
   ```json
   {
     "price": -10
   }
   ```
   - Tests: Check status code 400

### Environment Variables

Create an environment in Postman:
- `base_url`: `http://localhost:8080` (local)
- `base_url`: `http://your-alb-dns.amazonaws.com` (AWS)

## Load Testing with Locust

### Install Locust
```bash
pip install locust
```

### Create locustfile.py
```python
from locust import HttpUser, FastHttpUser, task, between
import random
import json

class ProductAPIUser(HttpUser):
    wait_time = between(1, 3)
    
    @task(3)
    def get_product(self):
        product_id = random.randint(1, 3)
        self.client.get(f"/products/{product_id}")
    
    @task(1)
    def update_product(self):
        product_id = random.randint(1, 3)
        data = {
            "name": f"Product {product_id}",
            "description": "Updated description",
            "price": random.uniform(10, 1000),
            "stock": random.randint(1, 100),
            "category": "Test"
        }
        self.client.post(
            f"/products/{product_id}/details",
            json=data,
            headers={"Content-Type": "application/json"}
        )

class FastProductAPIUser(FastHttpUser):
    wait_time = between(1, 3)
    
    @task(3)
    def get_product(self):
        product_id = random.randint(1, 3)
        self.client.get(f"/products/{product_id}")
    
    @task(1) 
    def update_product(self):
        product_id = random.randint(1, 3)
        data = {
            "name": f"Product {product_id}",
            "description": "Updated description",
            "price": random.uniform(10, 1000),
            "stock": random.randint(1, 100),
            "category": "Test"
        }
        self.client.post(
            f"/products/{product_id}/details",
            json=data,
            headers={"Content-Type": "application/json"}
        )
```

### Run Load Tests
```bash
# Test with HttpUser
locust -f locustfile.py --host http://localhost:8080 --users 100 --spawn-rate 10

# Test with FastHttpUser (modify the class in locustfile.py)
locust -f locustfile.py --host http://localhost:8080 --users 100 --spawn-rate 10
```

### Cart Load Test Harness (Week 6c)
Use the Python-based harness to replay the cart workflow (50 create, add-items,
and get-cart requests each) and persist the latency distribution in
`mysql_test_results.json`.

```bash
# Optional one-time dependency installation
pip install requests

# Execute the harness against the running API (update --host as needed)
python3 cart_load_test.py --host http://localhost:8080
```

The script stores detailed per-request measurements in
`mysql_test_results.json`, using the following structure:

- `metadata`: timestamp, target host, and request counts
- `results.create_cart`, `results.add_items`, `results.get_cart`: arrays of
  individual operation outcomes (status, latency, and error details when
  applicable)

Commit the generated JSON artifact for the Week 6c comparison to keep a stable
baseline across environments.

## Deployment to AWS ECS

### Prerequisites
- AWS CLI configured with credentials
- Terraform installed
- Docker installed

### Steps

1. **Fork and clone the infrastructure repo:**
```bash
git clone https://github.com/YourUsername/CS6650_2b_demo.git
cd CS6650_2b_demo
```

2. **Replace the server code in `src/` with your implementation**

3. **Initialize and deploy with Terraform:**
```bash
cd terraform
terraform init
terraform plan
terraform apply
```

4. **Get the ALB URL from output:**
```bash
terraform output alb_url
```

5. **Test your deployed API:**
```bash
curl http://your-alb-url.amazonaws.com/products/1
```

## Performance Considerations

### Data Structure Choice
- Using `sync.RWMutex` allows multiple concurrent reads (GET operations)
- Single lock for writes ensures data consistency
- HashMap provides O(1) average case for lookups

### HttpUser vs FastHttpUser Differences
- **FastHttpUser**: Uses `geventhttpclient` for connection pooling
- **HttpUser**: Uses Python's `requests` library, creates new connections
- Differences are more noticeable with:
  - High request rates (>1000 RPS)
  - Small payloads
  - Low latency environments

### Expected Results
- GET operations should handle 1000+ RPS
- POST operations will be slower due to write locks
- Memory usage will grow with product count
- Consider adding an LRU cache for production

## Monitoring

Check CloudWatch logs:
```bash
aws logs tail /ecs/product-api --follow
```

## Cleanup

```bash
terraform destroy
```

## Security Notes

- Non-root user in Docker container
- Input validation on all endpoints
- No sensitive data in logs
- Health check endpoint for monitoring
