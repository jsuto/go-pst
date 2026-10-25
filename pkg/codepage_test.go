package pst_test

import (
	"encoding/binary"
	"testing"

	pst "github.com/jsuto/go-pst/pkg"
)

func TestDecodeString8(t *testing.T) {
	propertyReader := pst.PropertyReader{File: &pst.File{DefaultCodePage: 1252}}

	for _, test := range []struct {
		codePage int
		data     []byte
		want     string
	}{
		{1252, []byte("Caf\xe9"), "Café"},
		{1250, []byte("\xe1rv\xedzt\xfbr\xf5"), "árvíztűrő"},
		{932, []byte("\x93\xfa\x96\x7b"), "日本"},
		{936, []byte("\xd6\xd0\xce\xc4"), "中文"},
		{949, []byte("\xc7\xd1\xb1\xb9"), "한국"},
		{65001, []byte("\xc3\xa9"), "é"},
		// Unsupported (UTF-7) and unknown code pages fall back to File.DefaultCodePage.
		{65000, []byte("Caf\xe9"), "Café"},
		{12345, []byte("Caf\xe9"), "Café"},
	} {
		got, err := propertyReader.DecodeString8(test.data, test.codePage)

		if err != nil {
			t.Errorf("%d: %+v", test.codePage, err)
		} else if got != test.want {
			t.Errorf("%d: got %q, want %q", test.codePage, got, test.want)
		}
	}
}

func TestGetCodePage(t *testing.T) {
	file := &pst.File{DefaultCodePage: 1251}

	messageCodePage := func(codePage uint32) pst.Property {
		data := make([]byte, 4)
		binary.LittleEndian.PutUint32(data, codePage)

		return pst.Property{ID: 0x3FFD, Type: pst.PropertyTypeInteger32, Data: data}
	}

	for _, test := range []struct {
		properties []pst.Property
		want       int
	}{
		{[]pst.Property{messageCodePage(1250)}, 1250},
		{nil, 1251},
		{[]pst.Property{messageCodePage(65000)}, 1251},
	} {
		propertyContext := pst.PropertyContext{Properties: test.properties, File: file}

		if got := propertyContext.GetCodePage(); got != test.want {
			t.Errorf("%+v: got %d, want %d", test.properties, got, test.want)
		}
	}
}
