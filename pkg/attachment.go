// go-pst is a library for reading Personal Storage Table (.pst) files (written in Go/Golang).
//
// Copyright 2023 Marten Mooij
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
	"encoding/binary"
	"github.com/jsuto/go-pst/pkg/properties"
	"io"

	"github.com/rotisserie/eris"
)

// Attachment represents a message attachment.
type Attachment struct {
	File             *File
	Identifier       Identifier
	PropertyContext  *PropertyContext
	LocalDescriptors []LocalDescriptor
	properties.Attachment
}

// HasAttachments returns true if this message has attachments.
func (message *Message) HasAttachments() (bool, error) {
	reader, err := message.PropertyContext.GetPropertyReader(3591, message.LocalDescriptors)

	if err != nil {
		return false, eris.Wrap(err, "failed to get property reader")
	}

	value, err := reader.GetInteger32()

	if err != nil {
		return false, eris.Wrap(err, "failed to read int32")
	}

	return value&0x10 != 0, nil
}

// GetAttachmentTableContext returns the table context of the attachments of this message.
// Note we only return the attachment identifier property.
func (message *Message) GetAttachmentTableContext() (*TableContext, error) {
	hasAttachments, err := message.HasAttachments()

	if err != nil {
		return nil, eris.Wrap(err, "failed to check if there are attachments")
	}

	if !hasAttachments {
		return nil, ErrAttachmentsNotFound
	}

	if message.AttachmentTableContext == nil {
		// Initialize the attachments table context.
		attachmentLocalDescriptor, err := FindLocalDescriptor(1649, message.LocalDescriptors)

		if err != nil {
			return nil, eris.Wrap(err, "failed to find attachment local descriptor")
		}

		attachmentHeapOnNode, err := message.File.GetHeapOnNodeFromLocalDescriptor(attachmentLocalDescriptor)

		if err != nil {
			return nil, eris.Wrap(err, "failed to get attachment Heap-on-Node")
		}

		attachmentLocalDescriptors, err := message.File.GetLocalDescriptorsFromIdentifier(attachmentLocalDescriptor.LocalDescriptorsIdentifier)

		if err != nil {
			return nil, eris.Wrap(err, "failed to get attachment local descriptors")
		}

		attachmentTableContext, err := message.File.GetTableContext(attachmentHeapOnNode, attachmentLocalDescriptors, 26610)

		if eris.Is(err, ErrTableContextNoRows) {
			// The message is flagged as having attachments, but the attachment table is empty.
			return nil, ErrAttachmentsNotFound
		} else if err != nil {
			return nil, eris.Wrap(err, "failed to get attachment table context")
		}

		message.AttachmentTableContext = &attachmentTableContext
	}

	return message.AttachmentTableContext, nil
}

// GetAttachmentCount returns the amount of rows in the attachment table context.
func (message *Message) GetAttachmentCount() (int, error) {
	attachmentTableContext, err := message.GetAttachmentTableContext()

	if eris.Is(err, ErrAttachmentsNotFound) {
		return 0, nil
	} else if err != nil {
		return 0, eris.Wrap(err, "failed to get attachment table context")
	}

	return len(attachmentTableContext.Properties), nil
}

// GetAttachment returns the specified attachment.
func (message *Message) GetAttachment(attachmentIndex int) (*Attachment, error) {
	attachmentsTableContext, err := message.GetAttachmentTableContext()

	if err != nil {
		return nil, eris.Wrap(err, "failed to get attachments table context")
	} else if attachmentIndex > len(attachmentsTableContext.Properties)-1 {
		return nil, ErrAttachmentIndexInvalid
	}

	var attachmentHNID Identifier

	for _, attachmentProperty := range attachmentsTableContext.Properties[attachmentIndex] {
		// We only get the attachment identifier property from GetAttachmentTableContext.
		propertyReader, err := attachmentsTableContext.GetPropertyReader(attachmentProperty)

		if err != nil {
			return nil, eris.Wrap(err, "failed to get attachments table context property reader")
		}

		identifier, err := propertyReader.GetInteger32()

		if err != nil {
			return nil, eris.Wrap(err, "failed to read identifier")
		}

		attachmentHNID = Identifier(identifier)
	}

	if attachmentHNID == 0 {
		return nil, eris.New("failed to get attachment HNID")
	}

	attachmentLocalDescriptor, err := FindLocalDescriptor(attachmentHNID, message.LocalDescriptors)

	if err != nil {
		return nil, eris.Wrap(err, "failed to find attachment local descriptor")
	}

	attachmentLocalDescriptors, err := message.File.GetLocalDescriptorsFromIdentifier(attachmentLocalDescriptor.LocalDescriptorsIdentifier)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get local descriptors from identifier")
	}

	attachmentHeapOnNode, err := message.File.GetHeapOnNodeFromLocalDescriptor(attachmentLocalDescriptor)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get attachment Heap-on-Node")
	}

	attachmentPropertyContext, err := message.File.GetPropertyContext(attachmentHeapOnNode)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get attachment property context")
	}

	attachment := &Attachment{
		File:             message.File,
		Identifier:       attachmentLocalDescriptor.Identifier,
		PropertyContext:  attachmentPropertyContext,
		LocalDescriptors: attachmentLocalDescriptors,
	}

	if err := attachmentPropertyContext.Populate(attachment, attachmentLocalDescriptors); err != nil {
		return nil, eris.Wrap(err, "failed to populate attachment property context")
	}

	return attachment, nil
}

// GetAttachment returns the attachment.
// Note that the properties aren't populated (call PropertyContext.Populate).
func (file *File) GetAttachment(messageIdentifier Identifier) (*Attachment, error) {
	attachmentsNode, err := file.GetNodeBTreeNode(messageIdentifier)

	if err != nil {
		return nil, eris.Wrap(err, "failed to find node b-tree node")
	}

	attachmentsDataNode, err := file.GetBlockBTreeNode(attachmentsNode.DataIdentifier)

	if err != nil {
		return nil, eris.Wrap(err, "failed to find block b-tree node")
	}

	attachmentsHeapOnNode, err := file.GetHeapOnNode(attachmentsDataNode)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get Heap-on-Node")
	}

	localDescriptors, err := file.GetLocalDescriptors(attachmentsNode)

	if err != nil {
		return nil, eris.Wrap(err, "failed to find local descriptors")
	}

	propertyContext, err := file.GetPropertyContext(attachmentsHeapOnNode)

	attachment := &Attachment{
		File:             file,
		Identifier:       messageIdentifier,
		PropertyContext:  propertyContext,
		LocalDescriptors: localDescriptors,
	}

	if err := propertyContext.Populate(attachment, localDescriptors); err != nil {
		return nil, eris.Wrap(err, "failed to populate attachment property context")
	}

	return attachment, nil
}

// GetAllAttachments returns the attachments of this message.
// See AttachmentIterator.
func (message *Message) GetAllAttachments() ([]*Attachment, error) {
	attachmentCount, err := message.GetAttachmentCount()

	if eris.Is(err, ErrAttachmentsNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, eris.Wrap(err, "failed to get attachment count")
	}

	attachments := make([]*Attachment, attachmentCount)

	for i := 0; i < attachmentCount; i++ {
		attachment, err := message.GetAttachment(i)

		if err != nil {
			return nil, eris.Wrap(err, "failed to get attachment")
		}

		attachments[i] = attachment
	}

	return attachments, nil
}

// AttachmentIterator implements an attachment iterator.
type AttachmentIterator struct {
	message *Message

	err               error
	currentIndex      int
	currentAttachment *Attachment
}

// Err return the error cause.
func (attachmentIterator *AttachmentIterator) Err() error {
	return attachmentIterator.err
}

// Next will ensure that Value returns the next item when executed.
// If the next value is not retrievable, Next will return false and Err() will return the error cause.
func (attachmentIterator *AttachmentIterator) Next() bool {
	hasNext := len(attachmentIterator.message.AttachmentTableContext.Properties) > attachmentIterator.currentIndex

	if !hasNext {
		return false
	}

	attachment, err := attachmentIterator.message.GetAttachment(attachmentIterator.currentIndex)

	if err != nil {
		attachmentIterator.err = eris.Wrap(err, "failed to get attachment")
		return false
	}

	attachmentIterator.currentIndex++
	attachmentIterator.currentAttachment = attachment

	return true
}

// Value returns the current value in the iterator.
func (attachmentIterator *AttachmentIterator) Value() *Attachment {
	return attachmentIterator.currentAttachment
}

// Size returns the amount of attachments in the message iterator.
func (attachmentIterator *AttachmentIterator) Size() int {
	return len(attachmentIterator.message.AttachmentTableContext.Properties)
}

func (attachmentIterator *AttachmentIterator) CurrentIndex() int {
	return attachmentIterator.currentIndex
}

// GetAttachmentIterator returns an iterator for attachments.
func (message *Message) GetAttachmentIterator() (AttachmentIterator, error) {
	attachmentCount, err := message.GetAttachmentCount()

	// TODO - Return an empty iterator instead of an error.
	if err != nil {
		return AttachmentIterator{}, eris.Wrap(err, "failed to get attachment count")
	} else if attachmentCount == 0 {
		return AttachmentIterator{}, ErrAttachmentsNotFound
	}

	return AttachmentIterator{
		message: message,
	}, nil
}

// Constants defining the attachment methods (PR_ATTACH_METHOD).
// References [MS-OXCMSG]: 2.2.2.9 PidTagAttachMethod.
const (
	AttachMethodNone            = 0
	AttachMethodByValue         = 1 // The data is in PR_ATTACH_DATA_BIN, see WriteTo.
	AttachMethodByReference     = 2
	AttachMethodByReferenceOnly = 4
	AttachMethodEmbeddedMessage = 5 // See GetEmbeddedMessage.
	AttachMethodOLE             = 6
)

// Constants defining the attachment flags (PR_ATTACH_FLAGS).
// References [MS-OXCMSG]: 2.2.2.18 PidTagAttachFlags.
const (
	AttachFlagInvisibleInHTML = 0x1
	AttachFlagInvisibleInRTF  = 0x2
	AttachFlagRenderedInBody  = 0x4 // ATT_MHTML_REF: referenced by the HTML body (Content-ID or Content-Location).
)

// propertyIDAttachDataObject is PR_ATTACH_DATA_OBJ, the same ID as PR_ATTACH_DATA_BIN but of type PtypObject.
const propertyIDAttachDataObject = 0x3701

// IsEmbeddedMessage returns true if the attachment is a message (forwarded as attachment, for example).
func (attachment *Attachment) IsEmbeddedMessage() bool {
	return attachment.GetAttachMethod() == AttachMethodEmbeddedMessage
}

// IsInline returns true if the attachment is marked as rendered in the body:
// ATT_MHTML_REF in PR_ATTACH_FLAGS, or hidden (PR_ATTACHMENT_HIDDEN) and having a Content-ID.
// Whether the HTML body actually references the Content-ID is up to the caller.
func (attachment *Attachment) IsInline() bool {
	return attachment.GetAttachFlags()&AttachFlagRenderedInBody != 0 ||
		(attachment.GetAttachmentHidden() && attachment.GetAttachContentId() != "")
}

// GetEmbeddedMessage returns the message of an attachment with PR_ATTACH_METHOD afEmbeddedMessage.
// The message can be used like any other: its recipients, attachments (also embedded messages) and properties.
// References [MS-PST]: 2.3.3.5 PtypObject Properties, 2.4.6.2 Attachment Object PC.
func (attachment *Attachment) GetEmbeddedMessage() (*Message, error) {
	if !attachment.IsEmbeddedMessage() {
		return nil, ErrAttachmentNotEmbeddedMessage
	}

	propertyReader, err := attachment.PropertyContext.GetPropertyReader(propertyIDAttachDataObject, attachment.LocalDescriptors)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get attachment data object property reader")
	} else if propertyReader.Property.Type != PropertyTypeObject {
		return nil, ErrPropertyTypeMismatch
	}

	// The value is the subnode identifier of the embedded message (4 bytes) followed by its size (4 bytes).
	objectReference := make([]byte, 4)

	if propertyReader.Size() < 4 {
		return nil, ErrPropertyDataTooShort
	} else if _, err := propertyReader.ReadAt(objectReference, 0); err != nil {
		return nil, eris.Wrap(err, "failed to read attachment data object")
	}

	messageIdentifier := Identifier(binary.LittleEndian.Uint32(objectReference))

	messageLocalDescriptor, err := FindLocalDescriptor(messageIdentifier, attachment.LocalDescriptors)

	if err != nil {
		return nil, eris.Wrap(err, "failed to find embedded message local descriptor")
	}

	messageHeapOnNode, err := attachment.File.GetHeapOnNodeFromLocalDescriptor(messageLocalDescriptor)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get embedded message Heap-on-Node")
	}

	messageLocalDescriptors, err := attachment.File.GetLocalDescriptorsFromIdentifier(messageLocalDescriptor.LocalDescriptorsIdentifier)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get embedded message local descriptors")
	}

	messagePropertyContext, err := attachment.File.GetPropertyContext(messageHeapOnNode)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get embedded message property context")
	}

	return attachment.File.newMessage(messageIdentifier, messagePropertyContext, messageLocalDescriptors)
}

// WriteTo writes the attachment data (PR_ATTACH_DATA_BIN) to the specified io.Writer.
// Embedded messages have no data, see GetEmbeddedMessage.
func (attachment *Attachment) WriteTo(writer io.Writer) (int64, error) {
	if attachment.IsEmbeddedMessage() {
		return 0, ErrAttachmentEmbeddedMessage
	}

	attachmentReader, err := attachment.PropertyContext.GetPropertyReader(14081, attachment.LocalDescriptors)

	if eris.Is(err, ErrPropertyNoData) {
		return 0, nil
	} else if err != nil {
		return 0, eris.Wrap(err, "failed to get attachment property reader")
	}

	sectionReader := io.NewSectionReader(&attachmentReader, 0, attachmentReader.Size())

	written, err := io.CopyN(writer, sectionReader, sectionReader.Size())

	if err != nil {
		return written, eris.Wrap(err, "failed to write attachment")
	}

	return written, nil
}
