package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/smtp"
	"time"

	log "github.com/sirupsen/logrus"
)

// DigestSender handles sending digest emails
type DigestSender struct {
	config Config
}

// NewDigestSender creates a new digest sender
func NewDigestSender(config Config) *DigestSender {
	return &DigestSender{config: config}
}

// Send sends the digest email
func (d *DigestSender) Send(active []Problem, resolved []Problem, stats *ProblemStats) error {
	if d.config.SMTPUser == "" || d.config.SMTPPassword == "" {
		return fmt.Errorf("SMTP credentials not configured")
	}

	// Generate email content
	subject := fmt.Sprintf("Homelab Log Anomaly Digest - %s", time.Now().Format("2006-01-02"))
	body, err := d.generateBody(active, resolved, stats)
	if err != nil {
		return fmt.Errorf("failed to generate email body: %w", err)
	}

	// Build message
	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"\r\n"+
		"%s",
		d.config.SMTPFrom,
		d.config.DigestRecipient,
		subject,
		body,
	)

	// Send email
	auth := smtp.PlainAuth("", d.config.SMTPUser, d.config.SMTPPassword, d.config.SMTPHost)
	addr := fmt.Sprintf("%s:%d", d.config.SMTPHost, d.config.SMTPPort)

	err = smtp.SendMail(addr, auth, d.config.SMTPFrom, []string{d.config.DigestRecipient}, []byte(msg))
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.WithFields(log.Fields{
		"recipient":       d.config.DigestRecipient,
		"active_problems": len(active),
		"resolved_today":  len(resolved),
	}).Info("Digest email sent")

	return nil
}

func (d *DigestSender) generateBody(active []Problem, resolved []Problem, stats *ProblemStats) (string, error) {
	tmpl := template.Must(template.New("digest").Funcs(template.FuncMap{
		"severityColor": severityColor,
		"severityEmoji": severityEmoji,
	}).Parse(digestTemplate))

	data := struct {
		Date         string
		Active       []Problem
		Resolved     []Problem
		Stats        *ProblemStats
		GrafanaURL   string
	}{
		Date:       time.Now().Format("Monday, January 2, 2006"),
		Active:     active,
		Resolved:   resolved,
		Stats:      stats,
		GrafanaURL: "http://192.168.1.143:8910",
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func severityColor(severity string) string {
	switch severity {
	case "critical":
		return "#dc3545"
	case "high":
		return "#fd7e14"
	case "medium":
		return "#ffc107"
	case "low":
		return "#28a745"
	default:
		return "#6c757d"
	}
}

func severityEmoji(severity string) string {
	switch severity {
	case "critical":
		return "🔴"
	case "high":
		return "🟠"
	case "medium":
		return "🟡"
	case "low":
		return "🟢"
	default:
		return "⚪"
	}
}

const digestTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
        .container { max-width: 800px; margin: 0 auto; background: white; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; }
        .header h1 { margin: 0; font-size: 24px; }
        .header p { margin: 10px 0 0; opacity: 0.9; }
        .stats { display: flex; gap: 20px; padding: 20px; background: #f8f9fa; border-bottom: 1px solid #e9ecef; }
        .stat { text-align: center; flex: 1; }
        .stat-value { font-size: 32px; font-weight: bold; color: #333; }
        .stat-label { font-size: 12px; color: #666; text-transform: uppercase; }
        .section { padding: 20px; }
        .section-title { font-size: 18px; font-weight: 600; color: #333; margin: 0 0 15px; border-bottom: 2px solid #667eea; padding-bottom: 10px; }
        .problem { background: #f8f9fa; border-radius: 6px; padding: 15px; margin-bottom: 10px; border-left: 4px solid; }
        .problem-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
        .problem-title { font-weight: 600; color: #333; }
        .problem-meta { font-size: 12px; color: #666; }
        .problem-duration { background: #e9ecef; padding: 2px 8px; border-radius: 12px; font-size: 11px; }
        .badge { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 600; color: white; text-transform: uppercase; }
        .empty { text-align: center; color: #666; padding: 30px; }
        .footer { padding: 20px; text-align: center; font-size: 12px; color: #666; border-top: 1px solid #e9ecef; }
        a { color: #667eea; text-decoration: none; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🏠 Homelab Anomaly Digest</h1>
            <p>{{.Date}}</p>
        </div>
        
        {{if .Stats}}
        <div class="stats">
            <div class="stat">
                <div class="stat-value">{{.Stats.ActiveCount}}</div>
                <div class="stat-label">Active Problems</div>
            </div>
            <div class="stat">
                <div class="stat-value">{{.Stats.NewToday}}</div>
                <div class="stat-label">New Today</div>
            </div>
            <div class="stat">
                <div class="stat-value">{{.Stats.ResolvedToday}}</div>
                <div class="stat-label">Resolved Today</div>
            </div>
        </div>
        {{end}}
        
        <div class="section">
            <h2 class="section-title">🔔 Active Problems</h2>
            {{if .Active}}
            {{range .Active}}
            <div class="problem" style="border-color: {{severityColor .Severity}}">
                <div class="problem-header">
                    <span class="problem-title">{{severityEmoji .Severity}} {{.Title}}</span>
                    <span class="badge" style="background: {{severityColor .Severity}}">{{.Severity}}</span>
                </div>
                <div class="problem-meta">
                    <span class="problem-duration">⏱️ {{.DurationString}}</span>
                    &nbsp;•&nbsp; {{.OccurrenceCount}} occurrences
                    {{if .AffectedHosts}}&nbsp;•&nbsp; Hosts: {{range $i, $h := .AffectedHosts}}{{if $i}}, {{end}}{{$h}}{{end}}{{end}}
                </div>
                {{if .LLMAnalysis}}
                <div style="margin-top: 8px; font-size: 13px; color: #555;">💡 {{.LLMAnalysis}}</div>
                {{end}}
            </div>
            {{end}}
            {{else}}
            <div class="empty">✨ No active problems! All systems nominal.</div>
            {{end}}
        </div>
        
        {{if .Resolved}}
        <div class="section">
            <h2 class="section-title">✅ Resolved Today</h2>
            {{range .Resolved}}
            <div class="problem" style="border-color: #28a745">
                <div class="problem-header">
                    <span class="problem-title">{{.Title}}</span>
                    <span class="badge" style="background: #28a745">resolved</span>
                </div>
                <div class="problem-meta">
                    Duration: {{.DurationString}} &nbsp;•&nbsp; {{.OccurrenceCount}} total occurrences
                </div>
            </div>
            {{end}}
        </div>
        {{end}}
        
        <div class="footer">
            <a href="{{.GrafanaURL}}/d/log-anomalies">View in Grafana</a> &nbsp;•&nbsp;
            Generated by log-anomaly-ml-processor
        </div>
    </div>
</body>
</html>`
