# Testing and Development Guide

This guide covers all testing scenarios and development workflows for the job_scorer application.

## Quick Start

### Run All Tests
```bash
./test.sh
```

### Start Development Server with Hot Reload
```bash
./dev.sh setup   # First time setup
./dev.sh run     # Start development server
```

### Run Tests in Watch Mode
```bash
./dev.sh test
```

## Test Coverage

Our comprehensive test suite covers:

### 1. Models (`models/job_test.go`)
- ✅ Job creation and validation
- ✅ Job scoring and promising threshold logic
- ✅ Job notification criteria
- ✅ Error handling for invalid data

### 2. Configuration (`config/config_test.go`)
- ✅ Environment variable loading
- ✅ Default value handling
- ✅ Type conversion (int, bool)
- ✅ Configuration validation

### 3. Utilities (`utils/`)
- **Logger** (`utils/logger_test.go`)
  - ✅ Service-specific logging
  - ✅ Log level formatting
  - ✅ Concurrent logging safety
  - ✅ Output redirection
  
- **Rate Limiter** (`utils/ratelimiter_test.go`)
  - ✅ Request rate limiting
  - ✅ Time window management
  - ✅ Context cancellation
  - ✅ Concurrent request handling
  - ✅ Graceful shutdown

### 4. Services (`services/`)
- **Storage** (`services/storage/storage_test.go`)
  - ✅ JSON file operations
  - ✅ Save/load job data
  - ✅ Error handling for invalid files
  - ✅ Round-trip data integrity
  
- **Filter** (`services/filter/filter_test.go`)
  - ✅ Date-based job filtering
  - ✅ German language detection
  - ✅ Recent job identification
  - ✅ Integration pipeline testing
  
- **Evaluator** (`services/evaluator/evaluator_test.go`)
  - ✅ Mock Groq API client
  - ✅ Job evaluation scoring
  - ✅ Concurrent evaluation handling
  - ✅ Context cancellation
  - ✅ Error recovery

### 5. Controller (`controller/job_controller_test.go`)
- ✅ Complete pipeline orchestration
- ✅ Error propagation and handling
- ✅ Service interaction verification
- ✅ Context cancellation behavior
- ✅ Edge case scenarios

## Test Scenarios

### Unit Tests
Each component is tested in isolation using mocks:
```bash
go test ./models -v
go test ./config -v
go test ./utils -v
go test ./services/storage -v
go test ./services/filter -v
go test ./services/evaluator -v
go test ./controller -v
```

### Integration Tests
Test complete workflows with realistic data:
```bash
go test -tags=integration ./...
```

### Race Condition Tests
Verify concurrent safety:
```bash
go test -race ./...
```

### Coverage Reports
Generate detailed coverage reports:
```bash
./test.sh  # Automatically generates coverage.html
open coverage.html  # View in browser
```

## Development Workflow

### 1. Setup Development Environment
```bash
./dev.sh setup
```
This will:
- Install Air for hot reloading
- Check for file watchers (inotifywait/fswatch)
- Create .env from template
- Install Go dependencies
- Create necessary directories

### 2. Start Development Server
```bash
./dev.sh run
```
Features:
- 🔥 Hot reloading on file changes
- 📁 Automatic rebuilding
- 🔄 Process restart on changes
- 📝 Build error logging

### 3. Continuous Testing
```bash
./dev.sh test
```
Features:
- 🔍 Watch for .go file changes
- ⚡ Instant test execution
- 📊 Clear pass/fail reporting
- 🔄 Automatic re-runs

### 4. Manual Testing
```bash
./dev.sh build    # Build once
./job-scorer      # Run manually
```

## Test Data and Mocking

### Mock Services
All external dependencies are mocked for testing:

#### MockGroqClient
```go
type MockGroqClient struct {
    responses map[string]string
    errors    map[string]error
    callCount int
}
```

#### MockScraper
```go
type MockScraper struct {
    jobs  []*models.Job
    err   error
    calls int
}
```

### Test Data Creation
Helper functions for creating test data:
```go
func floatPtr(f float64) *float64 {
    return &f
}

// Create test job
job, err := models.NewJob(
    "Software Engineer",
    "Tech Corp", 
    "Basel",
    "2024-01-01",
    "80000 CHF",
    "https://linkedin.com/jobs/123",
    "https://logo.png",
    "2 hours ago",
)
```

## Error Testing

### Network Failures
- Groq API timeouts and errors
- LinkedIn scraping failures
- Rate limiting scenarios

### File System Issues
- CV file not found
- Permission errors
- Disk space issues

### Data Validation
- Invalid job data
- Malformed JSON
- Missing required fields

### Concurrency Issues
- Race conditions
- Deadlocks
- Resource contention

## Performance Testing

### Benchmarks
```bash
go test -bench=. -benchmem ./...
```

### Memory Profiling
```bash
go test -memprofile=mem.prof ./...
go tool pprof mem.prof
```

### CPU Profiling
```bash
go test -cpuprofile=cpu.prof ./...
go tool pprof cpu.prof
```

## Debugging

### Enable Debug Logging
```bash
export LOG_LEVEL=debug
./job-scorer
```

### Race Detection
```bash
go run -race .
```

### Memory Leak Detection
```bash
go test -race -count=100 ./...
```

## Continuous Integration

### Local CI Simulation
```bash
./test.sh  # Runs full CI pipeline locally
```

The test script includes:
- ✅ Linting (golangci-lint or go vet)
- ✅ Code formatting check
- ✅ Unit tests with coverage
- ✅ Integration tests
- ✅ Race condition detection
- ✅ Build verification
- ✅ Security checks

### CI Requirements
- Go 1.19+
- All tests must pass
- Minimum 80% test coverage
- No race conditions
- No linting errors

## Troubleshooting

### Common Issues

#### Tests Fail with "permission denied"
```bash
chmod +x test.sh dev.sh
```

#### File watcher not working
**Linux:**
```bash
sudo apt-get install inotify-tools
```

**macOS:**
```bash
brew install fswatch
```

#### Air not found
```bash
go install github.com/cosmtrek/air@latest
```

#### Coverage reports empty
Ensure you're running tests from the project root:
```bash
cd /path/to/job_scorer
./test.sh
```

### Debug Test Failures

#### Verbose Test Output
```bash
go test -v ./... | tee test_output.log
```

#### Single Package Testing
```bash
go test -v ./models
```

#### Specific Test Function
```bash
go test -v -run TestJobValidate ./models
```

#### Test with Debug Output
```bash
go test -v -args -debug ./...
```

## Test Maintenance

### Adding New Tests
1. Create `*_test.go` files alongside source code
2. Follow existing patterns for mocking
3. Test both success and error cases
4. Include edge cases and boundary conditions
5. Update this documentation

### Mock Maintenance
- Keep mocks simple and focused
- Update mocks when interfaces change
- Verify mock behavior matches real implementation
- Use interface-based mocking for better testability

### Test Data
- Use realistic but anonymized test data
- Create helper functions for common test scenarios
- Keep test data minimal but representative
- Document any special test data requirements

## Best Practices

### Test Structure
```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name     string
        input    InputType
        want     OutputType
        wantErr  bool
    }{
        // Test cases
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Error Testing
```go
if tt.wantErr {
    if err == nil {
        t.Errorf("Expected error but got none")
    }
    return
}

if err != nil {
    t.Errorf("Unexpected error: %v", err)
    return
}
```

### Assertion Patterns
```go
if got != want {
    t.Errorf("Function() = %v, want %v", got, want)
}
```

### Cleanup
```go
defer func() {
    // Cleanup code
    os.RemoveAll(tmpDir)
}()
```

## Conclusion

This comprehensive testing suite ensures the job_scorer application is reliable, maintainable, and performs well under various conditions. The development workflow supports rapid iteration while maintaining code quality.

For questions or issues, refer to the troubleshooting section or review the test implementation for examples. 