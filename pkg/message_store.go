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

	"github.com/rotisserie/eris"
)

// GetMessageStore returns the message store of the PST file.
func (file *File) GetMessageStore() (*PropertyContext, error) {
	dataBTreeNode, err := file.GetDataBTreeNode(IdentifierMessageStore)

	if err != nil {
		return nil, eris.Wrapf(err, "failed to find data b-tree node")
	}

	heapOnNode, err := file.GetHeapOnNode(dataBTreeNode)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get Heap-on-Node")
	}

	return file.GetPropertyContext(heapOnNode)
}

// Property IDs of the message store referencing special folders.
// References [MS-PST]: 2.4.3 Message Store, [MS-OXCSTOR].
const (
	propertyIDIPMSubtreeEntryID     = 0x35E0 // PR_IPM_SUBTREE_ENTRYID: top of the folders of the user.
	propertyIDIPMWastebasketEntryID = 0x35E3 // PR_IPM_WASTEBASKET_ENTRYID: Deleted Items.
)

// GetIPMSubtreeIdentifier returns the identifier of the folder at the top of the folders of the user
// (shown as the PST file in Outlook, its sub-folders are Inbox, Sent Items...).
func (file *File) GetIPMSubtreeIdentifier() (Identifier, error) {
	return file.getMessageStoreFolderIdentifier(propertyIDIPMSubtreeEntryID)
}

// GetDeletedItemsIdentifier returns the identifier of the Deleted Items folder.
func (file *File) GetDeletedItemsIdentifier() (Identifier, error) {
	return file.getMessageStoreFolderIdentifier(propertyIDIPMWastebasketEntryID)
}

// getMessageStoreFolderIdentifier returns the folder identifier of an entry ID property of the message store.
// A PST entry ID is 4 bytes of flags, the 16 bytes of the provider UID and the 4 bytes of the NID.
// References [MS-PST]: 2.4.3.2 Mapping between EntryID and NID.
func (file *File) getMessageStoreFolderIdentifier(propertyID uint16) (Identifier, error) {
	messageStore, err := file.GetMessageStore()

	if err != nil {
		return 0, eris.Wrap(err, "failed to get message store")
	}

	propertyReader, err := messageStore.GetPropertyReader(propertyID, nil)

	if err != nil {
		return 0, eris.Wrap(err, "failed to get entry ID property reader")
	} else if propertyReader.Property.Type != PropertyTypeBinary {
		return 0, ErrPropertyTypeMismatch
	} else if propertyReader.Size() != 24 {
		return 0, ErrPropertyDataTooShort
	}

	entryID := make([]byte, 24)

	if _, err := propertyReader.ReadAt(entryID, 0); err != nil {
		return 0, eris.Wrap(err, "failed to read entry ID")
	}

	return Identifier(binary.LittleEndian.Uint32(entryID[20:24])), nil
}
