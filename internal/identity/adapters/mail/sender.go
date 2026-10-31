package mailadapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	platformconfig "admin/internal/platform/config"
	goMail "github.com/wneessen/go-mail"
)

// Sender sends verification messages over the configured synchronous SMTP
// connection. It deliberately exposes no SMTP details to the application
// layer.
type Sender struct {
	client         *goMail.Client
	from           string
	timeout        time.Duration
	frontendOrigin string
}

func NewSender(config platformconfig.SMTPConfig, frontendOrigin string) (*Sender, error) {
	origin := strings.TrimRight(strings.TrimSpace(frontendOrigin), "/")
	if origin == "" {
		return nil, fmt.Errorf("verification frontend origin is required")
	}
	options := []goMail.Option{
		goMail.WithPort(config.Port),
		goMail.WithTimeout(time.Duration(config.TimeoutSeconds) * time.Second),
		goMail.WithUsername(config.Username),
		goMail.WithPassword(config.Password),
		goMail.WithSMTPAuth(goMail.SMTPAuthPlain),
	}
	switch config.TLSMode {
	case "disabled":
		options = append(options, goMail.WithTLSPolicy(goMail.NoTLS))
	case "starttls_required":
		options = append(options, goMail.WithTLSPolicy(goMail.TLSMandatory))
	case "implicit":
		options = append(options, goMail.WithSSL())
	default:
		return nil, fmt.Errorf("unsupported smtp tls mode")
	}
	client, err := goMail.NewClient(config.Host, options...)
	if err != nil {
		return nil, fmt.Errorf("create smtp client: %w", err)
	}
	return &Sender{client: client, from: config.From, timeout: time.Duration(config.TimeoutSeconds) * time.Second, frontendOrigin: origin}, nil
}

func (sender *Sender) SendVerification(ctx context.Context, recipient, token string) error {
	message := goMail.NewMsg()
	if err := message.From(sender.from); err != nil {
		return fmt.Errorf("set verification sender: %w", err)
	}
	if err := message.To(recipient); err != nil {
		return fmt.Errorf("set verification recipient: %w", err)
	}
	link := sender.frontendOrigin + "/profile#email_token=" + token
	message.Subject("验证你的邮箱")
	message.SetBodyString(goMail.TypeTextPlain, "请使用以下链接验证邮箱（15 分钟内有效）：\n"+link+"\n\n如果链接无法打开，请手动输入以下验证码：\n"+token+"\n\n如果不是你发起的操作，请忽略本邮件。")
	if sender.timeout <= 0 {
		return sender.client.DialAndSendWithContext(ctx, message)
	}
	sendContext, cancel := context.WithTimeout(ctx, sender.timeout)
	defer cancel()
	return sender.client.DialAndSendWithContext(sendContext, message)
}

var _ interface {
	SendVerification(context.Context, string, string) error
} = (*Sender)(nil)
