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

package pst

import (
	"strings"

	"github.com/rotisserie/eris"
	"golang.org/x/text/encoding/ianaindex"
)

// Property IDs of the message bodies.
// References [MS-OXCMSG]: 2.2.1.56 Message Content Properties.
const (
	propertyIDBody             = 0x1000 // PR_BODY
	propertyIDRTFCompressed    = 0x1009 // PR_RTF_COMPRESSED
	propertyIDHTML             = 0x1013 // PR_HTML
	propertyIDInternetCodePage = 0x3FDE // PR_INTERNET_CPID
)

// Body is the plain text and HTML body of a message, either may be empty.
type Body struct {
	// Text is PR_BODY, or the text converted from PR_RTF_COMPRESSED if there is neither PR_BODY nor PR_HTML.
	Text string
	// HTML is PR_HTML as stored, or the HTML de-encapsulated from PR_RTF_COMPRESSED (UTF-8) if there is no PR_HTML.
	HTML []byte
	// HTMLCodePage is the code page of HTML: PR_INTERNET_CPID, 65001 (UTF-8) for HTML from RTF or Unicode strings.
	// See GetCharsetName for its MIME charset name.
	HTMLCodePage int
	// FromRTF is set if Text or HTML was converted from PR_RTF_COMPRESSED.
	FromRTF bool
}

// GetBody returns the plain text and HTML bodies of the message.
// RTF is only used if there is no PR_HTML: RTF encapsulating HTML becomes the HTML body,
// other RTF becomes the text body if there is no PR_BODY.
func (message *Message) GetBody() (Body, error) {
	var body Body

	codePage := message.PropertyContext.GetCodePage()

	text, err := message.readBodyProperty(propertyIDBody)

	if err != nil {
		return Body{}, eris.Wrap(err, "failed to read PR_BODY")
	} else if text != nil {
		if body.Text, err = decodeBodyString(text, codePage); err != nil {
			return Body{}, eris.Wrap(err, "failed to decode PR_BODY")
		}
	}

	html, err := message.readBodyProperty(propertyIDHTML)

	if err != nil {
		return Body{}, eris.Wrap(err, "failed to read PR_HTML")
	} else if html != nil && len(html.data) > 0 {
		body.HTMLCodePage = message.getInternetCodePage(codePage)

		switch html.propertyType {
		case PropertyTypeString:
			// Unicode, decoded to UTF-8.
			decoded, err := decodeBodyString(html, codePage)

			if err != nil {
				return Body{}, eris.Wrap(err, "failed to decode PR_HTML")
			}

			body.HTML = []byte(decoded)
			body.HTMLCodePage = 65001
		default:
			// Binary or String8: the bytes in the internet code page.
			body.HTML = html.data
		}

		return body, nil
	}

	rtf, err := message.GetBodyRTF()

	if eris.Is(err, ErrPropertyNotFound) || eris.Is(err, ErrPropertyNoData) {
		return body, nil
	} else if err != nil {
		return Body{}, eris.Wrap(err, "failed to read PR_RTF_COMPRESSED")
	}

	rtfBody := ConvertRTF(rtf)

	switch {
	case rtfBody.IsHTML():
		body.HTML = []byte(rtfBody.HTML)
		body.HTMLCodePage = 65001
		body.FromRTF = true
	case body.Text == "" && rtfBody.Text != "":
		body.Text = rtfBody.Text
		body.FromRTF = true
	}

	return body, nil
}

// bodyProperty is the raw data of a body property.
type bodyProperty struct {
	propertyType PropertyType
	data         []byte
}

// readBodyProperty returns the raw data of the property, nil if the message doesn't have it.
func (message *Message) readBodyProperty(propertyID uint16) (*bodyProperty, error) {
	propertyReader, err := message.PropertyContext.GetPropertyReader(propertyID, message.LocalDescriptors)

	if eris.Is(err, ErrPropertyNotFound) || eris.Is(err, ErrPropertyNoData) {
		return nil, nil
	} else if err != nil {
		return nil, eris.Wrap(err, "failed to get property reader")
	}

	switch propertyReader.Property.Type {
	case PropertyTypeString, PropertyTypeString8, PropertyTypeBinary:
	default:
		return nil, ErrPropertyTypeMismatch
	}

	data := make([]byte, propertyReader.Size())

	if _, err := propertyReader.ReadAt(data, 0); err != nil {
		return nil, eris.Wrap(err, "failed to read property")
	}

	return &bodyProperty{propertyType: propertyReader.Property.Type, data: data}, nil
}

// decodeBodyString decodes a Unicode or String8 body property to UTF-8.
func decodeBodyString(property *bodyProperty, codePage int) (string, error) {
	propertyReader := PropertyReader{}

	switch property.propertyType {
	case PropertyTypeString:
		return propertyReader.DecodeString(property.data)
	case PropertyTypeString8:
		return propertyReader.DecodeString8(property.data, codePage)
	default:
		return "", ErrPropertyTypeMismatch
	}
}

// getInternetCodePage returns PR_INTERNET_CPID, the code page of the HTML body, or the given default.
func (message *Message) getInternetCodePage(defaultCodePage int) int {
	propertyReader, err := message.PropertyContext.GetPropertyReader(propertyIDInternetCodePage, message.LocalDescriptors)

	if err != nil {
		return defaultCodePage
	}

	codePage, err := propertyReader.GetInteger32()

	if err != nil || codePage <= 0 {
		return defaultCodePage
	}

	return int(codePage)
}

// GetCharsetName returns the MIME charset name of the code page, "" if it is unknown.
func GetCharsetName(codePage int) string {
	codePageEncoding := getCodePageEncoding(codePage)

	if codePageEncoding == nil {
		return ""
	}

	name, err := ianaindex.MIME.Name(codePageEncoding)

	if err != nil {
		return ""
	}

	return strings.ToLower(name)
}
