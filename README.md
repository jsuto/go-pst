# go-pst

A Go library for reading PST files (Microsoft Outlook and Enterprise Vault
exports).

This is a continuation of [mooijtech/go-pst](https://github.com/mooijtech/go-pst)
by Marten Mooij, which has not been maintained since 2023. It is maintained
for the needs of the PST import of the [Piler](https://www.mailpiler.org/)
email archive: reading PST files and converting their messages to
RFC 822 / MIME (EML). Fixes and features outside that scope (writing PST
files, OST or PAB support) are not planned.

The module path is `github.com/jsuto/go-pst`; versions start at `v0.1.0`
and are not compatible with upstream's `v6`.

## Status

Reads Unicode and ANSI PST files and converts their mail to EML. Planned:
test fixtures without personal data and robustness runs.

Changes compared to upstream v6.0.2:

- module path `github.com/jsuto/go-pst`, Go 1.27, updated dependencies
- the Linux io_uring reader (`NewAsync`) was removed; `File.Reader` is a
  plain `io.ReaderAt`
- recipient table: `Message.GetRecipients` returns the To/Cc/Bcc recipients
  with display name, address type, email address and SMTP address
- sender: `Message.GetSender` and `Message.GetSentRepresenting` (the From
  header), with `PR_SENDER_SMTP_ADDRESS` / `PR_SENT_REPRESENTING_SMTP_ADDRESS`;
  `Mailbox.Address` returns the SMTP address, else the email address as stored
  (possibly X.500)
- ANSI (32-bit) PST files (upstream #52): String8 properties are decoded with
  the code page of the message (`PR_MESSAGE_CODEPAGE`), else
  `File.DefaultCodePage` (1252 unless changed)
- local descriptors (subnodes) with level > 0 (upstream #45)
- attachments: embedded messages (`Attachment.GetEmbeddedMessage`, upstream
  #10) can be read like any other message, `Attachment.IsInline`, constants for
  `PR_ATTACH_METHOD` and `PR_ATTACH_FLAGS`; `WriteTo` returns an error for
  embedded messages
- bodies: `Message.GetBody` returns `PR_BODY` and `PR_HTML` (with the code
  page of `PR_INTERNET_CPID`, `GetCharsetName` for the MIME charset); without
  `PR_HTML`, compressed RTF is converted: RTF encapsulating HTML back to HTML
  ([MS-OXRTFEX]), other RTF to text (`ConvertRTF`); the RTF decompression
  follows [MS-OXRTFCP] (wrong dictionary and uncompressed RTF fixed)
- converter to EML (`pkg/eml`) and `cmd/pst2eml`
- `GetMessage` no longer prints to stdout

## Converting to EML

```bash
pst2eml -o out/ mailbox.pst
```

writes one `.eml` per message into a directory per folder (`out/Inbox/1.eml`,
...) and prints a report per folder: items read, converted, skipped (with the
reason) and errors; details of errors and skipped attachments go to stderr.

| Option | |
|---|---|
| `-o <directory>` | output directory (required) |
| `-skip-deleted` | don't convert the items of Deleted Items (counted as skipped) |
| `-codepage <n>` | code page of 8-bit strings without `PR_MESSAGE_CODEPAGE` (ANSI files), default 1252 |
| `-version` | print the version |

The exit status is 0 if every item was converted or skipped by design
(non-mail items, Enterprise Vault shortcuts, Deleted Items with
`-skip-deleted`), 1 if the file can't be read or items failed, 2 for usage
errors.

Binaries: `go install github.com/jsuto/go-pst/cmd/pst2eml@latest`, or the
release archives built by `tools/build-release.sh <version>` (static binaries
for Linux, Windows and macOS on amd64 and arm64, with `SHA256SUMS`).

The `eml` package does the conversion:

- `eml.Walk` calls a function for each item of each folder below the top of the
  user's folders, with its folder path and whether it is in Deleted Items.
  Search folders are skipped. Errors reading a folder or a message (also
  panics) are passed to the function, the walk continues.
- `eml.Convert` writes a message as RFC 822 / MIME. Messages with
  `PR_TRANSPORT_MESSAGE_HEADERS` keep those headers (without the fields of the
  original MIME structure), other messages (sent mail, drafts) get headers
  built from their properties: From, Sender, To, Cc, Bcc (with SMTP
  addresses; X.500 addresses without one are written as they are), Subject,
  Date, Message-ID, In-Reply-To, References, Importance. The body is
  `multipart/alternative` (text and HTML) in `multipart/related` (attachments
  referenced by the HTML) in `multipart/mixed` (attachments, embedded messages
  as `message/rfc822`). HTML keeps its bytes and charset. S/MIME messages
  (signed or encrypted) are rebuilt from the original S/MIME entity Outlook
  keeps as their attachment, so signatures stay verifiable.
- Non-mail items (appointments, contacts, ...) and Enterprise Vault shortcuts
  return a `*eml.SkipError`; `eml.Classify` decides by the message class.

```go
err := eml.Walk(pstFile, func(item eml.Item) error {
  if item.Err != nil {
    return nil // or count it
  }

  var buffer bytes.Buffer

  if _, err := eml.Convert(&buffer, item.Message); err != nil {
    return nil // *eml.SkipError or a conversion error
  }

  // buffer holds the message, item.FolderPath its folder.
  return nil
})
```

## Reading PST files

```bash
go get github.com/jsuto/go-pst
```

```go
package main

import (
  "fmt"
  "github.com/jsuto/go-pst/pkg"
  "github.com/jsuto/go-pst/pkg/properties"
  "github.com/rotisserie/eris"
  "golang.org/x/text/encoding"
  "os"
  "time"

  charsets "github.com/emersion/go-message/charset"
)

func main() {
  pst.ExtendCharsets(func(name string, enc encoding.Encoding) {
    charsets.RegisterEncoding(name, enc)
  })

  startTime := time.Now()

  fmt.Println("Initializing...")

  reader, err := os.Open("../data/enron.pst")

  if err != nil {
    panic(fmt.Sprintf("Failed to open PST file: %+v\n", err))
  }

  pstFile, err := pst.New(reader)

  if err != nil {
    panic(fmt.Sprintf("Failed to open PST file: %+v\n", err))
  }

  defer func() {
    pstFile.Cleanup()

    if errClosing := reader.Close(); errClosing != nil {
      panic(fmt.Sprintf("Failed to close PST file: %+v\n", err))
    }
  }()

  // Create attachments directory
  if _, err := os.Stat("attachments"); err != nil {
    if err := os.Mkdir("attachments", 0755); err != nil {
      panic(fmt.Sprintf("Failed to create attachments directory: %+v", err))
    }
  }

  // Walk through folders.
  if err := pstFile.WalkFolders(func(folder *pst.Folder) error {
    fmt.Printf("Walking folder: %s\n", folder.Name)

    messageIterator, err := folder.GetMessageIterator()

    if eris.Is(err, pst.ErrMessagesNotFound) {
      // Folder has no messages.
      return nil
    } else if err != nil {
      return err
    }

    // Iterate through messages.
    for messageIterator.Next() {
      message := messageIterator.Value()

      switch messageProperties := message.Properties.(type) {
      case *properties.Appointment:
        //fmt.Printf("Appointment: %s\n", messageProperties.String())
      case *properties.Contact:
        //fmt.Printf("Contact: %s\n", messageProperties.String())
      case *properties.Task:
        //fmt.Printf("Task: %s\n", messageProperties.String())
      case *properties.RSS:
        //fmt.Printf("RSS: %s\n", messageProperties.String())
      case *properties.AddressBook:
        //fmt.Printf("Address book: %s\n", messageProperties.String())
      case *properties.Message:
        fmt.Printf("Subject: %s\n", messageProperties.GetSubject())
      case *properties.Note:
        //fmt.Printf("Note: %s\n", messageProperties.String())
      default:
        fmt.Printf("Unknown message type\n")
      }

      attachmentIterator, err := message.GetAttachmentIterator()

      if eris.Is(err, pst.ErrAttachmentsNotFound) {
        // This message has no attachments.
        continue
      } else if err != nil {
        return err
      }

      // Iterate through attachments.
      for attachmentIterator.Next() {
        attachment := attachmentIterator.Value()

        var attachmentOutputPath string

        if attachment.GetAttachLongFilename() != "" {
          attachmentOutputPath = fmt.Sprintf("attachments/%d-%s", attachment.Identifier, attachment.GetAttachLongFilename())
        } else {
          attachmentOutputPath = fmt.Sprintf("attachments/UNKNOWN_%d", attachment.Identifier)
        }

        attachmentOutput, err := os.Create(attachmentOutputPath)

        if err != nil {
          return err
        }

        if _, err := attachment.WriteTo(attachmentOutput); err != nil {
          return err
        }

        if err := attachmentOutput.Close(); err != nil {
          return err
        }
      }

      if attachmentIterator.Err() != nil {
        return attachmentIterator.Err()
      }
    }

    return messageIterator.Err()
  }); err != nil {
    panic(fmt.Sprintf("Failed to walk folders: %+v\n", err))
  }

  fmt.Printf("Time: %s\n", time.Since(startTime).String())
}
```


## Documentation

- [Outlook Personal Folders (.pst) File Format](docs/README.md)
- [MS-PST](https://learn.microsoft.com/openspecs/office_file_formats/ms-pst/)
- [MS-OXPROPS](https://learn.microsoft.com/openspecs/exchange_server_protocols/ms-oxprops/) (property tags)
- [MS-OXCMAIL](https://learn.microsoft.com/openspecs/exchange_server_protocols/ms-oxcmail/) (MAPI to MIME mapping)

## Other implementations

- [outlook-pst-rs](https://github.com/microsoft/outlook-pst-rs)
- [java-libpst](https://github.com/rjohnsondev/java-libpst)
- [pstreader](https://github.com/Jmcleodfoss/pstreader)
- [libpff](https://github.com/libyal/libpff)
- [XstReader](https://github.com/Dijji/XstReader)
- [PANhunt](https://github.com/Dionach/PANhunt/blob/master/pst.py)

## Credits

go-pst was written by Marten Mooij ([Mooij Technologies](https://github.com/mooijtech)),
with contributions from the upstream contributors. Upstream thanked
[James McLeod](https://github.com/Jmcleodfoss) (pstreader) for his help.

## License

Apache License 2.0, see [LICENSE.txt](LICENSE.txt). The original copyright
notices in the source files are kept.
