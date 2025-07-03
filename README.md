# Job Scorer - Go Edition

An intelligent job search and evaluation system written in Go that finds promising jobs in Basel and Zurich areas, evaluates them against your CV using AI, and sends email notifications for the best matches.

## 🚀 Features

- 🔍 **Automated Job Search**: Searches LinkedIn for jobs in Basel and Zurich every hour
- 🤖 **AI-Powered Initial Screening**: Uses Groq LLM to evaluate job titles and basic criteria
- 📄 **CV Matching**: Fetches full job descriptions and compares them against your CV
- 📧 **Smart Email Notifications**: Only sends emails for jobs that pass both screenings
- ⚡ **Concurrent Processing**: Efficient Go-based concurrent evaluation of multiple jobs
- 🎯 **Customized Criteria**: Tailored for EU citizens with L permits seeking career growth
- 🚦 **Rate Limiting**: Intelligent rate limiting to respect API limits
- 📊 **Structured Logging**: Comprehensive logging with different log levels
- 🗂️ **Clean Architecture**: Well-organized codebase with clear separation of concerns

## 🏗️ Architecture

The application is built with a clean, modular architecture:

```
job-scorer/
├── main.go                          # Application entry point
├── config/                          # Configuration management
├── models/                          # Data models
├── services/                        # Business logic services
│   ├── scraper/                    # LinkedIn job scraping
│   ├── evaluator/                  # AI-powered job evaluation
│   ├── cv/                         # CV reading and processing
│   ├── filter/                     # Job filtering logic
│   ├── notification/               # Email notifications
│   └── storage/                    # File storage operations
├── controller/                      # Application orchestration
└── utils/                          # Shared utilities
```

## 📋 Prerequisites

- Go 1.21 or higher
- Groq API key for LLM access
- SMTP credentials for email notifications
- Your CV in PDF format

## 🛠️ Installation

1. **Clone and build:**
```bash
git clone <repository-url>
cd job-scorer
go mod download
go build -o job-scorer
```

2. **Create configuration file:**
```bash
cp env.example .env
```

3. **Configure your environment variables in `.env`:**
```env
# Groq API Configuration
GROQ_API_KEY=your_groq_api_key_here
GROQ_MODEL=gemma2-9b-it

# SMTP Configuration
SMTP_HOST=smtp.your-provider.com
SMTP_PORT=587
SMTP_SECURE=false
SMTP_USER=your_email@domain.com
SMTP_PASS=your_app_password
SMTP_FROM=your_email@domain.com
SMTP_TO=recipient@domain.com

# Application Configuration
JOB_LOCATIONS=90009885,90009888
CRON_SCHEDULE=0 */1 * * *
RUN_ON_STARTUP=true
CV_PATH=CV_Vasiliki Ploumistou_22_05.pdf

# Rate Limiting
MAX_REQUESTS_PER_MINUTE=30
```

4. **Place your CV file in the root directory**

## 🏃‍♂️ Usage

**Run the application:**
```bash
./job-scorer
```

The system will:
- Run immediately on startup (if `RUN_ON_STARTUP=true`)
- Schedule automatic runs based on `CRON_SCHEDULE`
- Save results to JSON files
- Send email notifications for recommended jobs

**Build for different platforms:**
```bash
# For Linux
GOOS=linux GOARCH=amd64 go build -o job-scorer-linux

# For Windows
GOOS=windows GOARCH=amd64 go build -o job-scorer.exe

# For macOS
GOOS=darwin GOARCH=amd64 go build -o job-scorer-mac
```

## ⚙️ Configuration

### Job Search Locations

Edit the `JOB_LOCATIONS` environment variable:
```env
JOB_LOCATIONS=90009885,90009888  # Basel, Zurich location IDs
```

### Evaluation Criteria

The system evaluates jobs based on:
- **Field Match**: Marketing, business development, operations, administration
- **Location**: Basel preferred, Zurich acceptable
- **Language**: English required, German >B1 is a blocker
- **Experience Level**: Entry to mid-level preferred
- **CV Match**: Skills, education, and experience alignment

### Scheduling

Modify the cron schedule:
```env
CRON_SCHEDULE=0 */1 * * *  # Every hour
# CRON_SCHEDULE=0 9 * * *   # Every day at 9 AM
# CRON_SCHEDULE=0 9 * * 1   # Every Monday at 9 AM
```

## 📁 Output Files

- `allJobs.json`: All scraped jobs
- `promisingJobs.json`: Jobs that passed initial screening (score ≥ 7)
- `finalEvaluatedJobs.json`: Jobs that passed CV evaluation

## 📧 Email Format

Emails include:
- Modern HTML design with job cards
- Initial and final evaluation scores
- CV-based evaluation reasoning
- Direct application links
- JSON attachment with full data

## 🔧 Development

**Run in development mode:**
```bash
go run main.go
```

**Run tests:**
```bash
go test ./...
```

**Format code:**
```bash
go fmt ./...
```

**Lint code:**
```bash
golangci-lint run
```

## 🚨 Troubleshooting

### Common Issues

1. **CV Loading Errors**: Ensure PDF file exists and is readable
2. **LinkedIn Blocking**: Randomized headers and delays prevent rate limiting
3. **Email Failures**: Verify SMTP credentials and connection
4. **API Errors**: Check Groq API key and rate limits

### Debug Logging

The application provides comprehensive logging. Check the console output for:
- Job fetching progress
- Evaluation scores and reasons
- Email sending status
- Error messages with context

### Rate Limiting

The application includes intelligent rate limiting:
- Default: 30 requests per minute
- Automatic backoff when limits are reached
- Configurable via `MAX_REQUESTS_PER_MINUTE`

## 🎨 Customization

### Adding New Evaluation Criteria

Edit the prompts in:
- `services/evaluator/evaluator.go`: Initial screening criteria
- `services/evaluator/evaluator.go`: CV matching criteria

### Adding New Job Sources

Implement the scraper interface in `services/scraper/` to add new job sources.

### Changing Email Templates

Modify the HTML template in `services/notification/email.go`.

## 🔒 Security & Legal

- Uses public LinkedIn job listings
- Respects rate limits and robots.txt
- No authentication bypass or scraping of private data
- Ensure compliance with LinkedIn's terms of service
- Follow your local regulations regarding automated data collection

## 📊 Performance

**Go vs Node.js Benefits:**
- **Memory Usage**: ~50-70% lower memory consumption
- **Startup Time**: ~3x faster startup
- **Concurrent Processing**: Better handling of multiple jobs
- **Binary Distribution**: Single executable, no dependencies
- **Rate Limiting**: More efficient implementation with Go channels

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## 📄 License

MIT License - See LICENSE file for details.

---

**Built with ❤️ in Go for efficient job searching and career growth!** 