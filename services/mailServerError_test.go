package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A5: a mail the mail server refused gets a German explanation before the server's own text, which stays (it named
// the cause of the quota failures of 31.10.2026).
func TestMailServerError(t *testing.T) {
	for name, tc := range map[string]struct{ message, want string }{
		"quota": {"com.sun.mail.smtp.SMTPSendFailedException: 454 4.7.1 Quotas exceeded\n",
			"Der Mailserver hat sein Versandkontingent erreicht, die Mail wurde nicht verschickt. Bitte melde das dem " +
				"Support. (com.sun.mail.smtp.SMTPSendFailedException: 454 4.7.1 Quotas exceeded)"},
		"address": {"javax.mail.SendFailedException: Invalid Addresses;\n  nested exception is:\n\tcom.sun.mail.smtp." +
			"SMTPAddressFailedException: 501 5.1.3 The recipient address <x> is not a valid RFC-5322 address\n",
			"Der Mailserver hat die Empfänger-Adresse abgelehnt, die Mail wurde nicht verschickt. Bitte prüfe die " +
				"E-Mail-Adresse. (javax.mail.SendFailedException: Invalid Addresses; nested exception is: com.sun.mail." +
				"smtp.SMTPAddressFailedException: 501 5.1.3 The recipient address <x> is not a valid RFC-5322 address)"},
		"temporary": {"421 4.3.2 Service not available",
			"Der Mailserver hat die Mail vorübergehend abgelehnt, sie wurde nicht verschickt. (421 4.3.2 Service not available)"},
		"permanent": {"554 5.7.1 Message rejected",
			"Der Mailserver hat die Mail abgelehnt, sie wurde nicht verschickt. (554 5.7.1 Message rejected)"},
		"unknown": {"connection reset ", "connection reset"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, mailServerError(tc.message).Error())
		})
	}
}
