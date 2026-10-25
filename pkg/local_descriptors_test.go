package pst_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	pst "github.com/jsuto/go-pst/pkg"
)

// subnodeFile builds an in-memory file with subnode blocks (SLBLOCK/SIBLOCK) and the block b-tree referencing them.
type subnodeFile struct {
	formatType pst.FormatType
	data       bytes.Buffer
	blockBTree pst.BTreeStore
}

func newSubnodeFile(formatType pst.FormatType) *subnodeFile {
	return &subnodeFile{formatType: formatType, blockBTree: pst.NewBTreeStoreInMemory()}
}

// putIdentifier writes a NID or BID: 4 bytes in ANSI files, 8 bytes in Unicode files.
func (subnodeFile *subnodeFile) putIdentifier(block []byte, identifier uint32) []byte {
	if subnodeFile.formatType == pst.FormatTypeANSI {
		return binary.LittleEndian.AppendUint32(block, identifier)
	}

	return binary.LittleEndian.AppendUint64(block, uint64(identifier))
}

// addBlock adds a subnode block with the given level and entries (each entry is a list of identifiers).
func (subnodeFile *subnodeFile) addBlock(blockIdentifier uint32, level uint8, entries ...[]uint32) {
	block := []byte{2, level}
	block = binary.LittleEndian.AppendUint16(block, uint16(len(entries)))

	if subnodeFile.formatType != pst.FormatTypeANSI {
		block = append(block, 0, 0, 0, 0) // dwPadding
	}

	for _, entry := range entries {
		for _, identifier := range entry {
			block = subnodeFile.putIdentifier(block, identifier)
		}
	}

	subnodeFile.blockBTree.Load(pst.BTreeNode{
		Identifier: pst.Identifier(blockIdentifier),
		FileOffset: int64(subnodeFile.data.Len()),
		Size:       uint16(len(block)),
	})

	subnodeFile.data.Write(block)
	subnodeFile.data.Write(make([]byte, 64-len(block)%64)) // Blocks are 64-byte aligned.
}

func (subnodeFile *subnodeFile) file() *pst.File {
	return &pst.File{
		FormatType: subnodeFile.formatType,
		Reader:     bytes.NewReader(subnodeFile.data.Bytes()),
		BlockBTree: subnodeFile.blockBTree,
	}
}

func TestLocalDescriptorsSIBlock(t *testing.T) {
	for _, formatType := range []pst.FormatType{pst.FormatTypeUnicode, pst.FormatTypeANSI} {
		subnodeFile := newSubnodeFile(formatType)

		// SLBLOCKs with SLENTRY (nid, bidData, bidSub).
		subnodeFile.addBlock(0x100, 0, []uint32{0x21, 0x1000, 0}, []uint32{0x41, 0x1004, 0})
		subnodeFile.addBlock(0x104, 0, []uint32{0x692, 0x1008, 0x100C})
		// SIBLOCK with SIENTRY (nid, bid) referencing the SLBLOCKs.
		subnodeFile.addBlock(0x108, 1, []uint32{0x21, 0x100}, []uint32{0x692, 0x104})
		// SIBLOCK referencing a SIBLOCK, and a level 2 block.
		subnodeFile.addBlock(0x10C, 1, []uint32{0x21, 0x108})
		subnodeFile.addBlock(0x110, 2, []uint32{0x21, 0x108})

		file := subnodeFile.file()

		localDescriptors, err := file.GetLocalDescriptorsFromIdentifier(0x108)

		if err != nil {
			t.Fatalf("%d: Failed to get local descriptors: %+v", formatType, err)
		}

		want := []pst.LocalDescriptor{
			{Identifier: 0x21, DataIdentifier: 0x1000},
			{Identifier: 0x41, DataIdentifier: 0x1004},
			{Identifier: 0x692, DataIdentifier: 0x1008, LocalDescriptorsIdentifier: 0x100C},
		}

		if len(localDescriptors) != len(want) {
			t.Fatalf("%d: got %+v, want %+v", formatType, localDescriptors, want)
		}

		for i := range want {
			if localDescriptors[i] != want[i] {
				t.Errorf("%d: local descriptor %d: got %+v, want %+v", formatType, i, localDescriptors[i], want[i])
			}
		}

		for _, blockIdentifier := range []pst.Identifier{0x10C, 0x110} {
			if _, err := file.GetLocalDescriptorsFromIdentifier(blockIdentifier); !errors.Is(err, pst.ErrLocalDescriptorBranchNode) {
				t.Errorf("%d: block 0x%X: got %v, want %v", formatType, blockIdentifier, err, pst.ErrLocalDescriptorBranchNode)
			}
		}
	}
}

func TestLocalDescriptorsEntryCountInvalid(t *testing.T) {
	subnodeFile := newSubnodeFile(pst.FormatTypeUnicode)

	subnodeFile.addBlock(0x100, 0, []uint32{0x21, 0x1000, 0})

	// Claim more entries than the block holds.
	block, _ := subnodeFile.blockBTree.Get(pst.BTreeNode{Identifier: 0x100})
	binary.LittleEndian.PutUint16(subnodeFile.data.Bytes()[block.FileOffset+2:], 1000)

	if _, err := subnodeFile.file().GetLocalDescriptorsFromIdentifier(0x100); !errors.Is(err, pst.ErrLocalDescriptorsEntryCountInvalid) {
		t.Errorf("got %v, want %v", err, pst.ErrLocalDescriptorsEntryCountInvalid)
	}
}
