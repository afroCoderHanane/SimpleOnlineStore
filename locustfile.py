"""
Locust load testing file for Product API
Tests both HttpUser and FastHttpUser implementations
"""

from locust import HttpUser, FastHttpUser, task, between
import random
import json
import time

# Configuration
PRODUCT_IDS = [1, 2, 3]  # Pre-seeded product IDs
CATEGORIES = ["Electronics", "Gaming", "Office", "Home", "Sports"]


def ensure_cart(user):
    """Ensure the user has an active shopping cart by creating one if necessary."""
    cart_id = getattr(user, "cart_id", None)
    if cart_id:
        return True

    response = user.client.post(
        "/shopping-carts",
        json={"customer_id": random.randint(1, 100000)},
        name="/shopping-carts",
        catch_response=True,
    )
    with response as resp:
        if resp.status_code == 201:
            try:
                data = resp.json()
            except ValueError:
                resp.failure("Invalid JSON from create cart")
                return False
            user.cart_id = data.get("shopping_cart_id")
            user.cart_items = 0
            resp.success()
            return True
        resp.failure(f"Create cart failed with {resp.status_code}")
        return False


class ProductAPIUser(HttpUser):
    """
    Standard HttpUser implementation using Python requests library.
    Creates new TCP connections for each request.
    """
    
    wait_time = between(1, 3)  # Wait 1-3 seconds between tasks
    
    def on_start(self):
        """Called when a user starts before any task is scheduled"""
        self.products_viewed = []
        self.products_updated = []
        self.cart_id = None
        self.cart_items = 0
    
    @task(3)  # Weight of 3 - this task is 3x more likely than weight 1 tasks
    def get_product(self):
        """Simulate browsing products - most common operation"""
        product_id = random.choice(PRODUCT_IDS)
        
        with self.client.get(
            f"/products/{product_id}",
            catch_response=True,
            name="/products/[id]"  # Group all product GETs in stats
        ) as response:
            if response.status_code == 200:
                self.products_viewed.append(product_id)
                response.success()
            elif response.status_code == 404:
                response.failure(f"Product {product_id} not found")
            else:
                response.failure(f"Got unexpected status {response.status_code}")
    
    @task(1)  # Weight of 1 - less frequent operation
    def update_product_details(self):
        """Simulate updating product information"""
        product_id = random.choice(PRODUCT_IDS)
        
        # Generate random product data
        product_data = {
            "name": f"Product {product_id} - Updated at {int(time.time())}",
            "description": f"Updated description for testing - {random.randint(1, 1000)}",
            "price": round(random.uniform(10.99, 999.99), 2),
            "stock": random.randint(0, 100),
            "category": random.choice(CATEGORIES),
            "imageUrl": f"https://example.com/product{product_id}.jpg"
        }
        
        with self.client.post(
            f"/products/{product_id}/details",
            json=product_data,
            catch_response=True,
            name="/products/[id]/details"  # Group all product POSTs in stats
        ) as response:
            if response.status_code == 204:
                self.products_updated.append(product_id)
                response.success()
            elif response.status_code == 404:
                response.failure(f"Product {product_id} not found for update")
            elif response.status_code == 400:
                response.failure(f"Invalid data for product {product_id}")
            else:
                response.failure(f"Got unexpected status {response.status_code}")
    
    @task(1)
    def get_nonexistent_product(self):
        """Occasionally test 404 handling"""
        product_id = random.randint(100, 999)  # IDs that don't exist

        with self.client.get(
            f"/products/{product_id}",
            catch_response=True,
            name="/products/[nonexistent]"
        ) as response:
            if response.status_code == 404:
                response.success()  # 404 is expected here
            else:
                response.failure(f"Expected 404 but got {response.status_code}")

    @task(1)
    def add_item_to_cart(self):
        """Add an item to the active shopping cart."""
        if not ensure_cart(self):
            return

        product_id = random.choice(PRODUCT_IDS)
        quantity = random.randint(1, 3)

        with self.client.post(
            f"/shopping-carts/{self.cart_id}/items",
            json={"product_id": product_id, "quantity": quantity},
            catch_response=True,
            name="/shopping-carts/[id]/items",
        ) as response:
            if response.status_code == 204:
                self.cart_items += quantity
                response.success()
            elif response.status_code in (404, 400):
                response.failure(f"Cart item update failed: {response.status_code}")
            else:
                response.failure(f"Unexpected status {response.status_code}")

    @task(1)
    def checkout_cart(self):
        """Checkout the cart when it has items."""
        if not getattr(self, "cart_id", None) or getattr(self, "cart_items", 0) == 0:
            return

        with self.client.post(
            f"/shopping-carts/{self.cart_id}/checkout",
            name="/shopping-carts/[id]/checkout",
            catch_response=True,
        ) as response:
            if response.status_code == 200:
                try:
                    order = response.json()
                except ValueError:
                    response.failure("Invalid JSON response from checkout")
                    return
                if order.get("order_id"):
                    response.success()
                else:
                    response.failure("Missing order_id in checkout response")
                self.cart_id = None
                self.cart_items = 0
            elif response.status_code in (400, 404):
                response.failure(f"Checkout failed: {response.status_code}")
            else:
                response.failure(f"Unexpected checkout status {response.status_code}")
    
    def on_stop(self):
        """Called when the user stops"""
        print(f"User viewed {len(self.products_viewed)} products and updated {len(self.products_updated)}")


class FastProductAPIUser(FastHttpUser):
    """
    FastHttpUser implementation using geventhttpclient.
    Maintains persistent connections with connection pooling.
    Better for high-throughput scenarios.
    """
    
    wait_time = between(1, 3)
    
    # Connection pool settings
    connection_timeout = 60.0
    network_timeout = 60.0
    
    def on_start(self):
        """Called when a user starts before any task is scheduled"""
        self.products_viewed = []
        self.products_updated = []
        self.cart_id = None
        self.cart_items = 0
    
    @task(3)
    def get_product(self):
        """Simulate browsing products - most common operation"""
        product_id = random.choice(PRODUCT_IDS)
        
        with self.client.get(
            f"/products/{product_id}",
            catch_response=True,
            name="/products/[id]"
        ) as response:
            if response.status_code == 200:
                self.products_viewed.append(product_id)
                response.success()
            elif response.status_code == 404:
                response.failure(f"Product {product_id} not found")
            else:
                response.failure(f"Got unexpected status {response.status_code}")
    
    @task(1)
    def update_product_details(self):
        """Simulate updating product information"""
        product_id = random.choice(PRODUCT_IDS)
        
        product_data = {
            "name": f"Product {product_id} - Fast Updated at {int(time.time())}",
            "description": f"Fast update description - {random.randint(1, 1000)}",
            "price": round(random.uniform(10.99, 999.99), 2),
            "stock": random.randint(0, 100),
            "category": random.choice(CATEGORIES),
            "imageUrl": f"https://example.com/product{product_id}.jpg"
        }
        
        with self.client.post(
            f"/products/{product_id}/details",
            json=product_data,
            catch_response=True,
            name="/products/[id]/details"
        ) as response:
            if response.status_code == 204:
                self.products_updated.append(product_id)
                response.success()
            elif response.status_code == 404:
                response.failure(f"Product {product_id} not found for update")
            elif response.status_code == 400:
                response.failure(f"Invalid data for product {product_id}")
            else:
                response.failure(f"Got unexpected status {response.status_code}")
    
    @task(1)
    def get_nonexistent_product(self):
        """Occasionally test 404 handling"""
        product_id = random.randint(100, 999)

        with self.client.get(
            f"/products/{product_id}",
            catch_response=True,
            name="/products/[nonexistent]"
        ) as response:
            if response.status_code == 404:
                response.success()
            else:
                response.failure(f"Expected 404 but got {response.status_code}")

    @task(1)
    def add_item_to_cart(self):
        if not ensure_cart(self):
            return

        product_id = random.choice(PRODUCT_IDS)
        quantity = random.randint(1, 3)

        with self.client.post(
            f"/shopping-carts/{self.cart_id}/items",
            json={"product_id": product_id, "quantity": quantity},
            catch_response=True,
            name="/shopping-carts/[id]/items",
        ) as response:
            if response.status_code == 204:
                self.cart_items += quantity
                response.success()
            elif response.status_code in (400, 404):
                response.failure(f"Cart item update failed: {response.status_code}")
            else:
                response.failure(f"Unexpected status {response.status_code}")

    @task(1)
    def checkout_cart(self):
        if not getattr(self, "cart_id", None) or getattr(self, "cart_items", 0) == 0:
            return

        with self.client.post(
            f"/shopping-carts/{self.cart_id}/checkout",
            name="/shopping-carts/[id]/checkout",
            catch_response=True,
        ) as response:
            if response.status_code == 200:
                try:
                    order = response.json()
                except ValueError:
                    response.failure("Invalid JSON response from checkout")
                    return
                if order.get("order_id"):
                    response.success()
                else:
                    response.failure("Missing order_id in checkout response")
                self.cart_id = None
                self.cart_items = 0
            elif response.status_code in (400, 404):
                response.failure(f"Checkout failed: {response.status_code}")
            else:
                response.failure(f"Unexpected checkout status {response.status_code}")
    
    def on_stop(self):
        """Called when the user stops"""
        print(f"FastUser viewed {len(self.products_viewed)} products and updated {len(self.products_updated)}")


class StressTestUser(FastHttpUser):
    """
    Aggressive stress testing configuration with minimal wait time.
    Used to find the breaking point of the system.
    """
    
    wait_time = between(0.1, 0.5)  # Much shorter wait times
    connection_timeout = 30.0
    network_timeout = 30.0

    def on_start(self):
        self.cart_id = None
        self.cart_items = 0
    
    @task(10)  # Heavy emphasis on reads
    def rapid_get(self):
        """Rapid-fire GET requests"""
        product_id = random.choice(PRODUCT_IDS)
        self.client.get(
            f"/products/{product_id}",
            name="/products/[id]"
        )
    
    @task(1)  # Occasional writes
    def occasional_update(self):
        """Less frequent updates during stress test"""
        product_id = random.choice(PRODUCT_IDS)
        self.client.post(
            f"/products/{product_id}/details",
            json={
                "name": f"Stress Test Product {product_id}",
                "description": "Minimal data for speed",
                "price": 99.99,
                "stock": 10
            },
            name="/products/[id]/details"
        )

    @task(1)
    def cart_mutation(self):
        if not ensure_cart(self):
            return

        product_id = random.choice(PRODUCT_IDS)
        quantity = random.randint(1, 5)
        self.client.post(
            f"/shopping-carts/{self.cart_id}/items",
            json={"product_id": product_id, "quantity": quantity},
            name="/shopping-carts/[id]/items",
        )
        self.cart_items += quantity

    @task(1)
    def cart_checkout(self):
        if getattr(self, "cart_id", None) and getattr(self, "cart_items", 0) > 0:
            response = self.client.post(
                f"/shopping-carts/{self.cart_id}/checkout",
                name="/shopping-carts/[id]/checkout",
            )
            if response.status_code == 200:
                self.cart_id = None
                self.cart_items = 0


class MixedBehaviorUser(HttpUser):
    """
    Simulates more realistic user behavior patterns.
    Includes browsing sessions, comparison shopping, and occasional updates.
    """
    
    wait_time = between(2, 5)
    
    def on_start(self):
        self.cart_id = None
        self.cart_items = 0
    
    @task(5)
    def browse_products(self):
        """Simulate a browsing session - look at multiple products"""
        num_products = random.randint(1, 3)
        for _ in range(num_products):
            product_id = random.choice(PRODUCT_IDS)
            response = self.client.get(
                f"/products/{product_id}",
                name="/products/[id]"
            )
            if response.status_code == 200:
                # Simulate thinking time while looking at product
                time.sleep(random.uniform(0.5, 2))
                
                if random.random() > 0.7 and ensure_cart(self):
                    self.client.post(
                        f"/shopping-carts/{self.cart_id}/items",
                        json={"product_id": product_id, "quantity": 1},
                        name="/shopping-carts/[id]/items",
                    )
                    self.cart_items += 1
    
    @task(2)
    def compare_products(self):
        """Simulate comparing multiple products quickly"""
        for product_id in PRODUCT_IDS:
            self.client.get(
                f"/products/{product_id}",
                name="/products/[id]"
            )
            time.sleep(0.1)  # Quick comparison
    
    @task(1)
    def admin_update(self):
        """Simulate admin updating product information"""
        product_id = random.choice(PRODUCT_IDS)
        
        # More realistic update - maybe just updating stock
        update_type = random.choice(["stock", "price", "full"])
        
        if update_type == "stock":
            data = {
                "name": f"Product {product_id}",
                "description": "In stock item",
                "price": 99.99,
                "stock": random.randint(0, 50)
            }
        elif update_type == "price":
            data = {
                "name": f"Product {product_id}",
                "description": "Price updated",
                "price": round(random.uniform(50, 200), 2),
                "stock": 25
            }
        else:  # full update
            data = {
                "name": f"Product {product_id} - New Version",
                "description": "Completely updated product information with new features",
                "price": round(random.uniform(100, 500), 2),
                "stock": random.randint(10, 100),
                "category": random.choice(CATEGORIES),
                "imageUrl": f"https://example.com/new/product{product_id}.jpg"
            }
        
        self.client.post(
            f"/products/{product_id}/details",
            json=data,
            name="/products/[id]/details"
        )

    @task(1)
    def periodic_checkout(self):
        if getattr(self, "cart_id", None) and getattr(self, "cart_items", 0) > 0:
            response = self.client.post(
                f"/shopping-carts/{self.cart_id}/checkout",
                name="/shopping-carts/[id]/checkout",
            )
            if response.status_code == 200:
                self.cart_id = None
                self.cart_items = 0