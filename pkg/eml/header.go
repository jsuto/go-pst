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
	"mime"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	pst "github.com/jsuto/go-pst/pkg"
	"github.com/jsuto/go-pst/pkg/properties"
)

// replacedHeaderFields are the fields of the transport headers describing the original MIME structure,
// replaced by the structure built from the properties.
var replacedHeaderFields = map[string]bool{
	"content-type": true, "content-transfer-encoding": true, "content-disposition": true, "mime-version": true,
}

// getTransportHeaderLines returns the lines of PR_TRANSPORT_MESSAGE_HEADERS as they are, without the fields of
// replacedHeaderFields and without lines that are not header fields (Exchange inserts
// "Microsoft Mail Internet Headers Version 2.0", for example).
func getTransportHeaderLines(transportHeaders string) []string {
	var lines []string

	keep := false

	for _, line := range strings.Split(transportHeaders, "\n") {
		line = strings.TrimSuffix(line, "\r")

		switch {
		case line == "":
			if len(lines) > 0 {
				// End of the header.
				return lines
			}
		case line[0] == ' ' || line[0] == '\t':
			// Continuation of the previous field.
			if keep {
				lines = append(lines, line)
			}
		default:
			name, _, isField := strings.Cut(line, ":")
			keep = isField && isHeaderFieldName(name) && !replacedHeaderFields[strings.ToLower(name)]

			if keep {
				lines = append(lines, line)
			}
		}
	}

	return lines
}

// isHeaderFieldName returns true if the name consists of printable ASCII characters except the colon.
// References RFC 5322: 2.2 Header Fields.
func isHeaderFieldName(name string) bool {
	if name == "" {
		return false
	}

	for i := 0; i < len(name); i++ {
		if name[i] <= ' ' || name[i] > '~' {
			return false
		}
	}

	return true
}

// buildHeaderLines builds the header of a message without transport headers from its properties.
func buildHeaderLines(message *pst.Message, messageProperties *properties.Message) ([]string, error) {
	var lines []string

	add := func(name string, value string) {
		if value != "" {
			lines = append(lines, foldHeaderField(name, value)...)
		}
	}

	if date := getMessageDate(messageProperties); !date.IsZero() {
		add("Date", date.UTC().Format("Mon, 02 Jan 2006 15:04:05 -0700"))
	}

	from, err := message.GetSentRepresenting()

	if err != nil {
		return nil, err
	}

	sender, err := message.GetSender()

	if err != nil {
		return nil, err
	}

	add("From", formatAddress(from))

	if sender.Address() != "" && !strings.EqualFold(sender.Address(), from.Address()) {
		add("Sender", formatAddress(sender))
	}

	add("Subject", encodeText(stripSubjectPrefixMarker(messageProperties.GetSubject())))

	recipients, err := message.GetRecipients()

	if err != nil {
		return nil, err
	}

	addressLists := map[pst.RecipientType][]string{}

	for _, recipient := range recipients {
		if recipient.P1 {
			continue
		}

		if address := formatAddress(recipient.Mailbox); address != "" {
			addressLists[recipient.Type] = append(addressLists[recipient.Type], address)
		}
	}

	add("To", strings.Join(addressLists[pst.RecipientTypeTo], ", "))
	add("Cc", strings.Join(addressLists[pst.RecipientTypeCc], ", "))
	add("Bcc", strings.Join(addressLists[pst.RecipientTypeBcc], ", "))
	add("Message-ID", strings.TrimSpace(messageProperties.GetInternetMessageId()))
	add("In-Reply-To", strings.TrimSpace(messageProperties.GetInReplyToId()))
	add("References", strings.TrimSpace(messageProperties.GetInternetReferences()))

	// PR_IMPORTANCE: 0 low, 1 normal, 2 high.
	switch messageProperties.GetImportance() {
	case 0:
		if messageProperties.Importance != nil {
			add("Importance", "low")
		}
	case 2:
		add("Importance", "high")
	}

	return lines, nil
}

// getMessageDate returns PR_CLIENT_SUBMIT_TIME, else PR_MESSAGE_DELIVERY_TIME, else PR_CREATION_TIME (drafts).
func getMessageDate(messageProperties *properties.Message) time.Time {
	for _, date := range []int64{messageProperties.GetClientSubmitTime(), messageProperties.GetMessageDeliveryTime(), messageProperties.GetCreationTime()} {
		if date != 0 {
			return time.Unix(0, date)
		}
	}

	return time.Time{}
}

// stripSubjectPrefixMarker removes the 0x01 and the length character PST files put before the subject prefix.
func stripSubjectPrefixMarker(subject string) string {
	if strings.HasPrefix(subject, "\x01") {
		subject = subject[1:]

		if _, size := utf8.DecodeRuneInString(subject); size > 0 {
			subject = subject[size:]
		}
	}

	return subject
}

// encodeText encodes unstructured text with RFC 2047 if it isn't ASCII.
func encodeText(text string) string {
	for i := 0; i < len(text); i++ {
		if text[i] >= 0x80 || text[i] < ' ' && text[i] != '\t' {
			return mime.QEncoding.Encode("utf-8", strings.Map(replaceControlCharacters, text))
		}
	}

	return text
}

// replaceControlCharacters replaces line breaks and other control characters with spaces.
func replaceControlCharacters(character rune) rune {
	if character < ' ' && character != '\t' {
		return ' '
	}

	return character
}

// formatAddress formats a mailbox as "Display Name" <address> (RFC 2047 for non-ASCII display names).
// Addresses without @ (X.500 addresses of Exchange) are written as they are, "" if there is no address.
func formatAddress(mailbox pst.Mailbox) string {
	address := strings.TrimSpace(mailbox.Address())
	name := strings.TrimSpace(strings.Map(replaceControlCharacters, mailbox.DisplayName))

	if address == "" {
		return ""
	} else if strings.EqualFold(name, address) {
		name = ""
	}

	if strings.Contains(address, "@") && !strings.ContainsAny(address, " <>") {
		return (&mail.Address{Name: name, Address: address}).String()
	}

	if name == "" {
		return "<" + address + ">"
	}

	// Format the display name the way net/mail does, with a placeholder address.
	formattedName := strings.TrimSuffix((&mail.Address{Name: name, Address: "x@x"}).String(), " <x@x>")

	return formattedName + " <" + address + ">"
}

// foldHeaderField formats a header field, folding it at spaces into lines of at most 78 characters if possible.
func foldHeaderField(name string, value string) []string {
	var lines []string

	line := name + ":"

	for _, word := range strings.Split(value, " ") {
		if word == "" {
			continue
		}

		// Continuation lines start with the space before the word.
		if len(line)+1+len(word) > 78 && line != name+":" {
			lines = append(lines, line)
			line = ""
		}

		line += " " + word
	}

	return append(lines, line)
}
