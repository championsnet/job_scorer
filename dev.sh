#!/bin/bash

# Development script with hot reloading for job_scorer
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${GREEN}[DEV]${NC} $1"
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

# Function to install Air (hot reloading tool)
install_air() {
    print_status "Installing Air for hot reloading..."
    
    # Ensure Go bin is in PATH
    export PATH=$PATH:~/go/bin
    
    if command -v air &> /dev/null; then
        print_status "Air is already installed"
        return 0
    fi
    
    # Try to install Air
    if command -v go &> /dev/null; then
        go install github.com/air-verse/air@latest
        if command -v air &> /dev/null; then
            print_status "Air installed successfully"
            return 0
        fi
    fi
    
    # If Air installation failed, we'll use a simple file watcher
    print_warning "Could not install Air. Will use basic file watching instead."
    return 1
}

# Function to check if .env file exists
check_env() {
    if [ ! -f .env ]; then
        print_warning ".env file not found. Creating from template..."
        if [ -f env.example ]; then
            cp env.example .env
            print_status "Created .env from env.example"
            print_warning "Please edit .env with your actual configuration"
        else
            print_error "env.example not found. Please create .env manually"
            return 1
        fi
    else
        print_status ".env file found"
    fi
}

# Function to run tests continuously
run_tests_continuous() {
    print_section "Running Tests Continuously"
    
    # Function to run tests
    run_tests() {
        clear
        echo -e "${BLUE}Running tests at $(date)${NC}"
        echo "================================"
        
        # Run tests with short output
        if go test -v ./...; then
            echo -e "${GREEN}✓ All tests passed${NC}"
        else
            echo -e "${RED}✗ Some tests failed${NC}"
        fi
        
        echo ""
        echo -e "${YELLOW}Watching for changes... (Press Ctrl+C to stop)${NC}"
    }
    
    # Run tests initially
    run_tests
    
    # Watch for changes using inotifywait (Linux) or fswatch (macOS)
    if command -v inotifywait &> /dev/null; then
        # Linux
        while inotifywait -r -e modify,create,delete --include='\.go$' .; do
            sleep 1  # Debounce rapid changes
            run_tests
        done
    elif command -v fswatch &> /dev/null; then
        # macOS
        fswatch -o -e ".*" -i "\\.go$" . | while read num; do
            sleep 1  # Debounce rapid changes
            run_tests
        done
    else
        print_error "File watching requires 'inotifywait' (Linux) or 'fswatch' (macOS)"
        print_status "Install with: apt-get install inotify-tools (Ubuntu) or brew install fswatch (macOS)"
        return 1
    fi
}

# Function to run application with hot reloading
run_with_hot_reload() {
    print_section "Starting Development Server with Hot Reloading"
    
    # Ensure Go bin is in PATH
    export PATH=$PATH:~/go/bin
    
    check_env || return 1
    
    # Create .air.toml config if it doesn't exist
    if [ ! -f .air.toml ]; then
        print_status "Creating Air configuration file..."
        cat > .air.toml << 'EOF'
root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
  args_bin = []
  bin = "./tmp/job-scorer"
  cmd = "go build -o ./tmp/job-scorer ."
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor", "testdata", "node_modules", "coverage"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  full_bin = ""
  include_dir = []
  include_ext = ["go", "tpl", "tmpl", "html"]
  include_file = []
  kill_delay = "0s"
  log = "build-errors.log"
  rerun = false
  rerun_delay = 500
  send_interrupt = false
  stop_on_root = false

[color]
  app = ""
  build = "yellow"
  main = "magenta"
  runner = "green"
  watcher = "cyan"

[log]
  main_only = false
  time = false

[misc]
  clean_on_exit = false

[screen]
  clear_on_rebuild = false
  keep_scroll = true
EOF
        print_status "Created .air.toml configuration"
    fi
    
    # Create tmp directory
    mkdir -p tmp
    
    # Try to use Air first
    if command -v air &> /dev/null; then
        print_status "Starting Air hot reloader..."
        air
    else
        print_warning "Air not available. Using basic file watching..."
        run_basic_hot_reload
    fi
}

# Basic hot reload implementation
run_basic_hot_reload() {
    local app_pid=""
    
    # Function to build and run app
    build_and_run() {
        # Kill previous instance
        if [ -n "$app_pid" ] && kill -0 $app_pid 2>/dev/null; then
            print_status "Stopping previous instance (PID: $app_pid)"
            kill $app_pid
            sleep 1
        fi
        
        # Build application
        print_status "Building application..."
        if go build -o ./tmp/job-scorer .; then
            print_status "Build successful, starting application..."
            ./tmp/job-scorer &
            app_pid=$!
            print_status "Application started (PID: $app_pid)"
        else
            print_error "Build failed"
        fi
    }
    
    # Initial build and run
    mkdir -p tmp
    build_and_run
    
    # Cleanup function
    cleanup() {
        print_status "Cleaning up..."
        if [ -n "$app_pid" ] && kill -0 $app_pid 2>/dev/null; then
            kill $app_pid
        fi
        exit 0
    }
    trap cleanup INT TERM
    
    # Watch for changes
    print_status "Watching for changes... (Press Ctrl+C to stop)"
    
    if command -v inotifywait &> /dev/null; then
        # Linux
        while inotifywait -r -e modify,create,delete --include='\.go$' .; do
            sleep 1  # Debounce
            build_and_run
        done
    elif command -v fswatch &> /dev/null; then
        # macOS
        fswatch -o -e ".*" -i "\\.go$" . | while read num; do
            sleep 1  # Debounce
            build_and_run
        done
    else
        print_error "File watching requires 'inotifywait' (Linux) or 'fswatch' (macOS)"
        return 1
    fi
}

# Function to show help
show_help() {
    echo "Development script for job_scorer"
    echo ""
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  run         - Start application with hot reloading"
    echo "  test        - Run tests continuously"
    echo "  build       - Build application once"
    echo "  setup       - Set up development environment"
    echo "  help        - Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 run      # Start development server"
    echo "  $0 test     # Run tests in watch mode"
    echo ""
}

# Function to set up development environment
setup_dev() {
    print_section "Setting Up Development Environment"
    
    # Check Go installation
    if ! command -v go &> /dev/null; then
        print_error "Go is not installed. Please install Go first."
        return 1
    fi
    
    print_status "Go version: $(go version)"
    
    # Install Air
    install_air
    
    # Check for file watchers
    if ! command -v inotifywait &> /dev/null && ! command -v fswatch &> /dev/null; then
        print_warning "File watching tools not found."
        echo "For Linux (Ubuntu/Debian): sudo apt-get install inotify-tools"
        echo "For macOS: brew install fswatch"
    fi
    
    # Set up .env file
    check_env
    
    # Install dependencies
    print_status "Installing Go dependencies..."
    go mod download
    go mod tidy
    
    # Create necessary directories
    mkdir -p tmp coverage data
    
    print_status "Development environment setup complete!"
    print_status "Run './dev.sh run' to start the development server"
}

# Main script logic
case "${1:-help}" in
    "run")
        run_with_hot_reload
        ;;
    "test")
        run_tests_continuous
        ;;
    "build")
        print_section "Building Application"
        go build -o job-scorer .
        print_status "Build complete: job-scorer"
        ;;
    "setup")
        setup_dev
        ;;
    "help"|*)
        show_help
        ;;
esac 