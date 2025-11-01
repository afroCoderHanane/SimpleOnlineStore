#!/usr/bin/env python3
"""Cart load test harness for SimpleOnlineStore.

This script performs a fixed number of cart-related operations against the
running API and captures latency statistics for each request. The aggregated
results are stored in ``mysql_test_results.json`` to support the Week 6c
comparison exercises.
"""
from __future__ import annotations

import argparse
import json
import random
import time
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional

import requests
from requests.exceptions import RequestException


@dataclass
class OperationResult:
    """Represents the outcome of a single HTTP operation."""

    sequence: int
    operation: str
    success: bool
    status_code: Optional[int]
    response_time_ms: float
    response_body: Optional[Any]
    error: Optional[str] = None

    def to_dict(self) -> Dict[str, Any]:
        payload = asdict(self)
        # The JSON serializer cannot handle Response objects, so make sure the
        # response body is JSON-serialisable (``requests`` already gives us a
        # Python structure or a string).
        return payload


def _now_iso() -> str:
    """Return the current UTC timestamp in ISO-8601 format."""

    return datetime.now(timezone.utc).isoformat()


def _send_request(method: str, url: str, *, session: requests.Session, json_body: Optional[Dict[str, Any]] = None) -> OperationResult:
    """Send an HTTP request and convert the result into an OperationResult."""

    start = time.perf_counter()
    try:
        response = session.request(method, url, json=json_body, timeout=10)
        elapsed_ms = (time.perf_counter() - start) * 1000
        body: Any
        try:
            body = response.json()
        except ValueError:
            body = response.text
        return OperationResult(
            sequence=0,  # The caller will fill this in later.
            operation="",
            success=response.ok,
            status_code=response.status_code,
            response_time_ms=round(elapsed_ms, 2),
            response_body=body,
        )
    except RequestException as exc:
        elapsed_ms = (time.perf_counter() - start) * 1000
        return OperationResult(
            sequence=0,
            operation="",
            success=False,
            status_code=None,
            response_time_ms=round(elapsed_ms, 2),
            response_body=None,
            error=str(exc),
        )


def perform_operations(host: str, operations_per_type: int) -> Dict[str, List[Dict[str, Any]]]:
    """Execute the load test flow for the specified number of iterations."""

    results: Dict[str, List[Dict[str, Any]]] = {
        "create_cart": [],
        "add_items": [],
        "get_cart": [],
    }

    created_cart_ids: List[int] = []
    session = requests.Session()

    # Phase 1: Create carts.
    for i in range(operations_per_type):
        payload = {"customer_id": i + 1}
        result = _send_request(
            "POST",
            f"{host}/shopping-carts",
            session=session,
            json_body=payload,
        )
        result.sequence = i + 1
        result.operation = "create_cart"

        cart_id: Optional[int] = None
        if isinstance(result.response_body, dict):
            cart_id = result.response_body.get("shopping_cart_id")
        if result.success and cart_id is not None:
            created_cart_ids.append(int(cart_id))
        else:
            result.success = False
        results["create_cart"].append(result.to_dict())

    # If fewer carts than expected were created, re-use successful IDs to keep
    # issuing the required number of requests in the follow-up phases.
    def _select_cart(index: int) -> Optional[int]:
        if not created_cart_ids:
            return None
        return created_cart_ids[index % len(created_cart_ids)]

    # Phase 2: Add items to cart(s).
    for i in range(operations_per_type):
        cart_id = _select_cart(i)
        if cart_id is None:
            result = OperationResult(
                sequence=i + 1,
                operation="add_items",
                success=False,
                status_code=None,
                response_time_ms=0.0,
                response_body=None,
                error="No shopping carts available from creation phase",
            )
        else:
            payload = {
                "product_id": random.randint(1, 3),
                "quantity": random.randint(1, 5),
            }
            result = _send_request(
                "POST",
                f"{host}/shopping-carts/{cart_id}/items",
                session=session,
                json_body=payload,
            )
            result.sequence = i + 1
            result.operation = "add_items"
            if result.success:
                # Successful add-items requests return 204 (no JSON body). For
                # consistency, mark success only for 2xx codes.
                result.success = result.status_code is not None and 200 <= result.status_code < 300
        results["add_items"].append(result.to_dict())

    # Phase 3: Retrieve cart snapshots.
    for i in range(operations_per_type):
        cart_id = _select_cart(i)
        if cart_id is None:
            result = OperationResult(
                sequence=i + 1,
                operation="get_cart",
                success=False,
                status_code=None,
                response_time_ms=0.0,
                response_body=None,
                error="No shopping carts available from creation phase",
            )
        else:
            result = _send_request(
                "GET",
                f"{host}/shopping-carts/{cart_id}",
                session=session,
            )
            result.sequence = i + 1
            result.operation = "get_cart"
            if result.success:
                result.success = result.status_code is not None and 200 <= result.status_code < 300
        results["get_cart"].append(result.to_dict())

    session.close()
    return results


def write_results_file(results: Dict[str, List[Dict[str, Any]]], host: str, operations_per_type: int) -> Path:
    """Persist the run metadata and per-operation records to disk."""

    output_path = Path("mysql_test_results.json")
    payload = {
        "metadata": {
            "generated_at": _now_iso(),
            "host": host,
            "operations_per_type": operations_per_type,
            "operation_totals": {
                key: len(value) for key, value in results.items()
            },
        },
        "results": results,
    }
    output_path.write_text(json.dumps(payload, indent=2, sort_keys=True))
    return output_path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run cart load test sequence")
    parser.add_argument(
        "--host",
        default="http://localhost:8080",
        help="Base URL for the API service (default: %(default)s)",
    )
    parser.add_argument(
        "--operations",
        type=int,
        default=50,
        help="Number of operations per request type (default: %(default)s)",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    print(f"Running cart load test against {args.host}")
    print(f"Operations per type: {args.operations}")

    results = perform_operations(args.host.rstrip("/"), args.operations)
    output_path = write_results_file(results, args.host.rstrip("/"), args.operations)

    def _summarise(operation: str) -> str:
        dataset = results[operation]
        successes = sum(1 for item in dataset if item["success"])
        failures = len(dataset) - successes
        latencies = [item["response_time_ms"] for item in dataset if item["status_code"]]
        avg_latency = sum(latencies) / len(latencies) if latencies else 0.0
        return (
            f"{operation}: {successes} success(es), {failures} failure(s), "
            f"average latency {avg_latency:.2f} ms"
        )

    print("Summary:")
    for op in ("create_cart", "add_items", "get_cart"):
        print("  - " + _summarise(op))

    print(f"Detailed results written to {output_path}")


if __name__ == "__main__":
    main()
