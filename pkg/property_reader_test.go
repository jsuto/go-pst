package pst_test

import (
	"errors"
	"testing"
	"time"

	pst "github.com/jsuto/go-pst/pkg"
)

// TestPropertyReaderShortData checks that fixed-size values shorter than their type return an error instead of panicking.
// The size of Table Context values comes from the column descriptor in the file.
func TestPropertyReaderShortData(t *testing.T) {
	for name, get := range map[string]func(pst.PropertyReader) error{
		"integer16": func(reader pst.PropertyReader) error {
			reader.Property.Type = pst.PropertyTypeInteger16
			_, err := reader.GetInteger16()
			return err
		},
		"integer32": func(reader pst.PropertyReader) error {
			reader.Property.Type = pst.PropertyTypeInteger32
			_, err := reader.GetInteger32()
			return err
		},
		"integer64": func(reader pst.PropertyReader) error {
			reader.Property.Type = pst.PropertyTypeInteger64
			_, err := reader.GetInteger64()
			return err
		},
		"time": func(reader pst.PropertyReader) error {
			reader.Property.Type = pst.PropertyTypeTime
			_, err := reader.GetDate()
			return err
		},
	} {
		if err := get(pst.PropertyReader{Property: pst.Property{Data: []byte{1}}}); !errors.Is(err, pst.ErrPropertyDataTooShort) {
			t.Errorf("%s: got %v, want %v", name, err, pst.ErrPropertyDataTooShort)
		}
	}

	booleanReader := pst.PropertyReader{Property: pst.Property{Type: pst.PropertyTypeBoolean}}

	if _, err := booleanReader.GetBoolean(); !errors.Is(err, pst.ErrPropertyNoData) {
		t.Errorf("boolean: got %v, want %v", err, pst.ErrPropertyNoData)
	}
}

// TestPropertyReaderDateFromData checks a time value stored in the row of a Table Context.
func TestPropertyReaderDateFromData(t *testing.T) {
	// 2026-10-25 00:00:00 UTC in 100-nanosecond intervals since 1601-01-01.
	want := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	// 11644473600 is the number of seconds between 1601-01-01 and 1970-01-01.
	intervals := uint64(want.Unix()+11644473600) * 10000000
	data := make([]byte, 8)

	for i := range data {
		data[i] = byte(intervals >> (8 * i))
	}

	reader := pst.PropertyReader{Property: pst.Property{Type: pst.PropertyTypeTime, Data: data}}

	got, err := reader.GetDate()

	if err != nil {
		t.Fatalf("Failed to get date: %+v", err)
	} else if got != want.UnixNano() {
		t.Errorf("got %s, want %s", time.Unix(0, got).UTC(), want)
	}
}
