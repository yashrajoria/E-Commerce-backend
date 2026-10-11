package sender

import (
	"context"
	"testing"
)

func TestSendEmail_RejectsHeaderInjection(t *testing.T) {
	s := &SMTPSender{}
	for _, c := range [][2]string{{"a@x.com\r\nBcc: v@evil.com", "hi"}, {"a@x.com", "hi\nBcc: v@evil.com"}} {
		if _, err := s.SendEmail(context.Background(), c[0], c[1], "body"); err == nil {
			t.Fatalf("expected rejection for to=%q subject=%q", c[0], c[1])
		}
	}
}
