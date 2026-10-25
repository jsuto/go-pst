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
	"fmt"

	pst "github.com/jsuto/go-pst/pkg"
	"github.com/rotisserie/eris"
)

// Item is a message (or another item, see Classify) of a folder, or an error reading the folder or the message.
type Item struct {
	// FolderPath is the path of the folder below the top of the folders of the user, for example ["Inbox", "Projects"].
	FolderPath []string
	Folder     *pst.Folder
	// InDeletedItems is set for the Deleted Items folder (PR_IPM_WASTEBASKET_ENTRYID) and its sub-folders.
	InDeletedItems bool
	// Message is nil if Err is set.
	Message *pst.Message
	// MessageIdentifier is the identifier of the message, 0 for errors of the folder.
	MessageIdentifier pst.Identifier
	Err               error
}

// WalkFunc is called by Walk for each item. Returning an error stops the walk.
type WalkFunc func(item Item) error

// Walk calls the function for each item of each folder below the top of the folders of the user (the IPM subtree),
// or of all folders if the IPM subtree is not found. Search folders are skipped, their items are in other folders.
// Errors reading a folder or a message (also panics) are passed as an Item with Err, and the walk continues.
func Walk(file *pst.File, walkFunc WalkFunc) error {
	rootFolder, err := file.GetRootFolder()

	if err != nil {
		return eris.Wrap(err, "failed to get root folder")
	}

	walker := &walker{file: file, walkFunc: walkFunc, inIPMSubtree: true}

	if ipmSubtreeIdentifier, err := file.GetIPMSubtreeIdentifier(); err == nil {
		walker.ipmSubtreeIdentifier = ipmSubtreeIdentifier
		walker.inIPMSubtree = false
	}

	if deletedItemsIdentifier, err := file.GetDeletedItemsIdentifier(); err == nil {
		walker.deletedItemsIdentifier = deletedItemsIdentifier
	}

	return walker.walkFolder(&rootFolder, nil, false)
}

type walker struct {
	file                   *pst.File
	walkFunc               WalkFunc
	ipmSubtreeIdentifier   pst.Identifier
	deletedItemsIdentifier pst.Identifier
	inIPMSubtree           bool
}

func (walker *walker) walkFolder(folder *pst.Folder, folderPath []string, inDeletedItems bool) error {
	isIPMSubtree := walker.ipmSubtreeIdentifier != 0 && folder.Identifier == walker.ipmSubtreeIdentifier
	inDeletedItems = inDeletedItems || (walker.deletedItemsIdentifier != 0 && folder.Identifier == walker.deletedItemsIdentifier)

	if isIPMSubtree {
		// Paths start below the IPM subtree.
		walker.inIPMSubtree = true
		folderPath = nil
	}

	if walker.inIPMSubtree && folder.Identifier.GetType() != pst.IdentifierTypeSearchFolder {
		if err := walker.walkMessages(folder, folderPath, inDeletedItems); err != nil {
			return err
		}
	}

	subFolders, err := walker.getSubFolders(folder)

	if err != nil {
		if walker.inIPMSubtree {
			if err := walker.walkFunc(Item{FolderPath: folderPath, Folder: folder, InDeletedItems: inDeletedItems, Err: eris.Wrap(err, "failed to get sub-folders")}); err != nil {
				return err
			}
		}
	}

	for i := range subFolders {
		subFolderPath := append(append([]string(nil), folderPath...), subFolders[i].Name)

		if folder.Identifier == pst.IdentifierRootFolder && walker.ipmSubtreeIdentifier == 0 {
			// Without an IPM subtree, paths start below the root folder.
			subFolderPath = []string{subFolders[i].Name}
		}

		if err := walker.walkFolder(&subFolders[i], subFolderPath, inDeletedItems); err != nil {
			return err
		}
	}

	if isIPMSubtree {
		walker.inIPMSubtree = false
	}

	return nil
}

// getSubFolders returns the sub-folders, recovering from panics.
func (walker *walker) getSubFolders(folder *pst.Folder) (subFolders []pst.Folder, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic while reading the sub-folders: %v", recovered)
		}
	}()

	return folder.GetSubFolders()
}

func (walker *walker) walkMessages(folder *pst.Folder, folderPath []string, inDeletedItems bool) error {
	if folder.MessageCount == 0 {
		return nil
	}

	messageIdentifiers, err := getMessageIdentifiers(folder)

	if err != nil {
		return walker.walkFunc(Item{FolderPath: folderPath, Folder: folder, InDeletedItems: inDeletedItems, Err: err})
	}

	for _, messageIdentifier := range messageIdentifiers {
		message, err := getMessage(walker.file, messageIdentifier)

		item := Item{FolderPath: folderPath, Folder: folder, InDeletedItems: inDeletedItems, Message: message, MessageIdentifier: messageIdentifier, Err: err}

		if err != nil {
			item.Message = nil
		}

		if err := walker.walkFunc(item); err != nil {
			return err
		}
	}

	return nil
}

// getMessageIdentifiers returns the identifiers of the messages in the contents table of the folder.
func getMessageIdentifiers(folder *pst.Folder) (messageIdentifiers []pst.Identifier, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic while reading the contents table: %v", recovered)
		}
	}()

	tableContext, err := folder.GetMessageTableContext()

	if err != nil {
		return nil, eris.Wrap(err, "failed to get the contents table")
	}

	for _, row := range tableContext.Properties {
		// GetMessageTableContext only returns the message identifier column.
		for _, property := range row {
			propertyReader, err := tableContext.GetPropertyReader(property)

			if err != nil {
				return nil, eris.Wrap(err, "failed to get message identifier property reader")
			}

			messageIdentifier, err := propertyReader.GetInteger32()

			if err != nil {
				return nil, eris.Wrap(err, "failed to read message identifier")
			}

			messageIdentifiers = append(messageIdentifiers, pst.Identifier(messageIdentifier))
		}
	}

	return messageIdentifiers, nil
}

// getMessage reads the message, recovering from panics.
func getMessage(file *pst.File, messageIdentifier pst.Identifier) (message *pst.Message, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic while reading message %d: %v", messageIdentifier, recovered)
		}
	}()

	return file.GetMessage(messageIdentifier)
}
