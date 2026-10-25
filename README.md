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

Work in progress. Planned, roughly in this order:

- sender SMTP address
- ANSI (32-bit) PST files (upstream #52) and local descriptors with level > 0
  (upstream #45)
- attachment MIME type, Content-ID, inline flag and embedded messages
- plain text, HTML and compressed RTF bodies (including HTML encapsulated
  in RTF)
- a converter package and `cmd/pst2eml`

Changes compared to upstream v6.0.2:

- module path `github.com/jsuto/go-pst`, Go 1.27, updated dependencies
- the Linux io_uring reader (`NewAsync`) was removed; `File.Reader` is a
  plain `io.ReaderAt`
- recipient table: `Message.GetRecipients` returns the To/Cc/Bcc recipients
  with display name, address type, email address and SMTP address

## Usage

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
