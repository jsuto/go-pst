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
	"strings"
)

// Kind is the kind of an item, decided by its message class.
type Kind int

// Constants defining the kinds of items.
const (
	// KindMail is mail, including meeting requests and responses, reports (NDR, receipts) and posts.
	KindMail Kind = iota
	// KindNonMail is an appointment, contact, task, note...
	KindNonMail
	// KindShortcut is an Enterprise Vault shortcut: a stub of an archived message.
	KindShortcut
)

func (kind Kind) String() string {
	switch kind {
	case KindMail:
		return "mail"
	case KindShortcut:
		return "Enterprise Vault shortcut"
	default:
		return "non-mail item"
	}
}

// mailClassPrefixes are the message classes (and their sub-classes) converted as mail.
// References https://learn.microsoft.com/en-us/office/vba/outlook/concepts/forms/item-types-and-message-classes
var mailClassPrefixes = []string{"ipm.note", "ipm.schedule.meeting", "ipm.post", "report.", "ipm.outlook.recall"}

// Classify returns the kind of an item with the message class (PR_MESSAGE_CLASS).
// Message classes are case-insensitive, an empty message class is mail.
func Classify(messageClass string) Kind {
	messageClass = strings.ToLower(messageClass)

	if messageClass == "" || messageClass == "ipm" {
		return KindMail
	}

	if hasClassPrefix(messageClass, "ipm.note.enterprisevault.shortcut") {
		return KindShortcut
	}

	for _, prefix := range mailClassPrefixes {
		if hasClassPrefix(messageClass, prefix) {
			return KindMail
		}
	}

	return KindNonMail
}

// hasClassPrefix returns true if the message class is the prefix or one of its sub-classes.
func hasClassPrefix(messageClass string, prefix string) bool {
	if strings.HasSuffix(prefix, ".") {
		return strings.HasPrefix(messageClass, prefix)
	}

	return messageClass == prefix || strings.HasPrefix(messageClass, prefix+".")
}
