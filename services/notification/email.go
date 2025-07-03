package notification

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"job-scorer/config"
	"job-scorer/models"
	"job-scorer/utils"

	"gopkg.in/gomail.v2"
)

type EmailNotifier struct {
	config *config.SMTPConfig
	logger *utils.Logger
}

func NewEmailNotifier(cfg *config.Config) *EmailNotifier {
	return &EmailNotifier{
		config: &cfg.SMTP,
		logger: utils.NewLogger("EmailNotifier"),
	}
}

func (e *EmailNotifier) SendJobNotification(jobs []*models.Job) error {
	if len(jobs) == 0 {
		e.logger.Info("No jobs to send notification for")
		return nil
	}

	e.logger.Info("Preparing to send notification for %d jobs to %d recipients", len(jobs), len(e.config.ToRecipients))

	// Verify SMTP connection first
	if err := e.verifySMTPConnection(); err != nil {
		return fmt.Errorf("SMTP verification failed: %w", err)
	}

	// Send to each recipient
	var lastError error
	successCount := 0

	for _, recipient := range e.config.ToRecipients {
		recipient = strings.TrimSpace(recipient)
		if recipient == "" {
			continue
		}

		e.logger.Info("Sending notification to: %s", recipient)
		
		if err := e.sendToRecipient(jobs, recipient); err != nil {
			e.logger.Error("Failed to send email to %s: %v", recipient, err)
			lastError = err
		} else {
			successCount++
			e.logger.Info("✅ Email notification sent successfully to %s", recipient)
		}
	}

	if successCount == 0 {
		return fmt.Errorf("failed to send email to any recipient: %w", lastError)
	}

	e.logger.Info("✅ Email notifications sent successfully to %d/%d recipients", successCount, len(e.config.ToRecipients))
	return nil
}

func (e *EmailNotifier) sendToRecipient(jobs []*models.Job, recipient string) error {
	// Create the email
	m := gomail.NewMessage()
	m.SetHeader("From", e.config.From)
	m.SetHeader("To", recipient)
	m.SetHeader("Subject", fmt.Sprintf("🎯 Job Alert: %d New Recommended Jobs Found!", len(jobs)))

	// Create HTML body
	htmlBody := e.createHTMLBody(jobs)
	m.SetBody("text/html", htmlBody)

	// Create JSON attachment
	jsonData, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		e.logger.Error("Error marshaling jobs to JSON: %v", err)
	} else {
		timestamp := time.Now().Format("2006-01-02_15-04-05")
		filename := fmt.Sprintf("recommended_jobs_%s.json", timestamp)
		m.Attach(filename, gomail.SetCopyFunc(func(w io.Writer) error {
			_, err := w.Write(jsonData)
			return err
		}))
	}

	// Create dialer
	d := gomail.NewDialer(e.config.Host, e.config.Port, e.config.User, e.config.Pass)
	d.TLSConfig = nil // Use STARTTLS

	// Send the email
	if err := d.DialAndSend(m); err != nil {
		e.logger.Error("Failed to send email to %s: %v", recipient, err)
		return fmt.Errorf("failed to send email to %s: %w", recipient, err)
	}

	return nil
}

func (e *EmailNotifier) verifySMTPConnection() error {
	e.logger.Info("Verifying SMTP connection...")

	d := gomail.NewDialer(e.config.Host, e.config.Port, e.config.User, e.config.Pass)
	d.TLSConfig = nil

	// Try to connect
	closer, err := d.Dial()
	if err != nil {
		e.logger.Error("SMTP connection failed: %v", err)
		return err
	}
	defer closer.Close()

	e.logger.Info("✅ SMTP connection verified")
	return nil
}

func (e *EmailNotifier) createHTMLBody(jobs []*models.Job) string {
	var html strings.Builder

	html.WriteString(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background-color: #f5f5f5; }
        .container { max-width: 800px; margin: 0 auto; background-color: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 20px; border-radius: 8px; margin-bottom: 20px; text-align: center; }
        .job-card { border: 1px solid #ddd; border-radius: 8px; margin: 15px 0; padding: 15px; background-color: #fafafa; }
        .job-title { font-size: 18px; font-weight: bold; color: #333; margin-bottom: 8px; }
        .company { font-size: 16px; color: #666; margin-bottom: 8px; }
        .location { font-size: 14px; color: #888; margin-bottom: 10px; }
        .scores { display: flex; gap: 15px; margin: 10px 0; }
        .score-badge { padding: 8px 15px; border-radius: 25px; font-weight: bold; font-size: 14px; }
        .match-score { background: linear-gradient(135deg, #4CAF50 0%, #45a049 100%); color: white; box-shadow: 0 2px 8px rgba(76, 175, 80, 0.3); }
        .description { font-size: 14px; color: #555; margin: 15px 0; padding: 15px; background-color: #f8f9fa; border-radius: 8px; line-height: 1.6; border-left: 4px solid #667eea; }
        .reason { font-size: 14px; color: #555; margin: 10px 0; padding: 10px; background-color: #f0f0f0; border-radius: 5px; line-height: 1.4; }
        .apply-btn { display: inline-block; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px; font-weight: bold; margin-top: 10px; }
        .apply-btn:hover { opacity: 0.9; }
        .footer { text-align: center; margin-top: 30px; color: #666; font-size: 12px; }
        .summary { background-color: #e8f5e8; padding: 15px; border-radius: 5px; margin-bottom: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🎯 New Job Recommendations</h1>
            <p>Your personalized job matches are ready!</p>
        </div>
        
        <div class="summary">
            <strong>📊 Summary:</strong> Found ` + strconv.Itoa(len(jobs)) + ` highly recommended jobs that match your CV and criteria.
        </div>
`)

	for _, job := range jobs {
		html.WriteString(fmt.Sprintf(`
        <div class="job-card">
            <div class="job-title">%s</div>
            <div class="company">🏢 %s</div>
            <div class="location">📍 %s</div>
            
            <div class="scores">`, 
			e.escapeHTML(job.Position), 
			e.escapeHTML(job.Company), 
			e.escapeHTML(job.Location)))

		if job.FinalScore != nil {
			html.WriteString(fmt.Sprintf(`
                <span class="score-badge match-score">Match: %.1f/10</span>`, *job.FinalScore))
		}

		html.WriteString(`</div>`)

		if job.JobDescription != "" {
			// Truncate description to 300 characters and add ellipsis if needed
			description := job.JobDescription
			if len(description) > 300 {
				description = description[:297] + "..."
			}
			html.WriteString(fmt.Sprintf(`
            <div class="description">
                <strong>📝 Job Description:</strong><br>%s
            </div>`, e.escapeHTML(description)))
		}

		if job.FinalReason != "" {
			html.WriteString(fmt.Sprintf(`
            <div class="reason">
                <strong>🎯 Why this matches your profile:</strong> %s
            </div>`, e.escapeHTML(job.FinalReason)))
		}

		if job.JobURL != "" {
			html.WriteString(fmt.Sprintf(`
            <a href="%s" class="apply-btn" target="_blank">🚀 Apply Now</a>`, job.JobURL))
		}

		html.WriteString(`</div>`)
	}

	html.WriteString(`
        <div class="personalized-message" style="background: linear-gradient(135deg, #ff9a9e 0%, #fecfef 50%, #fecfef 100%); padding: 20px; border-radius: 10px; margin: 30px 0; text-align: center; box-shadow: 0 4px 15px rgba(255, 154, 158, 0.3);">
            <div style="font-size: 18px; font-weight: bold; color: #d63384; margin-bottom: 10px; text-shadow: 1px 1px 2px rgba(0,0,0,0.1);">
                Good luck, my lovely Pepa! 🍀
            </div>
            <div style="font-size: 16px; color: #8b5a96; font-style: italic; font-family: 'Georgia', serif;">
                Η πεπίτσα όλα τα μπορεί 💙
            </div>
        </div>
        
        <div class="footer">
            <p>📧 This email was generated by your Job Scorer system</p>
            <p>🕒 Generated on ` + time.Now().Format("January 2, 2006 at 3:04 PM") + `</p>
            <p>💼 Keep building your career, one opportunity at a time!</p>
        </div>
    </div>
</body>
</html>`)

	return html.String()
}

func (e *EmailNotifier) escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

func (e *EmailNotifier) IsConfigured() bool {
	return e.config.Host != "" && e.config.User != "" && e.config.Pass != "" && e.config.From != "" && len(e.config.ToRecipients) > 0
} 