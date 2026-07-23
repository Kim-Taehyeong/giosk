package notify

import (
	"fmt"
	"net/smtp"
	"strings"
)

// SMTPMailer는 net/smtp 기반 이메일 발송(587 STARTTLS/25 평문 자동). Helm smtp.* 로 주입.
type SMTPMailer struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// Send는 to 수신자들에게 plain-text 메일 1통을 보낸다.
func (m *SMTPMailer) Send(to []string, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", m.Host, m.Port)
	var auth smtp.Auth
	if m.Username != "" {
		auth = smtp.PlainAuth("", m.Username, m.Password, m.Host)
	}
	from := m.From
	if from == "" {
		from = m.Username
	}
	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n",
		from, strings.Join(to, ", "), subject, body))
	return smtp.SendMail(addr, auth, from, to, msg)
}
