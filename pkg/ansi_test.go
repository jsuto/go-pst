package pst_test

import (
	"os"
	"strings"
	"testing"

	pst "github.com/jsuto/go-pst/pkg"
	"github.com/jsuto/go-pst/pkg/properties"
)

// TestANSI reads data/32-bit.pst, an ANSI file with String8 properties (upstream #52).
func TestANSI(t *testing.T) {
	reader, err := os.Open("../data/32-bit.pst")

	if err != nil {
		t.Fatalf("Failed to open PST file: %+v", err)
	}

	defer reader.Close()

	pstFile, err := pst.New(reader)

	if err != nil {
		t.Fatalf("Failed to open PST file: %+v", err)
	} else if pstFile.FormatType != pst.FormatTypeANSI {
		t.Fatalf("got format type %d, want ANSI", pstFile.FormatType)
	}

	defer pstFile.Cleanup()

	var folderNames []string
	var messages []*pst.Message

	if err := pstFile.WalkFolders(func(folder *pst.Folder) error {
		folderNames = append(folderNames, folder.Name)

		messageIterator, err := folder.GetMessageIterator()

		if err != nil {
			return nil
		}

		for messageIterator.Next() {
			messages = append(messages, messageIterator.Value())
		}

		return messageIterator.Err()
	}); err != nil {
		t.Fatalf("Failed to walk folders: %+v", err)
	}

	if got := strings.Join(folderNames, ", "); !strings.Contains(got, "Calendar") {
		t.Errorf("got folders %s, want Calendar among them", got)
	}

	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(messages))
	}

	message := messages[0]

	if _, ok := message.Properties.(*properties.Appointment); !ok {
		t.Errorf("got %T, want *properties.Appointment", message.Properties)
	}

	if codePage := message.PropertyContext.GetCodePage(); codePage != 1252 {
		t.Errorf("got code page %d, want 1252", codePage)
	}

	var messageProperties properties.Message

	if err := message.PropertyContext.Populate(&messageProperties, message.LocalDescriptors); err != nil {
		t.Fatalf("Failed to populate message properties: %+v", err)
	} else if !strings.Contains(messageProperties.GetSubject(), "Olympus training") {
		t.Errorf("got subject %q", messageProperties.GetSubject())
	}

	sender, err := message.GetSender()

	if err != nil {
		t.Fatalf("Failed to get sender: %+v", err)
	} else if sender.DisplayName == "" || sender.Address() == "" {
		t.Errorf("got sender %+v", sender)
	}

	recipients, err := message.GetRecipients()

	if err != nil {
		t.Fatalf("Failed to get recipients: %+v", err)
	} else if len(recipients) != 7 {
		t.Errorf("got %d recipients, want 7", len(recipients))
	}
}
