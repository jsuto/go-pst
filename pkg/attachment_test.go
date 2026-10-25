package pst_test

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	pst "github.com/jsuto/go-pst/pkg"
	"github.com/jsuto/go-pst/pkg/properties"
)

// TestEmbeddedMessages reads the messages forwarded as attachment in the private test PST (testdata/private, not in git).
func TestEmbeddedMessages(t *testing.T) {
	reader, err := os.Open("../testdata/private/backup.pst")

	if errors.Is(err, os.ErrNotExist) {
		t.Skip("testdata/private/backup.pst not found")
	} else if err != nil {
		t.Fatalf("Failed to open PST file: %+v", err)
	}

	defer reader.Close()

	pstFile, err := pst.New(reader)

	if err != nil {
		t.Fatalf("Failed to open PST file: %+v", err)
	}

	defer pstFile.Cleanup()

	var embeddedMessageCount int

	if err := pstFile.WalkFolders(func(folder *pst.Folder) error {
		messageIterator, err := folder.GetMessageIterator()

		if err != nil {
			return nil
		}

		for messageIterator.Next() {
			attachments, err := messageIterator.Value().GetAllAttachments()

			if err != nil {
				return err
			}

			for _, attachment := range attachments {
				if !attachment.IsEmbeddedMessage() {
					if _, err := attachment.GetEmbeddedMessage(); !errors.Is(err, pst.ErrAttachmentNotEmbeddedMessage) {
						t.Errorf("%s: got %v, want %v", folder.Name, err, pst.ErrAttachmentNotEmbeddedMessage)
					}

					continue
				}

				embeddedMessageCount++

				if _, err := attachment.WriteTo(io.Discard); !errors.Is(err, pst.ErrAttachmentEmbeddedMessage) {
					t.Errorf("%s: WriteTo: got %v, want %v", folder.Name, err, pst.ErrAttachmentEmbeddedMessage)
				}

				embeddedMessage, err := attachment.GetEmbeddedMessage()

				if err != nil {
					t.Fatalf("%s: Failed to get embedded message: %+v", folder.Name, err)
				}

				embeddedMessageProperties, ok := embeddedMessage.Properties.(*properties.Message)

				if !ok {
					t.Fatalf("%s: got %T, want *properties.Message", folder.Name, embeddedMessage.Properties)
				}

				// Both test messages were received by mail before being forwarded as attachment.
				if embeddedMessageProperties.GetSubject() == "" || embeddedMessageProperties.GetTransportMessageHeaders() == "" {
					t.Errorf("%s: embedded message without subject or transport headers", folder.Name)
				}

				sentRepresenting, err := embeddedMessage.GetSentRepresenting()

				if err != nil {
					t.Errorf("%s: Failed to get embedded message sender: %+v", folder.Name, err)
				} else if !strings.Contains(sentRepresenting.Address(), "@") {
					t.Errorf("%s: embedded message without SMTP sender", folder.Name)
				}

				recipients, err := embeddedMessage.GetRecipients()

				if err != nil {
					t.Errorf("%s: Failed to get embedded message recipients: %+v", folder.Name, err)
				} else if len(recipients) == 0 {
					t.Errorf("%s: embedded message without recipients", folder.Name)
				}
			}
		}

		return messageIterator.Err()
	}); err != nil {
		t.Fatalf("Failed to walk folders: %+v", err)
	}

	if embeddedMessageCount != 2 {
		t.Errorf("got %d embedded messages, want 2", embeddedMessageCount)
	}
}
