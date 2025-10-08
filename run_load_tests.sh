#!/bin/bash

# Load Testing Script for Product API
# This script provides various test scenarios to compare HttpUser vs FastHttpUser

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
HOST=${HOST:-"http://localhost:8080"}
RESULTS_DIR="load_test_results_$(date +%Y%m%d_%H%M%S)"

# Create results directory
mkdir -p "$RESULTS_DIR"

echo -e "${GREEN}Product API Load Testing Suite${NC}"
echo -e "${GREEN}================================${NC}"
echo "Host: $HOST"
echo "Results will be saved to: $RESULTS_DIR"
echo ""

# Function to run a test scenario
run_test() {
    local test_name=$1
    local user_class=$2
    local users=$3
    local spawn_rate=$4
    local run_time=$5
    local description=$6
    
    echo -e "${YELLOW}Running Test: $test_name${NC}"
    echo "Description: $description"
    echo "User Class: $user_class"
    echo "Users: $users, Spawn Rate: $spawn_rate, Duration: $run_time"
    echo "-------------------------------------------"
    
    # Run locust test
    locust -f locustfile.py \
        --host "$HOST" \
        --users "$users" \
        --spawn-rate "$spawn_rate" \
        --run-time "$run_time" \
        --headless \
        --only-summary \
        --csv "$RESULTS_DIR/${test_name}" \
        --html "$RESULTS_DIR/${test_name}.html" \
        --class-picker \
        -L INFO \
        "$user_class" 2>&1 | tee "$RESULTS_DIR/${test_name}.log"
    
    echo -e "${GREEN}Test $test_name completed!${NC}"
    echo ""
    sleep 5  # Give the system time to recover between tests
}

# Function to run comparison tests
run_comparison_test() {
    local users=$1
    local spawn_rate=$2
    local run_time=$3
    
    echo -e "${YELLOW}=== Comparison Test: HttpUser vs FastHttpUser ===${NC}"
    echo "Configuration: $users users, spawn rate $spawn_rate, duration $run_time"
    echo ""
    
    # Test with HttpUser
    run_test \
        "comparison_httpuser_${users}u" \
        "ProductAPIUser" \
        "$users" \
        "$spawn_rate" \
        "$run_time" \
        "Standard HttpUser implementation"
    
    # Test with FastHttpUser
    run_test \
        "comparison_fasthttpuser_${users}u" \
        "FastProductAPIUser" \
        "$users" \
        "$spawn_rate" \
        "$run_time" \
        "FastHttpUser with connection pooling"
}

# Main menu
echo "Select test scenario:"
echo "1) Quick Test (10 users, 30 seconds)"
echo "2) Standard Test (50 users, 2 minutes)"
echo "3) Heavy Load Test (200 users, 5 minutes)"
echo "4) Stress Test (500 users, 5 minutes)"
echo "5) Comparison Suite (runs multiple scenarios)"
echo "6) Custom Test"
echo "7) Exit"
echo ""

read -p "Enter choice [1-7]: " choice

case $choice in
    1)
        echo -e "${GREEN}Starting Quick Test...${NC}"
        run_comparison_test 10 2 30s
        ;;
    
    2)
        echo -e "${GREEN}Starting Standard Test...${NC}"
        run_comparison_test 50 5 2m
        ;;
    
    3)
        echo -e "${GREEN}Starting Heavy Load Test...${NC}"
        run_comparison_test 200 10 5m
        ;;
    
    4)
        echo -e "${GREEN}Starting Stress Test...${NC}"
        run_test \
            "stress_test" \
            "StressTestUser" \
            500 \
            20 \
            5m \
            "Aggressive stress testing with minimal wait times"
        ;;
    
    5)
        echo -e "${GREEN}Starting Comparison Suite...${NC}"
        echo "This will run multiple test scenarios. Total time: ~15 minutes"
        echo ""
        
        # Light load
        echo -e "${YELLOW}Phase 1: Light Load${NC}"
        run_comparison_test 10 2 1m
        
        # Medium load
        echo -e "${YELLOW}Phase 2: Medium Load${NC}"
        run_comparison_test 50 5 2m
        
        # Heavy load
        echo -e "${YELLOW}Phase 3: Heavy Load${NC}"
        run_comparison_test 100 10 2m
        
        # Very heavy load
        echo -e "${YELLOW}Phase 4: Very Heavy Load${NC}"
        run_comparison_test 200 15 3m
        
        echo -e "${GREEN}Comparison Suite Completed!${NC}"
        ;;
    
    6)
        echo "Custom Test Configuration"
        read -p "Enter number of users: " custom_users
        read -p "Enter spawn rate: " custom_spawn
        read -p "Enter run time (e.g., 30s, 2m, 1h): " custom_time
        read -p "User class (1=HttpUser, 2=FastHttpUser, 3=StressTest, 4=MixedBehavior): " custom_class
        
        case $custom_class in
            1) class_name="ProductAPIUser" ;;
            2) class_name="FastProductAPIUser" ;;
            3) class_name="StressTestUser" ;;
            4) class_name="MixedBehaviorUser" ;;
            *) class_name="ProductAPIUser" ;;
        esac
        
        run_test \
            "custom_test" \
            "$class_name" \
            "$custom_users" \
            "$custom_spawn" \
            "$custom_time" \
            "Custom test configuration"
        ;;
    
    7)
        echo "Exiting..."
        exit 0
        ;;
    
    *)
        echo -e "${RED}Invalid choice. Exiting...${NC}"
        exit 1
        ;;
esac

# Generate summary report
echo -e "${GREEN}Generating Summary Report...${NC}"

cat > "$RESULTS_DIR/summary.md" << EOF
# Load Test Results Summary

**Date:** $(date)
**Host:** $HOST
**Results Directory:** $RESULTS_DIR

## Test Scenarios Run

EOF

# Add CSV results summary if files exist
for csv in "$RESULTS_DIR"/*_stats.csv; do
    if [ -f "$csv" ]; then
        test_name=$(basename "$csv" _stats.csv)
        echo "### $test_name" >> "$RESULTS_DIR/summary.md"
        echo '```' >> "$RESULTS_DIR/summary.md"
        head -n 10 "$csv" >> "$RESULTS_DIR/summary.md"
        echo '```' >> "$RESULTS_DIR/summary.md"
        echo "" >> "$RESULTS_DIR/summary.md"
    fi
done

echo -e "${GREEN}All tests completed!${NC}"
echo "Results saved to: $RESULTS_DIR"
echo ""
echo "Files generated:"
ls -la "$RESULTS_DIR"/*.csv 2>/dev/null | awk '{print "  - CSV: " $9}'
ls -la "$RESULTS_DIR"/*.html 2>/dev/null | awk '{print "  - HTML Report: " $9}'
ls -la "$RESULTS_DIR"/*.log 2>/dev/null | awk '{print "  - Log: " $9}'
echo "  - Summary: $RESULTS_DIR/summary.md"

# Open HTML report if available (macOS/Linux)
if command -v open &> /dev/null; then
    # macOS
    first_html=$(ls "$RESULTS_DIR"/*.html 2>/dev/null | head -n 1)
    if [ -n "$first_html" ]; then
        echo ""
        read -p "Open HTML report in browser? (y/n): " open_report
        if [ "$open_report" = "y" ]; then
            open "$first_html"
        fi
    fi
elif command -v xdg-open &> /dev/null; then
    # Linux
    first_html=$(ls "$RESULTS_DIR"/*.html 2>/dev/null | head -n 1)
    if [ -n "$first_html" ]; then
        echo ""
        read -p "Open HTML report in browser? (y/n): " open_report
        if [ "$open_report" = "y" ]; then
            xdg-open "$first_html"
        fi
    fi
fi