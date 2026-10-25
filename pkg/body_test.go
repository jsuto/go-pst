package pst_test

import (
	"bytes"
	"errors"
	"os"
	"regexp"
	"testing"

	pst "github.com/jsuto/go-pst/pkg"
	"github.com/jsuto/go-pst/pkg/properties"
)

func TestGetCharsetName(t *testing.T) {
	for codePage, want := range map[int]string{
		65001: "utf-8", 1252: "windows-1252", 28591: "iso-8859-1", 28592: "iso-8859-2", 20127: "us-ascii",
		932: "shift_jis", 50220: "iso-2022-jp", 12345: "",
	} {
		if got := pst.GetCharsetName(codePage); got != want {
			t.Errorf("%d: got %q, want %q", codePage, got, want)
		}
	}
}

// walkMessages calls the function for each message of the PST file.
func walkMessages(t *testing.T, path string, function func(folder *pst.Folder, message *pst.Message)) {
	reader, err := os.Open(path)

	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("%s not found", path)
	} else if err != nil {
		t.Fatalf("Failed to open PST file: %+v", err)
	}

	defer reader.Close()

	pstFile, err := pst.New(reader)

	if err != nil {
		t.Fatalf("Failed to open PST file: %+v", err)
	}

	defer pstFile.Cleanup()

	if err := pstFile.WalkFolders(func(folder *pst.Folder) error {
		messageIterator, err := folder.GetMessageIterator()

		if err != nil {
			return nil
		}

		for messageIterator.Next() {
			function(folder, messageIterator.Value())
		}

		return messageIterator.Err()
	}); err != nil {
		t.Fatalf("Failed to walk folders: %+v", err)
	}
}

// TestBodyText checks that the text bodies of data/enron.pst match the generated PR_BODY field.
func TestBodyText(t *testing.T) {
	var messageCount int

	walkMessages(t, "../data/enron.pst", func(folder *pst.Folder, message *pst.Message) {
		messageProperties, ok := message.Properties.(*properties.Message)

		if !ok {
			return
		}

		body, err := message.GetBody()

		if err != nil {
			t.Fatalf("Failed to get body: %+v", err)
		} else if body.Text == "" || body.Text != messageProperties.GetBody() || len(body.HTML) != 0 || body.FromRTF {
			t.Errorf("%s: %q: unexpected body (text %d bytes, HTML %d bytes)", folder.Name, messageProperties.GetSubject(), len(body.Text), len(body.HTML))
		}

		messageCount++
	})

	if messageCount != 2178 {
		t.Errorf("got %d messages, want 2178", messageCount)
	}
}

var metaCharset = regexp.MustCompile(`(?i)<meta[^>]*charset=["']?([a-z0-9_-]+)`)

// TestBodyPrivate checks the bodies of the private test PST (see privateTestPST):
// HTML in the code page of PR_INTERNET_CPID, one plain text body, one RTF-only body and one empty body.
func TestBodyPrivate(t *testing.T) {
	var htmlCount, textCount, rtfCount, emptyCount, embeddedCount int

	walkMessages(t, privateTestPST(t), func(folder *pst.Folder, message *pst.Message) {
		if _, ok := message.Properties.(*properties.Message); !ok {
			return
		}

		body, err := message.GetBody()

		if err != nil {
			t.Fatalf("%s: Failed to get body: %+v", folder.Name, err)
		}

		switch {
		case len(body.HTML) > 0:
			htmlCount++

			if match := metaCharset.FindSubmatch(body.HTML); match == nil || !bytes.EqualFold(match[1], []byte(pst.GetCharsetName(body.HTMLCodePage))) {
				t.Errorf("%s: HTML code page %d does not match the meta charset", folder.Name, body.HTMLCodePage)
			}
		case body.FromRTF:
			rtfCount++
		case body.Text != "":
			textCount++
		default:
			emptyCount++
		}

		attachments, err := message.GetAllAttachments()

		if err != nil {
			t.Fatalf("%s: Failed to get attachments: %+v", folder.Name, err)
		}

		for _, attachment := range attachments {
			if !attachment.IsEmbeddedMessage() {
				continue
			}

			embeddedMessage, err := attachment.GetEmbeddedMessage()

			if err != nil {
				t.Fatalf("%s: Failed to get embedded message: %+v", folder.Name, err)
			}

			embeddedBody, err := embeddedMessage.GetBody()

			if err != nil {
				t.Fatalf("%s: Failed to get embedded message body: %+v", folder.Name, err)
			} else if len(embeddedBody.HTML) == 0 && embeddedBody.Text == "" {
				t.Errorf("%s: embedded message without body", folder.Name)
			}

			embeddedCount++
		}
	})

	if htmlCount != 19 || textCount != 1 || rtfCount != 1 || emptyCount != 1 || embeddedCount != 2 {
		t.Errorf("got %d HTML, %d text, %d RTF, %d empty bodies and %d embedded messages, want 19, 1, 1, 1 and 2",
			htmlCount, textCount, rtfCount, emptyCount, embeddedCount)
	}
}
