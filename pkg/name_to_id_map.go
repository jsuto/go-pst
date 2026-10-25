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
	"fmt"
	"strings"

	"github.com/rotisserie/eris"
	"golang.org/x/text/encoding/unicode"
)

// PropertySets defines the property sets (GUIDs) in the Name-To-ID Map.
// "Property set: A GUID that identifies a group of properties with a similar purpose."
// References [MS-OXPROPS]: "1.3.2 Commonly Used Property Sets".
var PropertySets = []string{
	"00020329-0000-0000-C000-000000000046",
	"00062008-0000-0000-C000-000000000046",
	"00062004-0000-0000-C000-000000000046",
	"00020386-0000-0000-C000-000000000046",
	"00062002-0000-0000-C000-000000000046",
	"6ED8DA90-450B-101B-98DA-00AA003F1305",
	"0006200A-0000-0000-C000-000000000046",
	"41F28F13-83F4-4114-A584-EEDB5A6B0BFF",
	"0006200E-0000-0000-C000-000000000046",
	"00062041-0000-0000-C000-000000000046",
	"00062003-0000-0000-C000-000000000046",
	"4442858E-A9E3-4E80-B900-317A210CC15B",
	"00020328-0000-0000-C000-000000000046",
	"71035549-0739-4DCB-9163-00F0580DBBDF",
	"00062040-0000-0000-C000-000000000046",
	"23239608-685D-4732-9C55-4C95CB4E8E33",
	"96357F7F-59E1-47D0-99A7-46515C183B54",
}

// PropertySet represents a collection of properties.
type PropertySet uint8

// Constants defining the commonly used property sets.
// Note that the defined order is important and reflects the PropertySets array.
const (
	PropertySetPublicStrings PropertySet = iota
	PropertySetCommon
	PropertySetAddress
	PropertySetInternetHeaders
	PropertySetAppointment
	PropertySetMeeting
	PropertySetLog
	PropertySetMessaging
	PropertySetNote
	PropertySetPostRSS
	PropertySetTask
	PropertySetUnifiedMessaging
	PropertySetMAPI
	PropertySetAirSync
	PropertySetSharing
	PropertySetXMLExtractedEntities
	PropertySetAttachment
)

// NameToIDMap represents the Name-To-ID Map.
type NameToIDMap struct {
	PropertySets []string
	NameToID     map[int64]int // Key: the property set index << 32 | the numerical name.
	IDToName     map[int]int
	StringToID   map[string]int
	IDToString   map[int]string
}

// GetNameToIDMap returns the Name-To-ID Map.
func (file *File) GetNameToIDMap() (*NameToIDMap, error) {
	nodeBTreeNode, err := file.GetNodeBTreeNode(IdentifierNameToIDMap)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get node b-tree node")
	}

	localDescriptors, err := file.GetLocalDescriptors(nodeBTreeNode)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get local descriptors")
	}

	blockBTreeNode, err := file.GetBlockBTreeNode(nodeBTreeNode.DataIdentifier)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get block b-tree node")
	}

	heapOnNode, err := file.GetHeapOnNode(blockBTreeNode)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get Heap-on-Node")
	}

	propertyContext, err := file.GetPropertyContext(heapOnNode)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get property context")
	}

	// References [MS-PST].pdf "2.4.7.2 GUID Stream"
	// The GUID Stream is a flat array of 16-byte GUID values that contains the GUIDs associated with all
	// the property sets used in all the named properties in the PST.
	guidStream, err := readNameToIDMapStream(propertyContext, 2, localDescriptors)

	if err != nil {
		return nil, eris.Wrap(err, "failed to read GUID stream")
	}

	// References [MS-PST].pdf "2.4.7.3 Entry Stream"
	// The Entry Stream is a flat array of NAMEID records that represent all the named properties in the PST.
	entryStream, err := readNameToIDMapStream(propertyContext, 3, localDescriptors)

	if err != nil {
		return nil, eris.Wrap(err, "failed to read entry stream")
	}

	// References [MS-PST].pdf "2.4.7.4 The String Stream"
	// The String Stream is a packed list of strings that is used for all the named properties in the PST.
	stringStream, err := readNameToIDMapStream(propertyContext, 4, localDescriptors)

	if err != nil {
		return nil, eris.Wrap(err, "failed to read string stream")
	}

	return parseNameToIDMap(guidStream, entryStream, stringStream)
}

// readNameToIDMapStream reads a stream of the Name-To-ID Map.
func readNameToIDMapStream(propertyContext *PropertyContext, propertyID uint16, localDescriptors []LocalDescriptor) ([]byte, error) {
	propertyReader, err := propertyContext.GetPropertyReader(propertyID, localDescriptors)

	if err != nil {
		return nil, eris.Wrap(err, "failed to get property reader")
	}

	data := make([]byte, propertyReader.Size())

	if _, err := propertyReader.ReadAt(data, 0); err != nil {
		return nil, eris.Wrap(err, "failed to read stream")
	}

	return data, nil
}

// parseNameToIDMap creates the Name-To-ID Map from its GUID, entry and string streams.
// Entries referencing data outside the streams are skipped.
// References [MS-PST].pdf "2.4.7 Named Property Lookup Map", "2.1.2 Properties"
func parseNameToIDMap(guidStream []byte, entryStream []byte, stringStream []byte) (*NameToIDMap, error) {
	propertySetToIndex := make(map[string]int, len(PropertySets))

	for i, propertySet := range PropertySets {
		propertySetToIndex[strings.ToLower(propertySet)] = i
	}

	var guids []string
	guidIndexes := make([]int, len(guidStream)/16)

	for i := range guidIndexes {
		guid := GUIDFromWindowsArray(guidStream[i*16 : (i+1)*16]).String()

		if propertySetIndex, ok := propertySetToIndex[guid]; ok {
			guids = append(guids, PropertySets[propertySetIndex])
			guidIndexes[i] = propertySetIndex
		} else {
			// Every GUID index (the GUID Stream) must map to a GUID, specify this index has no GUID.
			guidIndexes[i] = -1
		}
	}

	// Process the Entry Stream (NAMEID records).
	// References "2.4.7.1 NAMEID"
	nameToID := make(map[int64]int)
	idToName := make(map[int]int)
	stringToID := make(map[string]int)
	idToString := make(map[int]string)

	for i := 0; i+8 <= len(entryStream); i += 8 {
		namedPropertyIdentifier := binary.LittleEndian.Uint32(entryStream[i : i+4])
		namedPropertyGUID := binary.LittleEndian.Uint16(entryStream[i+4 : i+6])
		namedPropertyIdentifierType := namedPropertyGUID & 0x0001 // N: 0 for a numerical name, 1 for a string name.
		namedPropertyGUID >>= 1
		// Property index. This is the ordinal number of the named property, the named property ID is 0x8000 + index.
		namedPropertyID := int(binary.LittleEndian.Uint16(entryStream[i+6:i+8])) + 0x8000

		if namedPropertyIdentifierType == 0 {
			// The named property identifier contains the value of a numerical name.
			var propertySetIndex int

			switch {
			case namedPropertyGUID == 1:
				propertySetIndex = int(PropertySetMAPI)
			case namedPropertyGUID == 2:
				propertySetIndex = int(PropertySetPublicStrings)
			case namedPropertyGUID >= 3 && int(namedPropertyGUID)-3 < len(guidIndexes):
				propertySetIndex = guidIndexes[int(namedPropertyGUID)-3]
			default:
				// No GUID or not in the GUID Stream.
				propertySetIndex = -1
			}

			nameToID[int64(namedPropertyIdentifier)|int64(propertySetIndex)<<32] = namedPropertyID
			idToName[namedPropertyID] = int(namedPropertyIdentifier)
		} else {
			// The named property identifier is the byte offset into the String Stream
			// of the length (4 bytes) and the UTF-16 string name of the property.
			offset := int(namedPropertyIdentifier)

			if offset < 0 || offset > len(stringStream)-4 {
				continue
			}

			keyLength := int(binary.LittleEndian.Uint32(stringStream[offset : offset+4]))

			if keyLength <= 0 || keyLength > len(stringStream)-offset-4 {
				continue
			}

			utf16Decoder := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()

			key, err := utf16Decoder.String(string(stringStream[offset+4 : offset+4+keyLength]))

			if err != nil {
				return nil, eris.Wrap(err, "failed to decode UTF-16")
			}

			stringToID[key] = namedPropertyID
			idToString[namedPropertyID] = key
		}
	}

	return &NameToIDMap{
		PropertySets: guids,
		NameToID:     nameToID,
		IDToName:     idToName,
		StringToID:   stringToID,
		IDToString:   idToString,
	}, nil
}

// GetPropertyID returns the Name-To-ID property ID.
func (nameToIDMap *NameToIDMap) GetPropertyID(key int, propertySet PropertySet) (int, error) {
	nameToIDKey := int64(propertySet)<<32 | int64(key)

	value, found := nameToIDMap.NameToID[nameToIDKey]

	if !found {
		return -1, ErrNameToIDMapKeyNotFound
	}

	return value, nil
}

// GUID represents a GUID/UUID. It has the same structure as
// golang.org/x/sys/windows.GUID so that it can be used with functions expecting
// that type. It is defined as its own type as that is only available to builds
// targeted at `windows`. The representation matches that used by native Windows
// code.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// GUIDFromWindowsArray constructs a GUID from a Windows encoding array of bytes.
// References https://github.com/microsoft/go-winio/blob/master/pkg/guid/guid.go
func GUIDFromWindowsArray(b []byte) GUID {
	var g GUID

	g.Data1 = binary.LittleEndian.Uint32(b[0:4])
	g.Data2 = binary.LittleEndian.Uint16(b[4:6])
	g.Data3 = binary.LittleEndian.Uint16(b[6:8])

	copy(g.Data4[:], b[8:16])

	return g
}

func (g GUID) String() string {
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		g.Data1,
		g.Data2,
		g.Data3,
		g.Data4[:2],
		g.Data4[2:])
}
