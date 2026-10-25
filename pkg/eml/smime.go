// go-pst is a library for reading Personal Storage Table (.pst) files (written in Go/Golang).
//
// Copyright 2026 Janos SUTO
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package eml

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"path"
	"strings"

	pst "github.com/jsuto/go-pst/pkg"
)

// S/MIME messages (IPM.Note.SMIME, IPM.Note.SMIME.MultipartSigned) keep the signed or encrypted MIME entity as their
// only attachment; the body properties are Outlook's decoded copy. The message is rebuilt from the entity, so the
// signature stays verifiable. References [MS-OXOSMIME]: 2.1.3 Message Content.

// isSMIMEMessageClass returns true for the message classes of S/MIME messages.
func isSMIMEMessageClass(messageClass string) bool {
	return hasClassPrefix(strings.ToLower(messageClass), "ipm.note.smime")
}

// smimeAttachmentMIMETypes are the types of the attachment holding the S/MIME entity.
var smimeAttachmentMIMETypes = map[string]bool{
	"multipart/signed": true, "application/pkcs7-mime": true, "application/x-pkcs7-mime": true,
}

// isSMIMEAttachment returns true if the attachment holds the S/MIME entity: an S/MIME message has exactly one,
// smime.p7m (or .p7s) or of an S/MIME type.
func isSMIMEAttachment(attachmentCount int, attachMethod int32, mimeTag string, filename string) bool {
	if attachmentCount != 1 || attachMethod != pst.AttachMethodByValue {
		return false
	}

	extension := strings.ToLower(path.Ext(filename))

	return smimeAttachmentMIMETypes[strings.ToLower(strings.TrimSpace(mimeTag))] || extension == ".p7m" || extension == ".p7s"
}

// getSMIMEEntity returns the data of the attachment holding the S/MIME entity, nil if the message isn't S/MIME
// or doesn't have it.
func getSMIMEEntity(message *pst.Message, messageClass string) ([]byte, error) {
	if !isSMIMEMessageClass(messageClass) {
		return nil, nil
	}

	attachments, err := message.GetAllAttachments()

	if err != nil {
		return nil, err
	} else if len(attachments) != 1 {
		return nil, nil
	}

	attachment := attachments[0]
	filename := attachment.GetAttachLongFilename()

	if filename == "" {
		filename = attachment.GetAttachFilename()
	}

	if !isSMIMEAttachment(len(attachments), attachment.GetAttachMethod(), attachment.GetAttachMimeTag(), filename) {
		return nil, nil
	}

	var data bytes.Buffer

	if _, err := attachment.WriteTo(&data); err != nil {
		return nil, err
	}

	return data.Bytes(), nil
}

// writeSMIME writes the message from its S/MIME entity, returns false (and writes nothing) if the entity is not
// recognized, so the message is converted as an ordinary one.
//   - An entity with its MIME header (Content-Type...) is written as it is below the message header.
//   - A clear-signed body without its header gets the Content-Type of the transport headers, or multipart/signed
//     with the boundary of its first boundary line.
//   - A PKCS #7 object (DER) is written base64-encoded as application/pkcs7-mime, with the Content-Type of the
//     transport headers or with the smime-type of its content type.
func writeSMIME(writer io.Writer, headerLines []string, transportHeaders string, entity []byte) (bool, error) {
	header := append([]string(nil), headerLines...)
	originalContentType := getHeaderField(transportHeaders, "Content-Type")

	switch {
	case hasEntityHeader(entity):
		if getHeaderField(string(entity[:min(len(entity), maxEntityHeaderLength)]), "MIME-Version") == nil {
			header = append(header, "MIME-Version: 1.0")
		}

		// The fields of the entity continue the header.
		return true, writeLines(writer, header, "", entity)
	case isMultipartSigned(originalContentType, entity):
		header = append(header, "MIME-Version: 1.0")

		if strings.HasPrefix(strings.ToLower(strings.Join(originalContentType, "")), "content-type: multipart/signed") {
			header = append(header, originalContentType...)
		} else {
			header = append(header, foldHeaderField("Content-Type", mime.FormatMediaType("multipart/signed",
				map[string]string{"protocol": "application/pkcs7-signature", "boundary": findBoundary(entity)}))...)
		}

		return true, writeLines(writer, header, "\r\n", entity)
	case len(entity) > 0 && entity[0] == 0x30:
		// DER SEQUENCE: a PKCS #7 / CMS ContentInfo.
		header = append(header, "MIME-Version: 1.0")

		if strings.Contains(strings.ToLower(strings.Join(originalContentType, "")), "pkcs7-mime") {
			header = append(header, originalContentType...)
		} else {
			parameters := map[string]string{"name": "smime.p7m"}

			if smimeType := getSMIMEType(entity); smimeType != "" {
				parameters["smime-type"] = smimeType
			}

			header = append(header, foldHeaderField("Content-Type", mime.FormatMediaType("application/pkcs7-mime", parameters))...)
		}

		header = append(header, "Content-Transfer-Encoding: base64", `Content-Disposition: attachment; filename="smime.p7m"`)

		if err := writeLines(writer, header, "\r\n", nil); err != nil {
			return true, err
		}

		lineWriter := &lineWrapWriter{writer: writer, lineLength: 76}
		base64Writer := base64.NewEncoder(base64.StdEncoding, lineWriter)

		if _, err := base64Writer.Write(entity); err != nil {
			return true, err
		} else if err := base64Writer.Close(); err != nil {
			return true, err
		}

		_, err := io.WriteString(writer, "\r\n")

		return true, err
	default:
		return false, nil
	}
}

// isMultipartSigned returns true if the entity is the body of a clear-signed message: the original Content-Type
// (of the transport headers) is multipart/signed, or the body has a boundary and a PKCS #7 signature part.
func isMultipartSigned(originalContentType []string, entity []byte) bool {
	if strings.HasPrefix(strings.ToLower(strings.Join(originalContentType, "")), "content-type: multipart/signed") {
		return true
	}

	lowerEntity := bytes.ToLower(entity)

	return findBoundary(entity) != "" &&
		(bytes.Contains(lowerEntity, []byte("application/pkcs7-signature")) || bytes.Contains(lowerEntity, []byte("application/x-pkcs7-signature")))
}

// findBoundary returns the boundary of the first boundary line ("--boundary") at the start of a multipart body
// (after the preamble), "" if there is none.
func findBoundary(entity []byte) string {
	prefix := string(entity[:min(len(entity), 4096)])

	for _, line := range strings.Split(strings.ReplaceAll(prefix, "\r\n", "\n"), "\n") {
		if boundary := strings.TrimSpace(strings.TrimPrefix(line, "--")); strings.HasPrefix(line, "--") && boundary != "" && !strings.ContainsAny(boundary, " \t") {
			return boundary
		}
	}

	return ""
}

// maxEntityHeaderLength limits the search for the header of an S/MIME entity.
const maxEntityHeaderLength = 16384

// hasEntityHeader returns true if the entity starts with a MIME header (header fields with a Content-Type,
// followed by an empty line).
func hasEntityHeader(entity []byte) bool {
	prefix := string(entity[:min(len(entity), maxEntityHeaderLength)])
	end := strings.Index(prefix, "\n\n")

	if crlfEnd := strings.Index(prefix, "\r\n\r\n"); crlfEnd >= 0 && (end < 0 || crlfEnd < end) {
		end = crlfEnd
	}

	if end < 0 {
		return false
	}

	lines := strings.Split(strings.ReplaceAll(prefix[:end], "\r\n", "\n"), "\n")

	for i, line := range lines {
		if line == "" {
			return false
		} else if line[0] == ' ' || line[0] == '\t' {
			if i == 0 {
				return false
			}

			continue
		}

		if name, _, isField := strings.Cut(line, ":"); !isField || !isHeaderFieldName(name) {
			return false
		}
	}

	return getHeaderField(prefix[:end], "Content-Type") != nil
}

// writeLines writes the header lines with CRLF, then the separator ("\r\n" ends the header) and the content.
func writeLines(writer io.Writer, lines []string, separator string, content []byte) error {
	if _, err := io.WriteString(writer, strings.Join(lines, "\r\n")+"\r\n"+separator); err != nil {
		return err
	}

	_, err := writer.Write(content)

	return err
}

// smimeTypes maps the DER encoding of CMS content type OIDs to the smime-type parameter.
// References RFC 8551: 3.2.2 The smime-type Parameter.
var smimeTypes = []struct {
	oid       []byte
	smimeType string
}{
	{[]byte{0x06, 0x09, 0x2A, 0x86, 0x48, 0x86, 0xF7, 0x0D, 0x01, 0x07, 0x03}, "enveloped-data"},                 // 1.2.840.113549.1.7.3
	{[]byte{0x06, 0x09, 0x2A, 0x86, 0x48, 0x86, 0xF7, 0x0D, 0x01, 0x07, 0x02}, "signed-data"},                    // 1.2.840.113549.1.7.2
	{[]byte{0x06, 0x0B, 0x2A, 0x86, 0x48, 0x86, 0xF7, 0x0D, 0x01, 0x09, 0x10, 0x01, 0x17}, "authEnveloped-data"}, // 1.2.840.113549.1.9.16.1.23
	{[]byte{0x06, 0x0B, 0x2A, 0x86, 0x48, 0x86, 0xF7, 0x0D, 0x01, 0x09, 0x10, 0x01, 0x09}, "compressed-data"},    // 1.2.840.113549.1.9.16.1.9
}

// getSMIMEType returns the smime-type of a PKCS #7 / CMS object from its content type, the first OID.
func getSMIMEType(entity []byte) string {
	prefix := entity[:min(len(entity), 32)]
	first, smimeType := len(prefix), ""

	for _, candidate := range smimeTypes {
		if index := bytes.Index(prefix, candidate.oid); index >= 0 && index < first {
			first, smimeType = index, candidate.smimeType
		}
	}

	return smimeType
}
