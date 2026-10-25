package pst

import (
	"encoding/binary"
	"testing"
)

// nameIDEntry returns a NAMEID record of the Entry Stream.
func nameIDEntry(identifier uint32, guidIndex uint16, isString bool, propertyIndex uint16) []byte {
	entry := make([]byte, 8)

	binary.LittleEndian.PutUint32(entry[0:4], identifier)

	guid := guidIndex << 1

	if isString {
		guid |= 1
	}

	binary.LittleEndian.PutUint16(entry[4:6], guid)
	binary.LittleEndian.PutUint16(entry[6:8], propertyIndex)

	return entry
}

func TestParseNameToIDMap(t *testing.T) {
	// PSETID_Common (00062008-0000-0000-C000-000000000046) in the GUID Stream, in Windows byte order.
	guidStream := []byte{0x08, 0x20, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}
	// "x-test" (UTF-16LE) with its length.
	stringStream := []byte{12, 0, 0, 0, 'x', 0, '-', 0, 't', 0, 'e', 0, 's', 0, 't', 0}

	var entryStream []byte

	for _, entry := range [][]byte{
		nameIDEntry(0x8580, 3, false, 0),  // PidLidInternetAccountName in the first GUID of the GUID Stream.
		nameIDEntry(0, 2, true, 1),        // "x-test" in PS_PUBLIC_STRINGS.
		nameIDEntry(0x1234, 0, false, 2),  // No GUID.
		nameIDEntry(0x1235, 9, false, 3),  // GUID index outside the GUID Stream.
		nameIDEntry(1000, 2, true, 4),     // String offset outside the String Stream.
		nameIDEntry(2, 2, true, 5),        // String length outside the String Stream.
		nameIDEntry(0x10000, 1, false, 6), // Numerical name larger than 16 bits.
	} {
		entryStream = append(entryStream, entry...)
	}

	nameToIDMap, err := parseNameToIDMap(guidStream, entryStream, stringStream)

	if err != nil {
		t.Fatalf("Failed to parse Name-To-ID Map: %+v", err)
	}

	if id, err := nameToIDMap.GetPropertyID(0x8580, PropertySetCommon); err != nil || id != 0x8000 {
		t.Errorf("PidLidInternetAccountName: got 0x%X, %v, want 0x8000", id, err)
	}

	if id, ok := nameToIDMap.StringToID["x-test"]; !ok || id != 0x8001 {
		t.Errorf("x-test: got 0x%X, %v, want 0x8001", id, ok)
	}

	if name := nameToIDMap.IDToName[0x8006]; name != 0x10000 {
		t.Errorf("numerical name larger than 16 bits: got 0x%X, want 0x10000", name)
	}

	if len(nameToIDMap.IDToString) != 1 {
		t.Errorf("got string names %v, want only x-test", nameToIDMap.IDToString)
	}
}

// FuzzParseNameToIDMap checks that malformed streams don't panic.
func FuzzParseNameToIDMap(f *testing.F) {
	f.Add([]byte{}, nameIDEntry(0x8580, 3, false, 0), []byte{})
	f.Add(make([]byte, 16), nameIDEntry(0, 2, true, 1), []byte{2, 0, 0, 0, 'x', 0})

	f.Fuzz(func(t *testing.T, guidStream []byte, entryStream []byte, stringStream []byte) {
		_, _ = parseNameToIDMap(guidStream, entryStream, stringStream)
	})
}
