// go-pst is a library for reading Personal Storage Table (.pst) files (written in Go/Golang).
//
// Copyright 2026 Janos SUTO
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
	"strconv"
	"strings"
	"unicode/utf8"
)

// RTFBody is the result of ConvertRTF: HTML if the RTF encapsulates HTML (\fromhtml1), otherwise the text.
type RTFBody struct {
	HTML string
	Text string
}

// IsHTML returns true if the RTF encapsulated HTML.
func (rtfBody RTFBody) IsHTML() bool {
	return rtfBody.HTML != ""
}

// ConvertRTF converts a (decompressed) RTF body.
// RTF encapsulating HTML is converted back to HTML, references [MS-OXRTFEX]: 2.1.3 Decoding the HTML.
// Other RTF (including \fromtext) is converted to plain text, ignoring the formatting.
// The result is UTF-8; \'hh characters are decoded using the code page of the font (\fcharset, \cpg) or \ansicpg.
func ConvertRTF(rtf string) RTFBody {
	converter := newRTFConverter(rtf)

	converter.convert()

	if converter.isHTML {
		return RTFBody{HTML: converter.output.String()}
	}

	return RTFBody{Text: converter.output.String()}
}

// rtfGroupState is the state of an RTF group, restored at the end of the group.
type rtfGroupState struct {
	skip             bool // Ignored destination (font table, pictures, \* destinations...).
	htmlRTF          bool // \htmlrtf: RTF-only content of encapsulated HTML.
	htmlTag          bool // {\*\htmltag ...}: HTML content.
	fontTable        bool
	font             int
	unicodeSkipCount int // \ucN
}

type rtfConverter struct {
	rtf      string
	position int
	isHTML   bool
	output   strings.Builder

	state       rtfGroupState
	stack       []rtfGroupState
	groupStart  bool // The next token is the first of a group.
	ignorable   bool // \* was seen at the start of the group.
	skipPending int  // Characters to skip after \uN.

	defaultCodePage int
	fontCodePages   map[int]int
	tableFont       int // Font being defined in the font table.

	pendingBytes    []byte // \'hh and text bytes waiting to be decoded.
	pendingCodePage int
}

func newRTFConverter(rtf string) *rtfConverter {
	return &rtfConverter{
		rtf:             rtf,
		state:           rtfGroupState{unicodeSkipCount: 1},
		defaultCodePage: 1252,
		fontCodePages:   map[int]int{},
	}
}

// rtfDestinations are groups whose content is not text.
var rtfDestinations = map[string]bool{
	"fonttbl": true, "colortbl": true, "stylesheet": true, "info": true, "pict": true, "object": true,
	"header": true, "headerl": true, "headerr": true, "headerf": true, "footer": true, "footerl": true,
	"footerr": true, "footerf": true, "fldinst": true, "listtable": true, "listoverridetable": true,
	"rsidtbl": true, "generator": true, "xmlnstbl": true, "themedata": true, "colorschememapping": true,
	"latentstyles": true, "datastore": true, "mmathPr": true, "pgdsctbl": true, "filetbl": true,
	"revtbl": true, "userprops": true, "docvar": true, "nonshppict": true, "objdata": true,
	"bkmkstart": true, "bkmkend": true, "template": true, "xe": true, "tc": true, "author": true,
	"operator": true, "title": true, "subject": true, "comment": true, "company": true, "keywords": true,
}

// rtfCharsetCodePages maps \fcharset to code pages.
var rtfCharsetCodePages = map[int]int{
	0: 1252, 77: 10000, 128: 932, 129: 949, 130: 1361, 134: 936, 136: 950, 161: 1253, 162: 1254, 163: 1258,
	177: 1255, 178: 1256, 186: 1257, 204: 1251, 222: 874, 238: 1250, 254: 437,
}

// rtfSymbols maps control words to the characters they represent.
var rtfSymbols = map[string]string{
	"tab": "\t", "lquote": "‘", "rquote": "’", "ldblquote": "“", "rdblquote": "”",
	"emdash": "—", "endash": "–", "bullet": "•", "emspace": " ", "enspace": " ",
	"qmspace": " ", "zwj": "‍", "zwnj": "‌", "ltrmark": "‎", "rtlmark": "‏",
}

// isOutputSuppressed returns true if text in the current state is not part of the result.
func (converter *rtfConverter) isOutputSuppressed() bool {
	return converter.state.skip || converter.state.fontTable || (converter.isHTML && converter.state.htmlRTF && !converter.state.htmlTag)
}

func (converter *rtfConverter) convert() {
	for converter.position < len(converter.rtf) {
		character := converter.rtf[converter.position]
		converter.position++

		switch character {
		case '{':
			converter.flush()
			converter.stack = append(converter.stack, converter.state)
			converter.groupStart = true
			converter.ignorable = false

			continue
		case '}':
			converter.flush()

			if len(converter.stack) > 0 {
				converter.state = converter.stack[len(converter.stack)-1]
				converter.stack = converter.stack[:len(converter.stack)-1]
			}
		case '\\':
			converter.controlSequence()
		case '\r', '\n':
			// Line breaks in the RTF source are not text, nor the first token of a group.
			continue
		default:
			converter.text(character)
		}

		converter.groupStart = false
	}

	converter.flush()
}

// controlSequence handles a control word or control symbol after the backslash.
func (converter *rtfConverter) controlSequence() {
	if converter.position >= len(converter.rtf) {
		return
	}

	character := converter.rtf[converter.position]

	if !isASCIILetter(character) {
		converter.position++
		converter.controlSymbol(character)

		return
	}

	start := converter.position

	for converter.position < len(converter.rtf) && isASCIILetter(converter.rtf[converter.position]) {
		converter.position++
	}

	word := converter.rtf[start:converter.position]
	parameterStart := converter.position

	if converter.position < len(converter.rtf) && converter.rtf[converter.position] == '-' {
		converter.position++
	}

	for converter.position < len(converter.rtf) && converter.rtf[converter.position] >= '0' && converter.rtf[converter.position] <= '9' {
		converter.position++
	}

	parameterText := converter.rtf[parameterStart:converter.position]
	parameter, err := strconv.Atoi(parameterText)
	hasParameter := err == nil

	// A space delimiting the control word is part of it.
	if converter.position < len(converter.rtf) && converter.rtf[converter.position] == ' ' {
		converter.position++
	}

	converter.controlWord(word, parameter, hasParameter)
}

func (converter *rtfConverter) controlSymbol(character byte) {
	switch character {
	case '\'':
		// \'hh: a byte in the current code page.
		if converter.position+2 > len(converter.rtf) {
			converter.position = len(converter.rtf)

			return
		}

		value, err := strconv.ParseUint(converter.rtf[converter.position:converter.position+2], 16, 8)
		converter.position += 2

		if err == nil {
			converter.text(byte(value))
		}
	case '*':
		if converter.groupStart {
			converter.ignorable = true
			// Keep groupStart for the destination control word.
			converter.groupStart = true
			converter.position = converter.skipSpaces(converter.position)

			if converter.position < len(converter.rtf) && converter.rtf[converter.position] == '\\' {
				converter.position++
				converter.controlSequence()
			}
		}
	case '\\', '{', '}':
		converter.text(character)
	case '~':
		converter.emit(" ")
	case '_':
		converter.emit("‑")
	case '-':
		// Optional hyphen.
	case '\r', '\n':
		converter.emit("\n")
	}
}

func (converter *rtfConverter) skipSpaces(position int) int {
	for position < len(converter.rtf) && (converter.rtf[position] == '\r' || converter.rtf[position] == '\n') {
		position++
	}

	return position
}

func (converter *rtfConverter) controlWord(word string, parameter int, hasParameter bool) {
	if converter.groupStart {
		converter.groupStart = false

		switch {
		case word == "htmltag" && converter.ignorable:
			converter.flush()
			converter.state.htmlTag = true

			return
		case word == "fonttbl":
			converter.flush()
			converter.state.fontTable = true

			return
		case converter.ignorable || rtfDestinations[word]:
			converter.flush()
			converter.state.skip = true

			return
		}
	}

	switch word {
	case "fromhtml":
		converter.isHTML = true
	case "ansicpg":
		if hasParameter && isCodePageSupported(parameter) {
			converter.defaultCodePage = parameter
		}
	case "f":
		if converter.state.fontTable {
			converter.tableFont = parameter
		} else {
			converter.state.font = parameter
		}
	case "fcharset":
		if codePage, ok := rtfCharsetCodePages[parameter]; ok && converter.state.fontTable {
			converter.fontCodePages[converter.tableFont] = codePage
		}
	case "cpg":
		if converter.state.fontTable && isCodePageSupported(parameter) {
			converter.fontCodePages[converter.tableFont] = parameter
		}
	case "uc":
		if hasParameter && parameter >= 0 {
			converter.state.unicodeSkipCount = parameter
		}
	case "u":
		if hasParameter {
			if parameter < 0 {
				parameter += 65536
			}

			converter.emitRune(rune(parameter))
			converter.skipPending = converter.state.unicodeSkipCount
		}
	case "htmlrtf":
		converter.flush()
		converter.state.htmlRTF = !hasParameter || parameter != 0
	case "par", "line", "row":
		converter.emit("\n")
	case "cell":
		converter.emit("\t")
	case "bin":
		// Binary data of the given length.
		if hasParameter && parameter > 0 {
			converter.position = min(converter.position+parameter, len(converter.rtf))
		}
	default:
		if symbol, ok := rtfSymbols[word]; ok {
			converter.emit(symbol)
		}
	}
}

// text handles a byte of text, decoded with the code page of the current font.
func (converter *rtfConverter) text(character byte) {
	if converter.skipPending > 0 {
		converter.skipPending--

		return
	}

	if converter.isOutputSuppressed() {
		return
	}

	codePage := converter.defaultCodePage

	if fontCodePage, ok := converter.fontCodePages[converter.state.font]; ok {
		codePage = fontCodePage
	}

	if codePage != converter.pendingCodePage {
		converter.flush()
		converter.pendingCodePage = codePage
	}

	converter.pendingBytes = append(converter.pendingBytes, character)
}

// emit adds characters that don't depend on the code page.
func (converter *rtfConverter) emit(text string) {
	if converter.skipPending > 0 {
		converter.skipPending--

		return
	}

	if converter.isOutputSuppressed() {
		return
	}

	converter.flush()
	converter.output.WriteString(text)
}

func (converter *rtfConverter) emitRune(character rune) {
	if converter.isOutputSuppressed() {
		return
	}

	if !utf8.ValidRune(character) {
		character = utf8.RuneError
	}

	converter.flush()
	converter.output.WriteRune(character)
}

// flush decodes the pending bytes.
func (converter *rtfConverter) flush() {
	if len(converter.pendingBytes) == 0 {
		return
	}

	codePageEncoding := getCodePageEncoding(converter.pendingCodePage)

	if codePageEncoding == nil {
		codePageEncoding = getCodePageEncoding(DefaultCodePage)
	}

	if decoded, err := codePageEncoding.NewDecoder().Bytes(converter.pendingBytes); err == nil {
		converter.output.Write(decoded)
	}

	converter.pendingBytes = converter.pendingBytes[:0]
}

func isASCIILetter(character byte) bool {
	return (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z')
}
