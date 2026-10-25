package eml

import (
	"bytes"
	"strings"
	"testing"

	pst "github.com/jsuto/go-pst/pkg"
)

func TestClassify(t *testing.T) {
	for messageClass, want := range map[string]Kind{
		"":                                    KindMail,
		"IPM.Note":                            KindMail,
		"ipm.note":                            KindMail,
		"IPM.Note.SMIME.MultipartSigned":      KindMail,
		"IPM.Schedule.Meeting.Request":        KindMail,
		"IPM.Schedule.Meeting.Resp.Pos":       KindMail,
		"REPORT.IPM.Note.NDR":                 KindMail,
		"IPM.Post":                            KindMail,
		"IPM.Notes":                           KindNonMail,
		"IPM.Appointment":                     KindNonMail,
		"IPM.Contact":                         KindNonMail,
		"IPM.Task":                            KindNonMail,
		"IPM.StickyNote":                      KindNonMail,
		"IPM.Note.EnterpriseVault.Shortcut":   KindShortcut,
		"IPM.Note.EnterpriseVault.Shortcut.X": KindShortcut,
		"IPM.Note.EnterpriseVault.PendingArchive": KindMail,
	} {
		if got := Classify(messageClass); got != want {
			t.Errorf("%q: got %s, want %s", messageClass, got, want)
		}
	}
}

func TestGetTransportHeaderLines(t *testing.T) {
	transportHeaders := "Received: from a\r\n\tby b; Mon, 26 Nov 2001 14:27:13 -0600\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed;\r\n\tboundary=\"x\"\r\n" +
		"Microsoft Mail Internet Headers Version 2.0\r\n" +
		"Subject:  FW: assignment\r\n" +
		"content-transfer-encoding: 7bit\r\n" +
		"X-Odd: value\nTo: \"A\" <a@example.com>,\r\n  \"B\" <b@example.com>\r\n\r\n" +
		"Body: not a header\r\n"

	want := []string{
		"Received: from a", "\tby b; Mon, 26 Nov 2001 14:27:13 -0600",
		"Subject:  FW: assignment",
		"X-Odd: value", "To: \"A\" <a@example.com>,", "  \"B\" <b@example.com>",
	}

	if got := getTransportHeaderLines(transportHeaders); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestFormatAddress(t *testing.T) {
	x500 := "/O=FIRST ORGANIZATION/OU=EXCHANGE ADMINISTRATIVE GROUP(FYDIBOHF23SPDLT)/CN=RECIPIENTS/CN=0123456789ABCDEF"

	for _, test := range []struct {
		mailbox pst.Mailbox
		want    string
	}{
		{pst.Mailbox{DisplayName: "Alice Example", SMTPAddress: "alice@example.com"}, `"Alice Example" <alice@example.com>`},
		{pst.Mailbox{DisplayName: "Example, Bob", AddressType: "SMTP", EmailAddress: "bob@example.com"}, `"Example, Bob" <bob@example.com>`},
		{pst.Mailbox{DisplayName: "Szűts Árvíz", SMTPAddress: "arviz@example.com"}, `=?utf-8?q?Sz=C5=B1ts_=C3=81rv=C3=ADz?= <arviz@example.com>`},
		{pst.Mailbox{DisplayName: "carol@example.com", SMTPAddress: "carol@example.com"}, `<carol@example.com>`},
		{pst.Mailbox{DisplayName: "Dave", AddressType: "EX", EmailAddress: x500}, `"Dave" <` + x500 + `>`},
		{pst.Mailbox{AddressType: "EX", EmailAddress: x500}, `<` + x500 + `>`},
		{pst.Mailbox{DisplayName: "Nobody"}, ``},
	} {
		if got := formatAddress(test.mailbox); got != test.want {
			t.Errorf("%+v: got %q, want %q", test.mailbox, got, test.want)
		}
	}
}

func TestStripSubjectPrefixMarker(t *testing.T) {
	for subject, want := range map[string]string{
		"\x01\nUpdated: Olympus training": "Updated: Olympus training",
		"\x01\x05RE: hello":               "RE: hello",
		"plain subject":                   "plain subject",
		"\x01":                            "",
	} {
		if got := stripSubjectPrefixMarker(subject); got != want {
			t.Errorf("%q: got %q, want %q", subject, got, want)
		}
	}
}

func TestEncodeText(t *testing.T) {
	for text, want := range map[string]string{
		"hello":           "hello",
		"árvíztűrő":       "=?utf-8?q?=C3=A1rv=C3=ADzt=C5=B1r=C5=91?=",
		"line\r\nbreak":   "line  break",
		"tab\tis allowed": "tab\tis allowed",
	} {
		if got := encodeText(text); got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}

func TestFoldHeaderField(t *testing.T) {
	value := strings.Repeat("word ", 40)

	lines := foldHeaderField("Subject", value)

	if len(lines) < 3 {
		t.Fatalf("got %d lines, want folding", len(lines))
	}

	for i, line := range lines {
		if len(line) > 78 {
			t.Errorf("line %d has %d characters", i, len(line))
		}

		if i > 0 && !strings.HasPrefix(line, " ") {
			t.Errorf("continuation line %d doesn't start with a space: %q", i, line)
		}
	}

	if got := strings.Join(lines, ""); got != "Subject: "+strings.TrimSpace(value) {
		t.Errorf("unfolded: got %q", got)
	}

	// A word longer than a line is not split.
	if got := foldHeaderField("X", strings.Repeat("a", 100)); len(got) != 1 {
		t.Errorf("got %d lines, want 1", len(got))
	}
}

func TestGetAttachmentMIMEType(t *testing.T) {
	for _, test := range []struct {
		mimeTag, filename, want string
	}{
		{"image/png", "x.bin", "image/png"},
		{"Application/PDF", "", "application/pdf"},
		{"", "photo.JPG", "image/jpeg"},
		{"", "archive.tar.gz", "application/gzip"},
		{"invalid", "doc.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"text/plain; charset=utf-8", "notes", "application/octet-stream"},
		{"", "", "application/octet-stream"},
	} {
		if got := getAttachmentMIMEType(test.mimeTag, test.filename); got != test.want {
			t.Errorf("%q %q: got %q, want %q", test.mimeTag, test.filename, got, test.want)
		}
	}
}

func TestLineWrapWriter(t *testing.T) {
	var output bytes.Buffer

	writer := &lineWrapWriter{writer: &output, lineLength: 4}

	for _, chunk := range []string{"ab", "cdefghij", "k"} {
		if _, err := writer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}

	if got := output.String(); got != "abcd\r\nefgh\r\nijk" {
		t.Errorf("got %q", got)
	}
}
