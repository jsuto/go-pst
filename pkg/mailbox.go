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
	"github.com/rotisserie/eris"
)

// Mailbox is a display name with its addresses, as stored for recipients and senders.
type Mailbox struct {
	DisplayName string
	// AddressType is "SMTP" or "EX" (EmailAddress is then an X.500 address), among others.
	AddressType  string
	EmailAddress string
	SMTPAddress  string
}

// Address returns the SMTP address if there is one, otherwise EmailAddress
// (for example an X.500 address if AddressType is "EX").
func (mailbox Mailbox) Address() string {
	if mailbox.SMTPAddress != "" {
		return mailbox.SMTPAddress
	}

	return mailbox.EmailAddress
}

// IsEmpty returns true if the mailbox has neither a display name nor an address.
func (mailbox Mailbox) IsEmpty() bool {
	return mailbox.DisplayName == "" && mailbox.Address() == ""
}

// Property IDs of the sender of a message.
// References [MS-OXPROPS], [MS-OXOMSG]: 2.2.1.48 PidTagSenderSmtpAddress.
const (
	propertyIDSentRepresentingName         = 0x0042 // PR_SENT_REPRESENTING_NAME
	propertyIDSentRepresentingAddressType  = 0x0064 // PR_SENT_REPRESENTING_ADDRTYPE
	propertyIDSentRepresentingEmailAddress = 0x0065 // PR_SENT_REPRESENTING_EMAIL_ADDRESS
	propertyIDSentRepresentingSMTPAddress  = 0x5D02 // PR_SENT_REPRESENTING_SMTP_ADDRESS
	propertyIDSenderName                   = 0x0C1A // PR_SENDER_NAME
	propertyIDSenderAddressType            = 0x0C1E // PR_SENDER_ADDRTYPE
	propertyIDSenderEmailAddress           = 0x0C1F // PR_SENDER_EMAIL_ADDRESS
	propertyIDSenderSMTPAddress            = 0x5D01 // PR_SENDER_SMTP_ADDRESS
)

// GetSender returns the PR_SENDER_* properties: the mailbox that sent the message.
// Mail without PR_SENDER_SMTP_ADDRESS (for example in Sent Items) may still have an SMTP address in EmailAddress.
func (message *Message) GetSender() (Mailbox, error) {
	return message.getMailbox(propertyIDSenderName, propertyIDSenderAddressType, propertyIDSenderEmailAddress, propertyIDSenderSMTPAddress)
}

// GetSentRepresenting returns the PR_SENT_REPRESENTING_* properties: the mailbox the message was sent on behalf of
// (the From header), which is the same as GetSender unless the message was sent by a delegate.
func (message *Message) GetSentRepresenting() (Mailbox, error) {
	return message.getMailbox(propertyIDSentRepresentingName, propertyIDSentRepresentingAddressType, propertyIDSentRepresentingEmailAddress, propertyIDSentRepresentingSMTPAddress)
}

// getMailbox reads the mailbox from the property context of the message, missing properties are left empty.
func (message *Message) getMailbox(nameID uint16, addressTypeID uint16, emailAddressID uint16, smtpAddressID uint16) (Mailbox, error) {
	var mailbox Mailbox

	for _, field := range []struct {
		propertyID uint16
		value      *string
	}{
		{nameID, &mailbox.DisplayName},
		{addressTypeID, &mailbox.AddressType},
		{emailAddressID, &mailbox.EmailAddress},
		{smtpAddressID, &mailbox.SMTPAddress},
	} {
		propertyReader, err := message.PropertyContext.GetPropertyReader(field.propertyID, message.LocalDescriptors)

		if eris.Is(err, ErrPropertyNotFound) || eris.Is(err, ErrPropertyNoData) {
			continue
		} else if err != nil {
			return Mailbox{}, eris.Wrapf(err, "failed to get property reader: %x", field.propertyID)
		}

		value, err := readString(&propertyReader)

		if eris.Is(err, ErrPropertyNoData) {
			continue
		} else if err != nil {
			return Mailbox{}, eris.Wrapf(err, "failed to read property: %x", field.propertyID)
		}

		*field.value = value
	}

	return mailbox, nil
}

// readString reads a Unicode or 8-bit string property.
func readString(propertyReader *PropertyReader) (string, error) {
	switch propertyReader.Property.Type {
	case PropertyTypeString:
		return propertyReader.GetString()
	case PropertyTypeString8:
		return propertyReader.GetString8(65001) // TODO - Use the code page of the message, as in WriteMessagePackValue.
	default:
		return "", ErrPropertyTypeMismatch
	}
}
