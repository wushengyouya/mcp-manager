package email

import (
	"gopkg.in/gomail.v2"
)

// Sender 定义邮件发送接口
type Sender interface {
	Send(from string, to []string, subject, body string) error
}

// SMTPSender 定义 SMTP 发送器
type SMTPSender struct {
	host     string
	port     int
	username string
	password string
	sendFunc func(*gomail.Message) error
}

// NewSMTPSender 创建 SMTP 发送器
func NewSMTPSender(host string, port int, username, password string) Sender {
	sender := &SMTPSender{host: host, port: port, username: username, password: password}
	sender.sendFunc = func(msg *gomail.Message) error {
		dialer := gomail.NewDialer(sender.host, sender.port, sender.username, sender.password)
		return dialer.DialAndSend(msg)
	}
	return sender
}

// Send 发送邮件
func (s *SMTPSender) Send(from string, to []string, subject, body string) error {
	msg := gomail.NewMessage()
	msg.SetHeader("From", from)
	msg.SetHeader("To", to...)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/plain", body)
	return s.sendFunc(msg)
}
