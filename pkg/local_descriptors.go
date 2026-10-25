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

// LocalDescriptor represents an item in the local descriptors.
// A local descriptor is basically a reference to a node which contains the data.
type LocalDescriptor struct {
	Identifier                 Identifier
	DataIdentifier             Identifier
	LocalDescriptorsIdentifier Identifier
}

// NewLocalDescriptor creates a new local descriptor.
func NewLocalDescriptor(data []byte, formatType FormatType) LocalDescriptor {
	switch formatType {
	case FormatTypeANSI:
		return LocalDescriptor{
			Identifier:                 Identifier(binary.LittleEndian.Uint32(data[:4])),
			DataIdentifier:             Identifier(binary.LittleEndian.Uint32(data[4 : 4+4])),
			LocalDescriptorsIdentifier: Identifier(binary.LittleEndian.Uint32(data[8 : 8+4])),
		}
	default:
		// TODO - Reference [MS-PDF] that this is actually 32-bit.
		return LocalDescriptor{
			Identifier:                 Identifier(binary.LittleEndian.Uint32(data[:8])),
			DataIdentifier:             Identifier(binary.LittleEndian.Uint32(data[8 : 8+8])),
			LocalDescriptorsIdentifier: Identifier(binary.LittleEndian.Uint32(data[16 : 16+8])),
		}
	}
}

// GetLocalDescriptors returns the local descriptors of the b-tree node.
func (file *File) GetLocalDescriptors(btreeNodeEntry BTreeNode) ([]LocalDescriptor, error) {
	return file.GetLocalDescriptorsFromIdentifier(btreeNodeEntry.LocalDescriptorsIdentifier)
}

// GetLocalDescriptorsFromIdentifier returns the local descriptors of the local descriptors identifier.
// References "Local Descriptors", [MS-PST]: 2.2.2.8.3.3 Subnode BTree.
func (file *File) GetLocalDescriptorsFromIdentifier(localDescriptorsIdentifier Identifier) ([]LocalDescriptor, error) {
	return file.getLocalDescriptors(localDescriptorsIdentifier, 1)
}

// getLocalDescriptors reads a SLBLOCK (level 0) or a SIBLOCK (level 1, its entries reference SLBLOCKs).
// maxLevel limits the nesting, so malformed files can't recurse.
func (file *File) getLocalDescriptors(localDescriptorsIdentifier Identifier, maxLevel uint8) ([]LocalDescriptor, error) {
	if localDescriptorsIdentifier == 0 {
		// There are no local descriptors.
		return nil, nil
	}

	localDescriptorsNode, err := file.GetBlockBTreeNode(localDescriptorsIdentifier)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get local descriptors node")
	}

	// Block type (btype), level (cLevel) and entry count (cEnt).
	header := make([]byte, 4)

	if _, err = file.Reader.ReadAt(header, localDescriptorsNode.FileOffset); err != nil {
		return nil, eris.Wrap(err, "failed to read local descriptors header")
	} else if header[0] != 2 {
		return nil, ErrLocalDescriptorsSignatureInvalid
	}

	localDescriptorsLevel := header[1]
	localDescriptorsEntryCount := int(binary.LittleEndian.Uint16(header[2:4]))

	if localDescriptorsLevel > maxLevel {
		return nil, ErrLocalDescriptorBranchNode
	}

	var localDescriptorEntrySize int
	var localDescriptorsEntriesOffset int64

	switch {
	case file.FormatType == FormatTypeANSI && localDescriptorsLevel == 0:
		localDescriptorEntrySize = 12 // SLENTRY: nid, bidData, bidSub.
	case file.FormatType == FormatTypeANSI:
		localDescriptorEntrySize = 8 // SIENTRY: nid, bid.
	case localDescriptorsLevel == 0:
		localDescriptorEntrySize = 24
	default:
		localDescriptorEntrySize = 16
	}

	switch file.FormatType {
	case FormatTypeANSI:
		localDescriptorsEntriesOffset = localDescriptorsNode.FileOffset + 4
	default:
		// Followed by 4 bytes of padding (dwPadding).
		localDescriptorsEntriesOffset = localDescriptorsNode.FileOffset + 8
	}

	if localDescriptorsNode.Size != 0 && int64(localDescriptorsEntryCount*localDescriptorEntrySize) > int64(localDescriptorsNode.Size)-(localDescriptorsEntriesOffset-localDescriptorsNode.FileOffset) {
		return nil, ErrLocalDescriptorsEntryCountInvalid
	}

	localDescriptorsEntries := make([]byte, localDescriptorsEntryCount*localDescriptorEntrySize)

	if _, err := file.Reader.ReadAt(localDescriptorsEntries, localDescriptorsEntriesOffset); err != nil {
		return nil, eris.Wrap(err, "failed to read local descriptors entries")
	}

	if localDescriptorsLevel > 0 {
		// SIBLOCK: each entry references a SLBLOCK with the local descriptors.
		var localDescriptors []LocalDescriptor

		for i := 0; i < localDescriptorsEntryCount; i++ {
			localDescriptorEntry := localDescriptorsEntries[i*localDescriptorEntrySize : (i+1)*localDescriptorEntrySize]

			var blockIdentifier Identifier

			switch file.FormatType {
			case FormatTypeANSI:
				blockIdentifier = Identifier(binary.LittleEndian.Uint32(localDescriptorEntry[4:8]))
			default:
				blockIdentifier = Identifier(binary.LittleEndian.Uint32(localDescriptorEntry[8:16]))
			}

			childLocalDescriptors, err := file.getLocalDescriptors(blockIdentifier, localDescriptorsLevel-1)

			if err != nil {
				return nil, eris.Wrap(err, "failed to get local descriptors of SIBLOCK entry")
			}

			localDescriptors = append(localDescriptors, childLocalDescriptors...)
		}

		return localDescriptors, nil
	}

	localDescriptors := make([]LocalDescriptor, localDescriptorsEntryCount)

	for i := 0; i < localDescriptorsEntryCount; i++ {
		localDescriptorEntry := localDescriptorsEntries[i*localDescriptorEntrySize : (i+1)*localDescriptorEntrySize]

		localDescriptors[i] = NewLocalDescriptor(localDescriptorEntry, file.FormatType)
	}

	return localDescriptors, nil
}

// FindLocalDescriptor returns the local descriptor with the specified identifier or an error if not found.
func FindLocalDescriptor(identifier Identifier, localDescriptors []LocalDescriptor) (LocalDescriptor, error) {
	for _, localDescriptor := range localDescriptors {
		if localDescriptor.Identifier == identifier {
			return localDescriptor, nil
		}
	}

	return LocalDescriptor{}, ErrLocalDescriptorNotFound
}
