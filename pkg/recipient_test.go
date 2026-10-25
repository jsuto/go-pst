package pst_test

import (
	"os"
	"strings"
	"testing"

	pst "github.com/jsuto/go-pst/pkg"
	"github.com/jsuto/go-pst/pkg/properties"
)

// TestRecipients checks that the recipient table matches PR_DISPLAY_TO, PR_DISPLAY_CC and PR_DISPLAY_BCC.
func TestRecipients(t *testing.T) {
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

	var messageCount, recipientCount int

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

			recipients, err := messageIterator.Value().GetRecipients()

			if err != nil {
				t.Fatalf("Failed to get recipients: %+v", err)
			}

			displayNames := map[pst.RecipientType][]string{}

			for _, recipient := range recipients {
				if recipient.Address() == "" {
					t.Errorf("%s: %q: recipient %q without address", folder.Name, messageProperties.GetSubject(), recipient.DisplayName)
				}

				displayNames[recipient.Type] = append(displayNames[recipient.Type], recipient.DisplayName)
			}

			for recipientType, displayProperty := range map[pst.RecipientType]string{
				pst.RecipientTypeTo:  messageProperties.GetDisplayTo(),
				pst.RecipientTypeCc:  messageProperties.GetDisplayCc(),
				pst.RecipientTypeBcc: messageProperties.GetDisplayBcc(),
			} {
				if got := strings.Join(displayNames[recipientType], "; "); got != displayProperty {
					t.Errorf("%s: %q: recipient type %d: got %q, want %q", folder.Name, messageProperties.GetSubject(), recipientType, got, displayProperty)
				}
			}

			messageCount++
			recipientCount += len(recipients)
		}

		return messageIterator.Err()
	}); err != nil {
		t.Fatalf("Failed to walk folders: %+v", err)
	}

	if messageCount != 2178 || recipientCount != 4029 {
		t.Errorf("got %d messages with %d recipients, want 2178 with 4029", messageCount, recipientCount)
	}
}
