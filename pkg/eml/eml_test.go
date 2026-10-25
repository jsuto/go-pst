package eml_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/emersion/go-message"
	pst "github.com/jsuto/go-pst/pkg"
	"github.com/jsuto/go-pst/pkg/eml"
	"github.com/jsuto/go-pst/pkg/properties"
)

// openPST opens a test PST file, skipping the test if it doesn't exist.
func openPST(t *testing.T, path string) *pst.File {
	reader, err := os.Open(path)

	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("%s not found", path)
	} else if err != nil {
		t.Fatalf("Failed to open PST file: %+v", err)
	}

	t.Cleanup(func() { reader.Close() })

	pstFile, err := pst.New(reader)

	if err != nil {
		t.Fatalf("Failed to open PST file: %+v", err)
	}

	t.Cleanup(pstFile.Cleanup)

	return pstFile
}

// parsedPart is a leaf of a parsed message.
type parsedPart struct {
	mediaType   string
	disposition string
	body        []byte
}

// parseEML parses the converted message and returns its header and leaf parts (message/rfc822 parts are not entered).
func parseEML(t *testing.T, data []byte) (message.Header, []parsedPart) {
	if bytes.Contains(bytes.ReplaceAll(data, []byte("\r\n"), nil), []byte("\n")) {
		t.Errorf("bare LF in the message")
	}

	entity, err := message.Read(bytes.NewReader(data))

	if err != nil {
		t.Fatalf("Failed to parse the message: %v", err)
	}

	var parts []parsedPart

	var walk func(entity *message.Entity)

	walk = func(entity *message.Entity) {
		if multipartReader := entity.MultipartReader(); multipartReader != nil {
			for {
				part, err := multipartReader.NextPart()

				if err == io.EOF {
					return
				} else if err != nil {
					t.Fatalf("Failed to read part: %v", err)
				}

				walk(part)
			}
		}

		mediaType, _, _ := entity.Header.ContentType()
		disposition, _, _ := entity.Header.ContentDisposition()
		body, err := io.ReadAll(entity.Body)

		if err != nil {
			t.Fatalf("Failed to read the body of a %s part: %v", mediaType, err)
		}

		parts = append(parts, parsedPart{mediaType: mediaType, disposition: disposition, body: body})
	}

	walk(entity)

	return entity.Header, parts
}

func countAttachments(parts []parsedPart) int {
	var count int

	for _, part := range parts {
		if part.disposition == "attachment" || part.disposition == "inline" {
			count++
		}
	}

	return count
}

// TestConvertEnron converts data/enron.pst: all messages have transport headers and plain text bodies.
func TestConvertEnron(t *testing.T) {
	pstFile := openPST(t, "../../data/enron.pst")

	var convertedCount, attachmentCount int

	if err := eml.Walk(pstFile, func(item eml.Item) error {
		if item.Err != nil {
			t.Fatalf("%v: %+v", item.FolderPath, item.Err)
		}

		var output bytes.Buffer

		result, err := eml.Convert(&output, item.Message)

		if err != nil {
			t.Fatalf("%v: Failed to convert message %d: %+v", item.FolderPath, item.MessageIdentifier, err)
		} else if !result.HasTransportHeaders {
			t.Errorf("%v: message %d: headers were not taken from the transport headers", item.FolderPath, item.MessageIdentifier)
		}

		header, parts := parseEML(t, output.Bytes())

		var messageProperties properties.Message

		if err := item.Message.PropertyContext.Populate(&messageProperties, item.Message.LocalDescriptors); err != nil {
			t.Fatal(err)
		}

		if messageID := messageProperties.GetInternetMessageId(); messageID != "" && strings.TrimSpace(header.Get("Message-Id")) != messageID {
			t.Errorf("message %d: got Message-ID %q, want %q", item.MessageIdentifier, header.Get("Message-Id"), messageID)
		}

		if parts[0].mediaType != "text/plain" || normalizeLineEndings(string(parts[0].body)) != normalizeLineEndings(messageProperties.GetBody()) {
			t.Errorf("message %d: the first part (%s) is not PR_BODY", item.MessageIdentifier, parts[0].mediaType)
		}

		attachments, err := item.Message.GetAllAttachments()

		if err != nil {
			t.Fatal(err)
		} else if got := countAttachments(parts); got != len(attachments) || result.Attachments != len(attachments) {
			t.Errorf("message %d: got %d attachments (result %d), want %d", item.MessageIdentifier, got, result.Attachments, len(attachments))
		}

		convertedCount++
		attachmentCount += result.Attachments

		return nil
	}); err != nil {
		t.Fatalf("Failed to walk: %+v", err)
	}

	if convertedCount != 2178 || attachmentCount != 168 {
		t.Errorf("got %d messages with %d attachments, want 2178 with 168", convertedCount, attachmentCount)
	}
}

func normalizeLineEndings(text string) string {
	return strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

// TestConvertNonMail checks that the appointment of data/32-bit.pst is skipped.
func TestConvertNonMail(t *testing.T) {
	pstFile := openPST(t, "../../data/32-bit.pst")

	var skipped int

	if err := eml.Walk(pstFile, func(item eml.Item) error {
		if item.Err != nil {
			t.Fatalf("%v: %+v", item.FolderPath, item.Err)
		}

		var output bytes.Buffer

		_, err := eml.Convert(&output, item.Message)

		var skipError *eml.SkipError

		if !errors.As(err, &skipError) || skipError.Kind != eml.KindNonMail || output.Len() != 0 {
			t.Errorf("got %v and %d bytes, want a non-mail SkipError", err, output.Len())
		}

		skipped++

		return nil
	}); err != nil {
		t.Fatalf("Failed to walk: %+v", err)
	}

	if skipped != 1 {
		t.Errorf("got %d items, want 1", skipped)
	}
}

// TestConvertPrivate converts the private test PST (testdata/private, not in git): sent mail without transport
// headers, Bcc, an RTF-only body, attachments and forwarded messages. Only counts are checked, no content.
func TestConvertPrivate(t *testing.T) {
	pstFile := openPST(t, "../../testdata/private/backup.pst")

	var converted, skipped, embedded, bcc, builtHeaders int

	if err := eml.Walk(pstFile, func(item eml.Item) error {
		if item.Err != nil {
			t.Fatalf("%v: %+v", item.FolderPath, item.Err)
		}

		var output bytes.Buffer

		result, err := eml.Convert(&output, item.Message)

		var skipError *eml.SkipError

		if errors.As(err, &skipError) {
			skipped++
			return nil
		} else if err != nil {
			t.Fatalf("%v: %+v", item.FolderPath, err)
		}

		converted++

		header, parts := parseEML(t, output.Bytes())

		for _, part := range parts {
			if part.mediaType == "message/rfc822" {
				embedded++

				if _, inner := parseEML(t, part.body); len(inner) == 0 {
					t.Errorf("%v: empty embedded message", item.FolderPath)
				}
			}
		}

		if header.Has("Bcc") {
			bcc++
		}

		if !result.HasTransportHeaders {
			builtHeaders++

			if header.Has("Received") || !header.Has("Date") || !header.Has("Message-Id") {
				t.Errorf("%v: built header without Date or Message-ID, or with Received", item.FolderPath)
			}

			// Except for the draft, mail without transport headers was sent: it has a sender and recipients.
			if strings.Join(item.FolderPath, "/") != "Drafts" && (!strings.Contains(header.Get("From"), "@") || !header.Has("To")) {
				t.Errorf("%v: built header without SMTP From or To", item.FolderPath)
			}
		}

		return nil
	}); err != nil {
		t.Fatalf("Failed to walk: %+v", err)
	}

	if converted != 24 || skipped != 93 || embedded != 2 || bcc != 1 || builtHeaders != 12 {
		t.Errorf("got %d converted, %d skipped, %d embedded, %d with Bcc, %d with built headers, want 24, 93, 2, 1, 12",
			converted, skipped, embedded, bcc, builtHeaders)
	}
}
