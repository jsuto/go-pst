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
	"bytes"
	"encoding/binary"

	"github.com/rotisserie/eris"
)

// [MS-OXRTFCP]: Rich Text Format (RTF) Compression Algorithm:
// https://learn.microsoft.com/en-us/openspecs/exchange_server_protocols/ms-oxrtfcp/65dfe2df-1b69-43fc-8ebd-21819a7463fb?redirectedfrom=MSDN

type RTFDecoder struct{}

func NewRTFDecoder() *RTFDecoder {
	return &RTFDecoder{}
}

// LZFUHeader is the initial content of the dictionary (207 bytes).
// References [MS-OXRTFCP]: 2.1.2.1 Dictionary.
const LZFUHeader = "{\\rtf1\\ansi\\mac\\deff0\\deftab720{\\fonttbl;}{\\f0\\fnil \\froman \\fswiss \\fmodern \\fscript \\fdecor MS Sans SerifSymbolArialTimes New RomanCourier{\\colortbl\\red0\\green0\\blue0\r\n\\par \\pard\\plain\\f0\\fs20\\b\\i\\u\\tab\\tx"

var (
	CompressionTypeCompressed   = []byte("LZFu")
	CompressionTypeUncompressed = []byte("MELA")
)

// Constants of the compressed RTF format.
const (
	rtfHeaderSize     = 16 // COMPSIZE, RAWSIZE, COMPTYPE, CRC.
	rtfDictionarySize = 4096
	// rtfMaxExpansion bounds RAWSIZE: a 2-byte reference expands to at most 17 bytes.
	rtfMaxExpansion = 9
)

// Decode decompresses PR_RTF_COMPRESSED.
// References [MS-OXRTFCP]: 2.2 Message Syntax, 2.1.3.1 Decompression.
func (rtfDecoder *RTFDecoder) Decode(data []byte) (string, error) {
	if len(data) < rtfHeaderSize {
		return "", ErrRTFInvalid
	}

	// COMPSIZE is the size of the data following it.
	// The sizes are compared as int64, so they can't become negative on 32-bit platforms.
	compressedSize := int64(binary.LittleEndian.Uint32(data[:4]))
	rawSize := int64(binary.LittleEndian.Uint32(data[4:8]))
	compressionSignature := data[8:12]

	end := len(data)

	if compressedSize >= 12 && 4+compressedSize < int64(end) {
		end = int(4 + compressedSize)
	}

	input := data[rtfHeaderSize:end]

	switch {
	case bytes.Equal(compressionSignature, CompressionTypeUncompressed):
		if rawSize > int64(len(input)) {
			return "", ErrRTFInvalid
		}

		return string(input[:rawSize]), nil
	case !bytes.Equal(compressionSignature, CompressionTypeCompressed):
		return "", eris.Wrap(ErrRTFInvalid, "unknown compression signature")
	case rawSize > int64(len(input)+1)*rtfMaxExpansion:
		return "", eris.Wrap(ErrRTFInvalid, "uncompressed size too large")
	}

	uncompressedSize := int(rawSize)

	dictionary := make([]byte, rtfDictionarySize)
	writePosition := copy(dictionary, LZFUHeader)
	output := make([]byte, 0, uncompressedSize)
	position := 0

	for position < len(input) && len(output) < uncompressedSize {
		control := input[position]
		position++

		for bit := 0; bit < 8 && len(output) < uncompressedSize; bit++ {
			if control&(1<<bit) == 0 {
				// Literal.
				if position >= len(input) {
					return string(output), nil
				}

				dictionary[writePosition] = input[position]
				writePosition = (writePosition + 1) % rtfDictionarySize
				output = append(output, input[position])
				position++

				continue
			}

			// Dictionary reference: 12-bit offset, 4-bit length (+2), big-endian.
			if position+2 > len(input) {
				return "", eris.Wrap(ErrRTFInvalid, "truncated dictionary reference")
			}

			reference := binary.BigEndian.Uint16(input[position : position+2])
			position += 2

			readPosition := int(reference >> 4)
			length := int(reference&0xF) + 2

			if readPosition == writePosition {
				// End of the compressed data.
				return string(output), nil
			}

			for i := 0; i < length && len(output) < uncompressedSize; i++ {
				character := dictionary[readPosition]
				readPosition = (readPosition + 1) % rtfDictionarySize
				dictionary[writePosition] = character
				writePosition = (writePosition + 1) % rtfDictionarySize
				output = append(output, character)
			}
		}
	}

	return string(output), nil
}
