#!/bin/bash

# Comprehensive test script for job_scorer
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_section() {
    echo -e "\n${BLUE}================================${NC}"
    echo -e "${BLUE} $1${NC}"
    echo -e "${BLUE}================================${NC}\n"
}

# Check if Go is installed
if ! command -v go &> /dev/null; then
    print_error "Go is not installed or not in PATH"
    exit 1
fi

print_status "Go version: $(go version)"

# Clean previous test results
print_section "Cleaning Previous Test Results"
rm -f coverage.out coverage.html
rm -rf test_output_*

# Set test environment variables
export GROQ_API_KEY="test-key"
export SMTP_HOST="smtp.test.com"
export SMTP_PORT="587"
export SMTP_USER="test@example.com"
export SMTP_PASS="testpass"
export SMTP_FROM="test@example.com"
export SMTP_TO="recipient@example.com"

# Run linting
print_section "Running Go Linting"
if command -v golangci-lint &> /dev/null; then
    golangci-lint run ./...
else
    print_warning "golangci-lint not found. Running basic go vet instead."
    go vet ./...
fi

# Format check
print_section "Checking Code Formatting"
UNFORMATTED=$(go fmt ./...)
if [ -n "$UNFORMATTED" ]; then
    print_error "Code is not properly formatted. Run 'go fmt ./...' to fix."
    exit 1
else
    print_status "Code is properly formatted"
fi

# Run unit tests with coverage
print_section "Running Unit Tests with Coverage"
print_status "Running tests for all packages..."

# Create coverage directory
mkdir -p coverage

# Test individual packages with detailed output
packages=(
    "./models"
    "./config" 
    "./utils"
    "./services/storage"
    "./services/filter"
    "./services/evaluator"
)

failed_packages=()
total_coverage=0
package_count=0

for package in "${packages[@]}"; do
    package_name=$(basename "$package")
    print_status "Testing package: $package"
    
    if go test -v -race -coverprofile="coverage/${package_name}.out" "$package"; then
        print_status "✓ $package tests passed"
        
        # Extract coverage percentage
        if [ -f "coverage/${package_name}.out" ]; then
            coverage=$(go tool cover -func="coverage/${package_name}.out" | grep total | awk '{print $3}' | sed 's/%//')
            print_status "Coverage for $package: ${coverage}%"
            
            # Add to total coverage calculation
            if [ -n "$coverage" ]; then
                total_coverage=$(echo "$total_coverage + $coverage" | bc -l)
                package_count=$((package_count + 1))
            fi
        fi
    else
        print_error "✗ $package tests failed"
        failed_packages+=("$package")
    fi
    echo ""
done

# Combine coverage files
print_section "Combining Coverage Reports"
echo "mode: set" > coverage.out
for f in coverage/*.out; do
    if [ -f "$f" ]; then
        sed '1d' "$f" >> coverage.out
    fi
done

# Generate HTML coverage report
if [ -f coverage.out ]; then
    go tool cover -html=coverage.out -o coverage.html
    print_status "HTML coverage report generated: coverage.html"
    
    # Calculate average coverage
    if [ $package_count -gt 0 ]; then
        avg_coverage=$(echo "scale=2; $total_coverage / $package_count" | bc -l)
        print_status "Average test coverage: ${avg_coverage}%"
    fi
fi

# Run integration tests (if any exist)
print_section "Running Integration Tests"
if go test -v -tags=integration ./...; then
    print_status "✓ Integration tests passed"
else
    print_warning "Integration tests failed or none found"
fi

# Run race condition detection
print_section "Running Race Condition Detection"
if go test -race ./...; then
    print_status "✓ No race conditions detected"
else
    print_error "Race conditions detected!"
    failed_packages+=("race-detection")
fi

# Test build
print_section "Testing Application Build"
if go build -o job-scorer-test .; then
    print_status "✓ Application builds successfully"
    rm -f job-scorer-test
else
    print_error "✗ Application build failed"
    failed_packages+=("build")
fi

# Performance benchmarks (if any exist)
print_section "Running Benchmarks"
if go test -bench=. -benchmem ./... > benchmark.txt 2>&1; then
    print_status "✓ Benchmarks completed"
    if [ -s benchmark.txt ]; then
        print_status "Benchmark results saved to benchmark.txt"
    fi
else
    print_warning "No benchmarks found or benchmarks failed"
fi

# Security checks (basic)
print_section "Basic Security Checks"
print_status "Checking for common security issues..."

# Check for hardcoded credentials (basic check)
if grep -r "password\|secret\|key" --include="*.go" . | grep -v "test" | grep -v "example" | grep -v "// " | grep -v "Password" | grep -v "SecretKey" | grep -v "APIKey"; then
    print_warning "Potential hardcoded credentials found (review manually)"
else
    print_status "✓ No obvious hardcoded credentials found"
fi

# Final report
print_section "Test Summary"

if [ ${#failed_packages[@]} -eq 0 ]; then
    print_status "🎉 All tests passed successfully!"
    echo -e "${GREEN}✓ Code formatting: PASS${NC}"
    echo -e "${GREEN}✓ Unit tests: PASS${NC}"
    echo -e "${GREEN}✓ Race detection: PASS${NC}"
    echo -e "${GREEN}✓ Application build: PASS${NC}"
    
    if [ $package_count -gt 0 ]; then
        avg_coverage=$(echo "scale=2; $total_coverage / $package_count" | bc -l)
        echo -e "${GREEN}✓ Test coverage: ${avg_coverage}%${NC}"
    fi
    
    print_status "Coverage report: coverage.html"
    exit 0
else
    print_error "❌ Some tests failed!"
    echo -e "${RED}Failed packages:${NC}"
    for pkg in "${failed_packages[@]}"; do
        echo -e "${RED}  - $pkg${NC}"
    done
    exit 1
fi 