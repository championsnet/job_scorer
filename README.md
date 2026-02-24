# Job Scorer - Go Edition

An intelligent job search and evaluation system written in Go that finds promising jobs, evaluates them against your CV using AI, and sends email notifications for the best matches.

## 🚀 Features

- 🔍 **Automated Job Search**: Searches LinkedIn for jobs in Basel and Zurich every hour
- 🤖 **AI-Powered Initial Screening**: Uses GPT models to evaluate job titles and basic criteria
- 📄 **CV Matching**: Fetches full job descriptions and compares them against your CV
- 📧 **Smart Email Notifications**: Only sends emails for jobs that pass both screenings
- ⚡ **Concurrent Processing**: Efficient Go-based concurrent evaluation of multiple jobs
- 🎯 **Policy Driven**: Prompts, thresholds, filters, language rules, and email text are configurable in JSON
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
- OpenAI API key for LLM access
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

2. **Create configuration files:**
```bash
cp env.example .env
cp config/config.example.json config/config.json
```

3. **Configure your environment variables in `.env`:**
```env
# OpenAI API Configuration
OPENAI_API_KEY=your_openai_api_key_here
OPENAI_MODEL=gpt-5.2
POLICY_CONFIG_PATH=config/config.json

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
RUN_ON_STARTUP=true

# Rate Limiting
MAX_REQUESTS_PER_MINUTE=30
```

4. **Configure CV path in `config/config.json`**
```json
{
  "cv": {
    "path": "your_cv.pdf"
  }
}
```

5. **Place your CV file in the referenced location**

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

### Policy Configuration

All business-policy behavior is now in `config/config.json`, including:
- title/location/language filters and keyword lists
- LLM prompts and per-stage token budgets
- pipeline thresholds, batch sizes, and final validation rules
- CV parser order and fallback profile text
- email subject/body strings and notification gate rules
- scraper retries/backoff/pagination/delay constants

### Scheduling

Modify the schedule in `config/config.json`:
```json
{
  "app": {
    "cronSchedule": "0 */1 * * *"
  }
}
```

Set the scraping window in `config/config.json`:
```json
{
  "scraper": {
    "dateSincePosted": "past hour"
  }
}
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
4. **API Errors**: Check OpenAI API key and rate limits

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

### Adding New Job Sources

Implement the scraper interface in `services/scraper/` to add new job sources.

### Changing Email Templates

Edit the `notification` block in `config/config.json`.

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