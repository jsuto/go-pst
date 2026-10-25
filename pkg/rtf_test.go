package pst_test

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	pst "github.com/jsuto/go-pst/pkg"
)

func TestRTFDictionary(t *testing.T) {
	if len(pst.LZFUHeader) != 207 {
		t.Errorf("got %d bytes, want 207", len(pst.LZFUHeader))
	}
}

// TestRTFDecode uses the examples of [MS-OXRTFCP]: 4 Protocol Examples.
func TestRTFDecode(t *testing.T) {
	for _, test := range []struct {
		name       string
		compressed string
		want       string
	}{
		{
			"simple",
			"2d0000002b0000004c5a4675f1c5c7a703000a0072637067313235423 20af320686" +
				"56c090020627705b06c647d0a800fa0",
			"{\\rtf1\\ansi\\ansicpg1252\\pard hello world}\r\n",
		},
		{
			"crossing the write position",
			"1a0000001c0000004c5a4675e2d44b51410004205758595a0d6e7d010eb0",
			"{\\rtf1 WXYZWXYZWXYZWXYZWXYZ}",
		},
		{
			"uncompressed",
			"0f0000000300000" + "04d454c4100000000" + "7b7d0a",
			"{}\n",
		},
	} {
		data, err := hex.DecodeString(strings.ReplaceAll(test.compressed, " ", ""))

		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}

		got, err := pst.NewRTFDecoder().Decode(data)

		if err != nil {
			t.Errorf("%s: %+v", test.name, err)
		} else if got != test.want {
			t.Errorf("%s: got %q, want %q", test.name, got, test.want)
		}
	}
}

// TestRTFDecodeSizes checks RAWSIZE values that don't fit the data.
func TestRTFDecodeSizes(t *testing.T) {
	for _, header := range []string{
		"0f000000ffffffff4d454c4100000000", // Uncompressed, RAWSIZE 0xFFFFFFFF.
		"ffffffffffffffff4c5a467500000000", // Compressed, COMPSIZE and RAWSIZE 0xFFFFFFFF.
	} {
		data, _ := hex.DecodeString(header + "7b7d0a")

		if _, err := pst.NewRTFDecoder().Decode(data); !errors.Is(err, pst.ErrRTFInvalid) {
			t.Errorf("%s: got %v, want %v", header, err, pst.ErrRTFInvalid)
		}
	}
}

func FuzzRTFDecode(f *testing.F) {
	data, _ := hex.DecodeString("2d0000002b0000004c5a4675f1c5c7a703000a007263706731323542320af32068656c090020627705b06c647d0a800fa0")
	f.Add(data)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = pst.NewRTFDecoder().Decode(data)
	})
}
