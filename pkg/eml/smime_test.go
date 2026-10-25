package eml

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emersion/go-message"
	pst "github.com/jsuto/go-pst/pkg"
)

var smimeHeaderLines = []string{"From: \"Test Signer\" <signer@example.com>", "To: <recipient@example.com>", "Subject: S/MIME test"}

func readSMIMESample(t *testing.T, name string) []byte {
	data, err := os.ReadFile(filepath.Join("testdata", "smime", name))

	if err != nil {
		t.Fatal(err)
	}

	return data
}

// convertSMIME runs writeSMIME and returns the message.
func convertSMIME(t *testing.T, transportHeaders string, entity []byte) []byte {
	var output bytes.Buffer

	written, err := writeSMIME(&output, smimeHeaderLines, transportHeaders, entity)

	if err != nil {
		t.Fatal(err)
	} else if !written {
		t.Fatal("the S/MIME entity was not recognized")
	}

	return output.Bytes()
}

// runOpenSSL runs openssl smime with the message as input, skipping if openssl is not installed.
func runOpenSSL(t *testing.T, message []byte, arguments ...string) []byte {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Log("openssl not found, not verifying")
		return nil
	}

	path := filepath.Join(t.TempDir(), "message.eml")

	if err := os.WriteFile(path, message, 0o600); err != nil {
		t.Fatal(err)
	}

	output, err := exec.Command("openssl", append(append([]string{"smime"}, arguments...), "-in", path)...).CombinedOutput()

	if err != nil {
		t.Fatalf("openssl smime %v: %v: %s", arguments, err, output)
	}

	return output
}

func TestWriteSMIMESigned(t *testing.T) {
	entity := readSMIMESample(t, "signed.eml")
	output := convertSMIME(t, "", entity)

	// The entity with its header follows the message header unchanged, MIME-Version is not repeated.
	if want := strings.Join(smimeHeaderLines, "\r\n") + "\r\n"; !bytes.Equal(output, append([]byte(want), entity...)) {
		t.Errorf("got %q", output)
	}

	runOpenSSL(t, output, "-verify", "-noverify", "-out", os.DevNull)
}

func TestWriteSMIMESignedWithoutHeader(t *testing.T) {
	signed := readSMIMESample(t, "signed.eml")
	headerEnd := bytes.Index(signed, []byte("\n\n"))
	originalContentType := getHeaderField(string(signed[:headerEnd]), "Content-Type")
	body := signed[headerEnd+2:]

	for name, transportHeaders := range map[string]string{
		"with the Content-Type of the transport headers": "Received: from a\r\n" + strings.Join(originalContentType, "\r\n") + "\r\nSubject: x\r\n\r\n",
		"with the boundary of the body":                  "",
	} {
		output := convertSMIME(t, transportHeaders, body)

		entity, err := message.Read(bytes.NewReader(output))

		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		mediaType, parameters, _ := entity.Header.ContentType()

		if mediaType != "multipart/signed" || !bytes.Contains(body, []byte("\n--"+parameters["boundary"]+"\n")) {
			t.Errorf("%s: got %s %v", name, mediaType, parameters)
		} else if !bytes.HasSuffix(output, body) {
			t.Errorf("%s: the signed body changed", name)
		}

		runOpenSSL(t, output, "-verify", "-noverify", "-out", os.DevNull)
	}
}

func TestWriteSMIMEPKCS7(t *testing.T) {
	content := readSMIMESample(t, "content.txt")

	for _, test := range []struct {
		sample, smimeType string
		arguments         []string
	}{
		{"enveloped.p7m", "enveloped-data", []string{"-decrypt", "-inkey", "testdata/smime/signer.key", "-recip", "testdata/smime/signer.crt"}},
		{"opaque-signed.p7m", "signed-data", []string{"-verify", "-noverify"}},
	} {
		entity := readSMIMESample(t, test.sample)
		output := convertSMIME(t, "", entity)

		parsed, err := message.Read(bytes.NewReader(output))

		if err != nil {
			t.Fatalf("%s: %v", test.sample, err)
		}

		mediaType, parameters, _ := parsed.Header.ContentType()

		if mediaType != "application/pkcs7-mime" || parameters["smime-type"] != test.smimeType {
			t.Errorf("%s: got %s %v", test.sample, mediaType, parameters)
		}

		var decoded bytes.Buffer

		if _, err := decoded.ReadFrom(parsed.Body); err != nil {
			t.Fatal(err)
		} else if !bytes.Equal(decoded.Bytes(), entity) {
			t.Errorf("%s: the decoded body is not the PKCS #7 object", test.sample)
		}

		if result := runOpenSSL(t, output, test.arguments...); result != nil {
			normalize := func(data []byte) string { return strings.ReplaceAll(string(data), "\r\n", "\n") }

			if !strings.Contains(normalize(result), normalize(content[bytes.Index(content, []byte("\r\n\r\n"))+4:])) {
				t.Errorf("%s: openssl output %q does not contain the content", test.sample, result)
			}
		}
	}
}

func TestWriteSMIMEUnknown(t *testing.T) {
	var output bytes.Buffer

	if written, err := writeSMIME(&output, smimeHeaderLines, "", []byte("not an S/MIME entity")); written || err != nil || output.Len() != 0 {
		t.Errorf("got %v, %v and %d bytes, want nothing written", written, err, output.Len())
	}
}

func TestIsSMIMEAttachment(t *testing.T) {
	for _, test := range []struct {
		messageClass string
		count        int
		method       int32
		mimeTag      string
		filename     string
		want         bool
	}{
		{"IPM.Note.SMIME.MultipartSigned", 1, pst.AttachMethodByValue, "multipart/signed", "smime.p7m", true},
		{"IPM.Note.SMIME", 1, pst.AttachMethodByValue, "application/pkcs7-mime", "smime.p7m", true},
		{"ipm.note.smime", 1, pst.AttachMethodByValue, "", "SMIME.P7M", true},
		{"IPM.Note.SMIME", 2, pst.AttachMethodByValue, "application/pkcs7-mime", "smime.p7m", false},
		{"IPM.Note.SMIME", 1, pst.AttachMethodEmbeddedMessage, "", "", false},
		{"IPM.Note.SMIME", 1, pst.AttachMethodByValue, "application/pdf", "report.pdf", false},
		{"IPM.Note", 1, pst.AttachMethodByValue, "application/pkcs7-mime", "smime.p7m", false},
	} {
		if got := isSMIMEMessageClass(test.messageClass) && isSMIMEAttachment(test.count, test.method, test.mimeTag, test.filename); got != test.want {
			t.Errorf("%+v: got %v", test, got)
		}
	}
}
