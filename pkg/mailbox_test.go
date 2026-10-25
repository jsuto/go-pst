package pst_test

import (
	"os"
	"strings"
	"testing"

	pst "github.com/jsuto/go-pst/pkg"
	"github.com/jsuto/go-pst/pkg/properties"
)

func TestMailboxAddress(t *testing.T) {
	x500 := "/O=FIRST ORGANIZATION/OU=EXCHANGE ADMINISTRATIVE GROUP(FYDIBOHF23SPDLT)/CN=RECIPIENTS/CN=0123456789ABCDEF"

	for _, test := range []struct {
		mailbox pst.Mailbox
		want    string
	}{
		{pst.Mailbox{AddressType: "EX", EmailAddress: x500, SMTPAddress: "alice@example.com"}, "alice@example.com"},
		{pst.Mailbox{AddressType: "SMTP", EmailAddress: "bob@example.com"}, "bob@example.com"},
		{pst.Mailbox{AddressType: "EX", EmailAddress: x500}, x500},
		{pst.Mailbox{DisplayName: "Nobody"}, ""},
	} {
		if got := test.mailbox.Address(); got != test.want {
			t.Errorf("%+v: got %q, want %q", test.mailbox, got, test.want)
		}
	}
}

// TestSender checks GetSender and GetSentRepresenting against the generated properties on data/enron.pst.
func TestSender(t *testing.T) {
	reader, err := os.Open("../data/enron.pst")

	if err != nil {
		t.Fatalf("Failed to open PST file: %+v", err)
	}

	defer reader.Close()

	pstFile, err := pst.New(reader)

	if err != nil {
		t.Fatalf("Failed to open PST file: %+v", err)
	}

	defer pstFile.Cleanup()

	var smtpSenderCount int

	if err := pstFile.WalkFolders(func(folder *pst.Folder) error {
		messageIterator, err := folder.GetMessageIterator()

		if err != nil {
			return nil
		}

		for messageIterator.Next() {
			messageProperties, ok := messageIterator.Value().Properties.(*properties.Message)

			if !ok {
				continue
			}

			sender, err := messageIterator.Value().GetSender()

			if err != nil {
				t.Fatalf("Failed to get sender: %+v", err)
			}

			sentRepresenting, err := messageIterator.Value().GetSentRepresenting()

			if err != nil {
				t.Fatalf("Failed to get sent representing: %+v", err)
			}

			if sender.DisplayName != messageProperties.GetSenderName() || sender.AddressType != messageProperties.GetSenderAddressType() ||
				sender.EmailAddress != messageProperties.GetSenderEmailAddress() {
				t.Errorf("%s: %q: sender %+v does not match the PR_SENDER_* properties", folder.Name, messageProperties.GetSubject(), sender)
			}

			if sentRepresenting.DisplayName != messageProperties.GetSentRepresentingName() || sentRepresenting.AddressType != messageProperties.GetSentRepresentingAddressType() ||
				sentRepresenting.EmailAddress != messageProperties.GetSentRepresentingEmailAddress() {
				t.Errorf("%s: %q: sent representing %+v does not match the PR_SENT_REPRESENTING_* properties", folder.Name, messageProperties.GetSubject(), sentRepresenting)
			}

			if strings.Contains(sender.Address(), "@") {
				smtpSenderCount++
			}
		}

		return messageIterator.Err()
	}); err != nil {
		t.Fatalf("Failed to walk folders: %+v", err)
	}

	if smtpSenderCount != 314 {
		t.Errorf("got %d messages with an SMTP sender, want 314", smtpSenderCount)
	}
}
