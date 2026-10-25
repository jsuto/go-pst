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

// Package eml converts the messages of PST files to RFC 822 / MIME (EML).
//
// Messages with PR_TRANSPORT_MESSAGE_HEADERS keep those headers (without the fields of the original MIME structure),
// other messages (sent mail, drafts) get headers built from their properties. The body structure is rebuilt from
// the plain text and HTML bodies (or RTF), attachments and embedded messages (message/rfc822).
package eml

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"strings"
	"unicode/utf8"

	pst "github.com/jsuto/go-pst/pkg"
	"github.com/jsuto/go-pst/pkg/properties"
	"github.com/rotisserie/eris"
)

// MaxDepth limits the nesting of embedded messages.
const MaxDepth = 10

// SkipError is returned by Convert for items that are not converted.
type SkipError struct {
	Kind         Kind
	MessageClass string
}

func (skipError *SkipError) Error() string {
	return fmt.Sprintf("%s (%s)", skipError.Kind, skipError.MessageClass)
}

// Result describes a converted message.
type Result struct {
	// Attachments is the number of attachments written, including embedded messages (also nested ones).
	Attachments int
	// SkippedAttachments lists the attachments that were not written, with the reason.
	SkippedAttachments []string
	// HasTransportHeaders is set if the header is PR_TRANSPORT_MESSAGE_HEADERS, otherwise it was built from the properties.
	HasTransportHeaders bool
}

// Convert writes the message as RFC 822 / MIME to the writer, with CRLF line endings.
// Items that are not mail (see Classify) return a *SkipError and write nothing.
// Panics while reading the message are returned as errors; the writer may have received part of the message,
// so write to a buffer or a temporary file.
func Convert(writer io.Writer, message *pst.Message) (result Result, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic while converting the message: %v", recovered)
		}
	}()

	messageClass, err := message.GetMessageClass()

	if err != nil {
		return Result{}, eris.Wrap(err, "failed to get message class")
	} else if kind := Classify(messageClass); kind != KindMail {
		return Result{}, &SkipError{Kind: kind, MessageClass: messageClass}
	}

	bufferedWriter := bufio.NewWriter(writer)
	converter := &converter{result: &result}

	if err := converter.writeMessage(bufferedWriter, message, 0); err != nil {
		return result, err
	}

	return result, bufferedWriter.Flush()
}

type converter struct {
	result *Result
}

// entity is a MIME entity: a multipart with its parts, or a leaf with its content.
type entity struct {
	mediaType  string
	parameters map[string]string
	// encoding is the Content-Transfer-Encoding of a leaf: quoted-printable, base64 or 8bit (written as it is).
	encoding string
	// fields are additional header fields (Content-Disposition, Content-ID).
	fields   [][2]string
	parts    []*entity
	boundary string
	write    func(writer io.Writer) error
}

func newMultipart(subtype string, parts ...*entity) *entity {
	return &entity{mediaType: "multipart/" + subtype, parameters: map[string]string{}, parts: parts, boundary: newBoundary()}
}

// newBoundary returns a random multipart boundary.
func newBoundary() string {
	random := make([]byte, 12)

	_, _ = rand.Read(random)

	return "----=_pst2eml_" + hex.EncodeToString(random)
}

// headerLines returns the Content-* fields of the entity.
func (entity *entity) headerLines() []string {
	parameters := map[string]string{}

	for key, value := range entity.parameters {
		parameters[key] = value
	}

	if entity.boundary != "" {
		parameters["boundary"] = entity.boundary
	}

	contentType := mime.FormatMediaType(entity.mediaType, parameters)

	if contentType == "" {
		contentType = entity.mediaType
	}

	lines := foldHeaderField("Content-Type", contentType)

	if entity.encoding != "" && entity.parts == nil {
		lines = append(lines, "Content-Transfer-Encoding: "+entity.encoding)
	}

	for _, field := range entity.fields {
		lines = append(lines, foldHeaderField(field[0], field[1])...)
	}

	return lines
}

// writeContent writes the content of the entity: its parts or its encoded content.
func (entity *entity) writeContent(writer io.Writer) error {
	if entity.parts != nil {
		for i, part := range entity.parts {
			delimiter := "\r\n--" + entity.boundary + "\r\n"

			if i == 0 {
				delimiter = delimiter[2:]
			}

			if _, err := io.WriteString(writer, delimiter+strings.Join(part.headerLines(), "\r\n")+"\r\n\r\n"); err != nil {
				return err
			}

			if err := part.writeContent(writer); err != nil {
				return err
			}
		}

		_, err := io.WriteString(writer, "\r\n--"+entity.boundary+"--\r\n")

		return err
	}

	if entity.write == nil {
		return nil
	}

	switch entity.encoding {
	case "quoted-printable":
		quotedPrintableWriter := quotedprintable.NewWriter(writer)

		if err := entity.write(quotedPrintableWriter); err != nil {
			return err
		}

		return quotedPrintableWriter.Close()
	case "base64":
		lineWriter := &lineWrapWriter{writer: writer, lineLength: 76}
		base64Writer := base64.NewEncoder(base64.StdEncoding, lineWriter)

		if err := entity.write(base64Writer); err != nil {
			return err
		}

		return base64Writer.Close()
	default:
		return entity.write(writer)
	}
}

// lineWrapWriter inserts CRLF every lineLength bytes (for base64).
type lineWrapWriter struct {
	writer     io.Writer
	lineLength int
	column     int
}

func (lineWrapWriter *lineWrapWriter) Write(data []byte) (int, error) {
	written := 0

	for len(data) > 0 {
		if lineWrapWriter.column == lineWrapWriter.lineLength {
			if _, err := io.WriteString(lineWrapWriter.writer, "\r\n"); err != nil {
				return written, err
			}

			lineWrapWriter.column = 0
		}

		chunk := min(len(data), lineWrapWriter.lineLength-lineWrapWriter.column)

		n, err := lineWrapWriter.writer.Write(data[:chunk])
		written += n
		lineWrapWriter.column += n

		if err != nil {
			return written, err
		}

		data = data[chunk:]
	}

	return written, nil
}

// writeMessage writes a message (or an embedded message) with its header.
func (converter *converter) writeMessage(writer io.Writer, message *pst.Message, depth int) error {
	// The properties of mail, whatever type the message class gave message.Properties.
	var messageProperties properties.Message

	if err := message.PropertyContext.Populate(&messageProperties, message.LocalDescriptors); err != nil {
		return eris.Wrap(err, "failed to populate message properties")
	}

	var headerLines []string

	if transportHeaders := messageProperties.GetTransportMessageHeaders(); strings.TrimSpace(transportHeaders) != "" {
		headerLines = getTransportHeaderLines(transportHeaders)

		if depth == 0 {
			converter.result.HasTransportHeaders = true
		}
	} else {
		lines, err := buildHeaderLines(message, &messageProperties)

		if err != nil {
			return eris.Wrap(err, "failed to build header")
		}

		headerLines = lines
	}

	root, err := converter.buildEntity(message, depth)

	if err != nil {
		return err
	}

	headerLines = append(headerLines, "MIME-Version: 1.0")
	headerLines = append(headerLines, root.headerLines()...)

	if _, err := io.WriteString(writer, strings.Join(headerLines, "\r\n")+"\r\n\r\n"); err != nil {
		return err
	}

	return root.writeContent(writer)
}

// buildEntity builds the MIME structure of the message:
// multipart/mixed (attachments) of multipart/related (inline attachments) of multipart/alternative (text and HTML).
func (converter *converter) buildEntity(message *pst.Message, depth int) (*entity, error) {
	body, err := message.GetBody()

	if err != nil {
		return nil, eris.Wrap(err, "failed to get body")
	}

	var textEntity, htmlEntity *entity

	if body.Text != "" {
		text := body.Text
		textEntity = &entity{
			mediaType:  "text/plain",
			parameters: map[string]string{"charset": "utf-8"},
			encoding:   "quoted-printable",
			write: func(writer io.Writer) error {
				_, err := io.WriteString(writer, text)
				return err
			},
		}
	}

	if len(body.HTML) > 0 {
		html := body.HTML
		parameters := map[string]string{}

		if charset := pst.GetCharsetName(body.HTMLCodePage); charset != "" {
			parameters["charset"] = charset
		}

		htmlEntity = &entity{
			mediaType:  "text/html",
			parameters: parameters,
			encoding:   "quoted-printable",
			write: func(writer io.Writer) error {
				_, err := writer.Write(html)
				return err
			},
		}
	}

	var bodyEntity *entity

	switch {
	case textEntity != nil && htmlEntity != nil:
		bodyEntity = newMultipart("alternative", textEntity, htmlEntity)
	case htmlEntity != nil:
		bodyEntity = htmlEntity
	case textEntity != nil:
		bodyEntity = textEntity
	default:
		bodyEntity = &entity{mediaType: "text/plain", parameters: map[string]string{"charset": "utf-8"}, encoding: "7bit"}
	}

	inlineEntities, attachmentEntities, err := converter.buildAttachments(message, body.HTML, depth)

	if err != nil {
		return nil, err
	}

	if len(inlineEntities) > 0 {
		related := newMultipart("related", append([]*entity{bodyEntity}, inlineEntities...)...)
		related.parameters["type"] = bodyEntity.mediaType
		bodyEntity = related
	}

	if len(attachmentEntities) > 0 {
		return newMultipart("mixed", append([]*entity{bodyEntity}, attachmentEntities...)...), nil
	}

	return bodyEntity, nil
}

// buildAttachments returns the inline attachments (referenced by the HTML body) and the other attachments.
func (converter *converter) buildAttachments(message *pst.Message, html []byte, depth int) ([]*entity, []*entity, error) {
	attachments, err := message.GetAllAttachments()

	if err != nil {
		return nil, nil, eris.Wrap(err, "failed to get attachments")
	}

	lowerHTML := bytes.ToLower(html)

	var inlineEntities, attachmentEntities []*entity

	for i, attachment := range attachments {
		filename := attachment.GetAttachLongFilename()

		if filename == "" {
			filename = attachment.GetAttachFilename()
		}

		skip := func(reason string) {
			converter.result.SkippedAttachments = append(converter.result.SkippedAttachments, fmt.Sprintf("attachment %d (%q): %s", i+1, filename, reason))
		}

		attachmentEntity := &entity{parameters: map[string]string{}}

		switch {
		case attachment.IsEmbeddedMessage():
			embeddedMessage, err := attachment.GetEmbeddedMessage()

			if err != nil {
				skip(fmt.Sprintf("failed to read embedded message: %v", err))
				continue
			} else if depth+1 >= MaxDepth {
				skip("embedded messages nested too deep")
				continue
			}

			attachmentEntity.mediaType = "message/rfc822"
			attachmentEntity.encoding = "8bit"
			attachmentEntity.write = func(writer io.Writer) error {
				return converter.writeMessage(writer, embeddedMessage, depth+1)
			}
		case attachment.GetAttachMethod() == pst.AttachMethodByValue:
			attachmentEntity.mediaType = getAttachmentMIMEType(attachment.GetAttachMimeTag(), filename)
			attachmentEntity.encoding = "base64"
			attachmentEntity.write = func(writer io.Writer) error {
				_, err := attachment.WriteTo(writer)
				return err
			}

			if strings.HasPrefix(attachmentEntity.mediaType, "text/") {
				// The charset of text attachments isn't stored, declare UTF-8 if the data is UTF-8 (and not ASCII).
				var data bytes.Buffer

				if _, err := attachment.WriteTo(&data); err != nil {
					skip(fmt.Sprintf("failed to read attachment: %v", err))
					continue
				}

				if isNonASCIIUTF8(data.Bytes()) {
					attachmentEntity.parameters["charset"] = "utf-8"
				}

				attachmentEntity.write = func(writer io.Writer) error {
					_, err := writer.Write(data.Bytes())
					return err
				}
			}

			if filename != "" {
				attachmentEntity.parameters["name"] = filename
			}
		default:
			skip(fmt.Sprintf("attachment method %d is not supported (by reference or OLE)", attachment.GetAttachMethod()))
			continue
		}

		contentID := strings.Trim(attachment.GetAttachContentId(), "<> ")
		inline := len(html) > 0 && !attachment.IsEmbeddedMessage() &&
			((contentID != "" && bytes.Contains(lowerHTML, []byte("cid:"+strings.ToLower(contentID)))) || attachment.IsInline())

		disposition := "attachment"

		if inline {
			disposition = "inline"
		}

		dispositionParameters := map[string]string{}

		if filename != "" {
			dispositionParameters["filename"] = filename
		}

		if formatted := mime.FormatMediaType(disposition, dispositionParameters); formatted != "" {
			attachmentEntity.fields = append(attachmentEntity.fields, [2]string{"Content-Disposition", formatted})
		} else {
			attachmentEntity.fields = append(attachmentEntity.fields, [2]string{"Content-Disposition", disposition})
		}

		if contentID != "" {
			attachmentEntity.fields = append(attachmentEntity.fields, [2]string{"Content-ID", "<" + contentID + ">"})
		}

		converter.result.Attachments++

		if inline {
			inlineEntities = append(inlineEntities, attachmentEntity)
		} else {
			attachmentEntities = append(attachmentEntities, attachmentEntity)
		}
	}

	return inlineEntities, attachmentEntities, nil
}

// isNonASCIIUTF8 returns true if the data is valid UTF-8 with non-ASCII characters.
func isNonASCIIUTF8(data []byte) bool {
	for _, character := range data {
		if character >= 0x80 {
			return utf8.Valid(data)
		}
	}

	return false
}
