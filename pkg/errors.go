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
	"errors"
)

// Constants defining go-pst errors.
// We use stack-traces (Wrap) via https://github.com/rotisserie/eris
var (
	ErrFileSignatureInvalid              = errors.New("go-pst: invalid file signature")
	ErrFormatTypeUnsupported             = errors.New("go-pst: unsupported format type")
	ErrEncryptionTypeUnsupported         = errors.New("go-pst: unsupported encryption type")
	ErrContentTypeUnsupported            = errors.New("go-pst: unsupported content type")
	ErrMessageIdentifierTypeInvalid      = errors.New("go-pst: invalid message identifier type")
	ErrPropertyNotFound                  = errors.New("go-pst: failed to find property by ID")
	ErrPropertyContextInvalid            = errors.New("go-pst: invalid property context b-tree-on-heap header")
	ErrTableTypeInvalid                  = errors.New("go-pst: invalid table type")
	ErrBTreeNodeConflict                 = errors.New("go-pst: conflicting b-tree node entry")
	ErrBTreeNodeInvalid                  = errors.New("go-pst: invalid b-tree page")
	ErrBTreeNodeNotFound                 = errors.New("go-pst: failed to find b-tree node")
	ErrHeapOnNodeAllocationInvalid       = errors.New("go-pst: Heap-on-Node allocation outside its block")
	ErrHeapOnNodeExternalNode            = errors.New("go-pst: external node, no local descriptors provided")
	ErrTableSignatureInvalid             = errors.New("go-pst: invalid table signature")
	ErrTableContextNoColumns             = errors.New("go-pst: there are no columns in this table context")
	ErrTableContextInvalid               = errors.New("go-pst: table context rows or columns outside the row matrix")
	ErrTableContextNoRows                = errors.New("go-pst: there are no rows in this table context")
	ErrBlockTypeInvalid                  = errors.New("go-pst: unsupported block type")
	ErrBlockSignatureInvalid             = errors.New("go-pst: invalid block signature")
	ErrAttachmentIndexInvalid            = errors.New("go-pst: invalid attachment index, there are no more attachments")
	ErrLocalDescriptorsSignatureInvalid  = errors.New("go-pst: invalid local descriptors signature")
	ErrLocalDescriptorNotFound           = errors.New("go-pst: failed to find local descriptor")
	ErrLocalDescriptorBranchNode         = errors.New("go-pst: local descriptors nested deeper than allowed")
	ErrLocalDescriptorsEntryCountInvalid = errors.New("go-pst: local descriptors entries exceed the block")
	ErrPropertyTypeMismatch              = errors.New("go-pst: property type is not the same as the value expected from the caller")
	ErrPropertyNoData                    = errors.New("go-pst: property has no data")
	ErrPropertyDataTooShort              = errors.New("go-pst: property data is shorter than its type")
	ErrNameToIDMapKeyNotFound            = errors.New("go-pst: failed to find key in Name-To-ID Map")
	ErrMessagesNotFound                  = errors.New("go-pst: folder has no messages")
	ErrAttachmentsNotFound               = errors.New("go-pst: message has no attachments")
	ErrAttachmentNotEmbeddedMessage      = errors.New("go-pst: attachment is not an embedded message")
	ErrAttachmentEmbeddedMessage         = errors.New("go-pst: attachment is an embedded message, it has no data")
	ErrBlockIndexNotFound                = errors.New("go-pst: block index not found")
	ErrFolderStructureInvalid            = errors.New("go-pst: folder referenced twice or nested too deep")
	ErrRTFInvalid                        = errors.New("go-pst: invalid compressed RTF")
	ErrTotalBlocksSizeMismatch           = errors.New("go-pst: block total size mismatch")
)
