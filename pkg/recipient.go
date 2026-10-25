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

// RecipientType is the type of recipient (PR_RECIPIENT_TYPE without its flags).
// References [MS-OXOMSG]: 2.2.3.1 PidTagRecipientType.
type RecipientType int32

// Constants defining the recipient types.
const (
	RecipientTypeOriginator RecipientType = 0
	RecipientTypeTo         RecipientType = 1
	RecipientTypeCc         RecipientType = 2
	RecipientTypeBcc        RecipientType = 3
)

// Flags of PR_RECIPIENT_TYPE.
const (
	recipientTypeFlagP1        = 0x10000000
	recipientTypeFlagSubmitted = 0x80000000
)

// Property IDs read from the recipient table.
// References [MS-OXPROPS].
const (
	propertyIDRecipientType = 0x0C15 // PR_RECIPIENT_TYPE
	propertyIDDisplayName   = 0x3001 // PR_DISPLAY_NAME
	propertyIDAddressType   = 0x3002 // PR_ADDRTYPE
	propertyIDEmailAddress  = 0x3003 // PR_EMAIL_ADDRESS
	propertyIDSMTPAddress   = 0x39FE // PR_SMTP_ADDRESS
)

// recipientTableIdentifier is the local descriptor (subnode) of a message holding the recipient table.
// References [MS-PST]: 2.4.5 Message Objects.
const recipientTableIdentifier Identifier = 0x692

// Recipient represents a row of the recipient table of a message.
type Recipient struct {
	Type RecipientType
	// P1 is set for recipients of a resent message that are only used by the transport (MAPI_P1),
	// these are not shown as recipients of the message.
	P1 bool
	Mailbox
}

// GetRecipients returns the recipients from the recipient table of this message.
// Returns no recipients (and no error) if the message has no recipient table or it is empty.
func (message *Message) GetRecipients() ([]Recipient, error) {
	recipientLocalDescriptor, err := FindLocalDescriptor(recipientTableIdentifier, message.LocalDescriptors)

	if eris.Is(err, ErrLocalDescriptorNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, eris.Wrap(err, "failed to find recipient local descriptor")
	}

	recipientHeapOnNode, err := message.File.GetHeapOnNodeFromLocalDescriptor(recipientLocalDescriptor)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get recipient Heap-on-Node")
	}

	recipientLocalDescriptors, err := message.File.GetLocalDescriptorsFromIdentifier(recipientLocalDescriptor.LocalDescriptorsIdentifier)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get recipient local descriptors")
	}

	recipientTableContext, err := message.File.GetTableContext(recipientHeapOnNode, recipientLocalDescriptors,
		propertyIDRecipientType, propertyIDDisplayName, propertyIDAddressType, propertyIDEmailAddress, propertyIDSMTPAddress)

	if eris.Is(err, ErrTableContextNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, eris.Wrap(err, "failed to get recipient table context")
	}

	recipients := make([]Recipient, 0, len(recipientTableContext.Properties))

	for _, row := range recipientTableContext.Properties {
		var recipient Recipient

		for _, property := range row {
			propertyReader, err := recipientTableContext.GetPropertyReader(property, recipientLocalDescriptors...)

			if eris.Is(err, ErrPropertyNoData) {
				continue
			} else if err != nil {
				return nil, eris.Wrap(err, "failed to get recipient property reader")
			}

			if property.ID == propertyIDRecipientType {
				recipientType, err := propertyReader.GetInteger32()

				if err != nil {
					return nil, eris.Wrap(err, "failed to read recipient type")
				}

				recipient.Type = RecipientType(uint32(recipientType) &^ (recipientTypeFlagP1 | recipientTypeFlagSubmitted))
				recipient.P1 = uint32(recipientType)&recipientTypeFlagP1 != 0

				continue
			}

			value, err := readString(&propertyReader)

			if eris.Is(err, ErrPropertyNoData) {
				continue
			} else if err != nil {
				return nil, eris.Wrapf(err, "failed to read recipient property: %x", property.ID)
			}

			switch property.ID {
			case propertyIDDisplayName:
				recipient.DisplayName = value
			case propertyIDAddressType:
				recipient.AddressType = value
			case propertyIDEmailAddress:
				recipient.EmailAddress = value
			case propertyIDSMTPAddress:
				recipient.SMTPAddress = value
			}
		}

		recipients = append(recipients, recipient)
	}

	return recipients, nil
}
