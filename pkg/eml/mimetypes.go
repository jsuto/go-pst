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

package eml

import (
	"path"
	"strings"
)

// extensionMIMETypes maps file extensions to MIME types for attachments without PR_ATTACH_MIME_TAG.
// A fixed table (instead of mime.TypeByExtension) makes the output independent of the system's MIME type files.
var extensionMIMETypes = map[string]string{
	".7z": "application/x-7z-compressed", ".avif": "image/avif", ".bmp": "image/bmp", ".bz2": "application/x-bzip2",
	".csv": "text/csv", ".doc": "application/msword", ".docm": "application/vnd.ms-word.document.macroEnabled.12",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".dot": "application/msword",
	".dotx": "application/vnd.openxmlformats-officedocument.wordprocessingml.template", ".eml": "message/rfc822",
	".emz": "application/octet-stream", ".eps": "application/postscript", ".gif": "image/gif", ".gz": "application/gzip",
	".heic": "image/heic", ".htm": "text/html", ".html": "text/html", ".ics": "text/calendar", ".jpeg": "image/jpeg",
	".jpg": "image/jpeg", ".js": "text/javascript", ".json": "application/json", ".m4a": "audio/mp4", ".md": "text/markdown",
	".mov": "video/quicktime", ".mp3": "audio/mpeg", ".mp4": "video/mp4", ".msg": "application/vnd.ms-outlook",
	".odp": "application/vnd.oasis.opendocument.presentation", ".ods": "application/vnd.oasis.opendocument.spreadsheet",
	".odt": "application/vnd.oasis.opendocument.text", ".ogg": "audio/ogg", ".p7m": "application/pkcs7-mime",
	".p7s": "application/pkcs7-signature", ".pdf": "application/pdf", ".png": "image/png", ".pps": "application/vnd.ms-powerpoint",
	".ppt": "application/vnd.ms-powerpoint", ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".ps": "application/postscript", ".rar": "application/vnd.rar", ".rpm": "application/x-rpm", ".rtf": "application/rtf",
	".svg": "image/svg+xml", ".tar": "application/x-tar", ".tgz": "application/gzip", ".tif": "image/tiff", ".tiff": "image/tiff",
	".txt": "text/plain", ".vcf": "text/vcard", ".vsd": "application/vnd.visio", ".vsdx": "application/vnd.ms-visio.drawing",
	".wav": "audio/wav", ".webm": "video/webm", ".webp": "image/webp", ".wmf": "image/wmf", ".xls": "application/vnd.ms-excel",
	".xlsm": "application/vnd.ms-excel.sheet.macroEnabled.12",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xml": "application/xml",
	".xps": "application/vnd.ms-xpsdocument", ".zip": "application/zip",
}

// defaultMIMEType is used for attachments of unknown type.
const defaultMIMEType = "application/octet-stream"

// getAttachmentMIMEType returns PR_ATTACH_MIME_TAG if it is a valid type/subtype, otherwise the type of the file extension.
func getAttachmentMIMEType(mimeTag string, filename string) string {
	mimeTag = strings.ToLower(strings.TrimSpace(mimeTag))

	if slash := strings.IndexByte(mimeTag, '/'); slash > 0 && slash < len(mimeTag)-1 && !strings.ContainsAny(mimeTag, " ;\"") {
		return mimeTag
	}

	if mimeType, ok := extensionMIMETypes[strings.ToLower(path.Ext(filename))]; ok {
		return mimeType
	}

	return defaultMIMEType
}
