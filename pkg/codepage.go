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
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/ianaindex"
)

// CodePageIdentifierToEncoding maps the code page identifier to the IANA encoding for decoding String8 (PropertyReader).
// References https://docs.microsoft.com/en-us/windows/win32/intl/code-page-identifiers
var CodePageIdentifierToEncoding = map[int]string{
	28596: "iso-8859-6",
	1256:  "windows-1256",
	28594: "iso-8859-4",
	1257:  "windows-1257",
	28592: "iso-8859-2",
	1250:  "windows-1250",
	936:   "gbk",
	52936: "hz-gb-2312",
	54936: "gb18030",
	950:   "big5",
	28595: "iso-8859-5",
	20866: "koi8-r",
	21866: "koi8-u",
	1251:  "windows-1251",
	28597: "iso-8859-7",
	1253:  "windows-1253",
	38598: "iso-8859-8-i",
	1255:  "windows-1255",
	51932: "euc-jp",
	50220: "iso-2022-jp",
	50221: "csISO2022JP",
	932:   "shift_jis",
	949:   "euc-kr", // x/text EUC-KR decodes the Unified Hangul Code of code page 949.
	51949: "euc-kr",
	28593: "iso-8859-3",
	28605: "iso-8859-15",
	874:   "windows-874",
	28599: "iso-8859-9",
	1254:  "windows-1254",
	65001: "utf-8",
	20127: "us-ascii",
	1258:  "windows-1258",
	28591: "iso-8859-1",
	1252:  "windows-1252",
}

// DefaultCodePage is used by New for File.DefaultCodePage.
const DefaultCodePage = 1252

// propertyIDMessageCodePage is PR_MESSAGE_CODEPAGE, the code page of the String8 properties of a message.
// References [MS-OXCMSG]: 2.2.1.4 PidTagMessageCodepage.
const propertyIDMessageCodePage = 0x3FFD

// GetCodePage returns the code page for the String8 properties of this property context:
// PR_MESSAGE_CODEPAGE if it is set and supported, otherwise File.DefaultCodePage.
func (propertyContext *PropertyContext) GetCodePage() int {
	if propertyReader, err := propertyContext.GetPropertyReader(propertyIDMessageCodePage, nil); err == nil {
		if codePage, err := propertyReader.GetInteger32(); err == nil && isCodePageSupported(int(codePage)) {
			return int(codePage)
		}
	}

	if propertyContext.File != nil {
		return propertyContext.File.DefaultCodePage
	}

	return DefaultCodePage
}

// getCodePageEncoding returns the encoding of the code page, nil if it is unknown or not implemented.
func getCodePageEncoding(codePageIdentifier int) encoding.Encoding {
	name, ok := CodePageIdentifierToEncoding[codePageIdentifier]

	if !ok {
		return nil
	}

	codePageEncoding, err := ianaindex.IANA.Encoding(name)

	if err != nil {
		return nil
	}

	// Nil if the encoding is known but not implemented.
	return codePageEncoding
}

func isCodePageSupported(codePageIdentifier int) bool {
	return getCodePageEncoding(codePageIdentifier) != nil
}
