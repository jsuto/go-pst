# CLAUDE.md

A fork of [mooijtech/go-pst](https://github.com/mooijtech/go-pst)
(Apache-2.0, unmaintained since 2023; the author lost access to the
account in 2024). We continue it for one purpose: reading PST files
(Outlook exports, Enterprise Vault exports) and converting their messages
to RFC 822 / MIME (EML), for the PST import of the Piler email archive.
Fix and add what that needs, nothing more (no PST writing, no OST/PAB
work unless it comes for free).

Remotes: `origin` is this fork, `upstream` the original (read-only, dead).

## Why our own reader

Tested in October 2026 (details in `../piler-ee/docs/OUTLOOK-PST.md`, a
local private repo):

- **readpst** (libpst, GPL-2.0) reads everything, but builds the headers
  of mail without transport headers (Sent Items, drafts) from
  `PR_DISPLAY_TO` / `_CC` / `_BCC`: no sender address
  (`From: "Name" <MAILER-DAEMON>`), recipients as display names only, Bcc
  only in `X-libpst-forensic-bcc`. The recipient table with
  `PR_SMTP_ADDRESS` is in the PST, readpst ignores it. RTF-only bodies end
  up as an `application/rtf` attachment, attachment MIME types are lost.
- **go-pst v6.0.2** (this code) has no recipient table at all, fails on
  ANSI PSTs (#52), and doesn't implement "local descriptors level is
  not 0" (#45).
- **microsoft/outlook-pst-rs** (MIT, Rust) is the best maintained reader,
  but a Rust toolchain doesn't fit Piler's Go build.

## Work items (roughly in order)

1. **Housekeeping:** module path `github.com/jsuto/go-pst` (no `/v6`;
   tags start at `v0.1.0`), current Go version and dependencies, README
   says it's a continuation maintained for Piler's needs, keep
   `LICENSE.txt` and credit the original authors. Check whether the
   `go-uring` dependency (`pkg/io_uring.go`) is worth keeping.
2. **Recipient table** (`IdentifierTypeRecipientTable` exists in
   `pkg/btree.go`, nothing reads it): per recipient `PR_RECIPIENT_TYPE`
   (To/Cc/Bcc), `PR_DISPLAY_NAME`, `PR_SMTP_ADDRESS`, `PR_EMAIL_ADDRESS`,
   `PR_ADDRTYPE`.
3. **Sender SMTP address:** `PR_SENDER_SMTP_ADDRESS`,
   `PR_SENT_REPRESENTING_SMTP_ADDRESS` (and the named/PidTag variants).
   The sender is stored as an Exchange X.500 address
   (`/O=FIRST ORGANIZATION/OU=EXCHANGE ADMINISTRATIVE
   GROUP(FYDIBOHF23SPDLT)/CN=RECIPIENTS/CN=...`) even for a consumer
   Outlook.com account.
4. **Bugs:** #45 (local descriptors level > 0; needs a PST that triggers
   it, probably large messages or many attachments), #52 (wrong ANSI
   encryption offset; `data/32-bit.pst` fails with "property type is not
   the same as the value expected from the caller").
5. **Attachments:** `PR_ATTACH_MIME_TAG`, `PR_ATTACH_CONTENT_ID`, inline
   flag, embedded messages (`PR_ATTACH_METHOD` = embedded message) as a
   message that can be converted recursively (upstream #10).
6. **Bodies:** plain text, HTML (`PR_HTML` is binary, charset from
   `PR_INTERNET_CPID`), compressed RTF (`pkg/rtf.go`, MS-OXRTFCP): RTF
   that encapsulates HTML (MS-OXRTFEX) back to HTML, other RTF to text.
7. **Converter package and `cmd/pst2eml`** (the rules are below), with a
   report per folder: items read, converted, skipped (with the reason).
8. **Tests:** small fixtures without personal data (generated in Outlook
   with made-up addresses, or synthetic), plus robustness runs on
   `data/enron.pst`.

## Later

Found or deferred while working on the items above:

- **Test PST for #45** (subnode SIBLOCK, implemented in v0.1.4 and only
  tested with synthetic blocks): a level 0 subnode block holds 340
  entries (Unicode), so it needs a message with more than ~340 subnodes,
  i.e. hundreds of attachments. With classic Outlook on Windows:
  ```powershell
  $dir = "$env:TEMP\att"; New-Item -ItemType Directory -Force $dir | Out-Null
  1..400 | % { Set-Content "$dir\file$_.txt" "attachment $_" }
  $ol = New-Object -ComObject Outlook.Application
  $mail = $ol.CreateItem(0)
  $mail.Subject = "400 attachments"; $mail.To = "test@example.com"; $mail.Body = "test"
  Get-ChildItem $dir | % { [void]$mail.Attachments.Add($_.FullName) }
  $mail.Save()   # lands in Drafts, then export the PST
  ```
  Or run `tools/probe` on large real PSTs (upstream's report came from
  `WalkFolders` on unknown files, maybe large folders).
- **Large PSTs:** tested with the two largest files of the EDRM Enron set
  as republished on Hugging Face (`intellekthq/enron-ferc-pst`, CC BY 3.0
  US, 171 PSTs, 77 GiB, up to 1.96 GiB each, made by the same ZL
  Technologies tool as `data/enron.pst`; downloaded to
  `../piler-ee/testdata/pst/large/`): `zl_dasovich-j_000.pst` (1.96 GiB,
  33422 messages) and `_001` (1.88 GiB, 36593), about 12 s each, at most
  190 MB resident; same message count as readpst, 12121 of 12123
  attachments identical to readpst's. Not tested: PSTs over 4 GiB (no
  public one is known) and real Exchange or Outlook exports of that size.
- **S/MIME from a real PST:** the converter rebuilds S/MIME messages from
  their entity (tested with OpenSSL samples, `pkg/eml/testdata/smime`); no
  test PST has an S/MIME message, so the PST side (class, attachment) is
  untested.
- **ANSI test PST with mail** ("Outlook 97-2002" format) with non-ASCII
  text, e.g. Hungarian, to test String8 decoding with
  `PR_MESSAGE_CODEPAGE` on real data (`data/32-bit.pst` is ASCII only, one
  appointment).
- **Named properties** (0x8000 and up) are never populated into the
  generated structs (their keys use IDs >= 0x40000). None of the fields
  needed for EML conversion is named; if one is needed, look up its ID via
  `File.NameToIDMap`.
- **Meeting requests** are converted as mail with their HTML body; readpst
  adds a `text/calendar` part built from the properties. Not done.
- Attachments without data (`PR_ATTACH_DATA_BIN` empty, seen in
  `backup.pst`: two PDFs) are written as empty parts; readpst drops them.

## Converter rules

- Message with `PR_TRANSPORT_MESSAGE_HEADERS`: use those headers, they
  are the original ones; rebuild only the MIME body structure.
- Without them (sent mail, drafts, items created in Outlook): build the
  headers from the properties: From from the sender SMTP properties
  (fall back to `PR_SENDER_EMAIL_ADDRESS` only if it's SMTP), To/Cc/Bcc
  from the recipient table with addresses (quote display names properly,
  RFC 2047 for non-ASCII), Message-ID (`PR_INTERNET_MESSAGE_ID`), Date
  (`PR_CLIENT_SUBMIT_TIME`, else `PR_MESSAGE_DELIVERY_TIME`),
  In-Reply-To / References, Importance.
- `PR_SUBJECT` in a PST can start with 0x01 and a length character
  marking the prefix ("RE: ", "Updated: "); strip those two characters.
- An X.500 address that has no SMTP property is written as it is; the
  importer (Piler) maps it with a `legacyExchangeDN` to `mail` table. No
  Piler-specific code in this repo.
- Bodies: HTML and plain text as they are (multipart/alternative when
  both), RTF as described above, attachments with their MIME type and
  Content-ID, embedded messages as `message/rfc822`.
- Non-mail items (`IPM.Appointment`, `IPM.Contact`, ...) are counted and
  skipped by default. EV shortcut stubs (`IPM.Note.EnterpriseVault.Shortcut`)
  are reported, not converted as messages.

## Test data

- `data/enron.pst` (EDRM v2, Unicode, 29 MB): 2178 messages (1737 outside
  Deleted Items), all with transport headers, which are synthetic and
  broken (X.500 addresses in odd syntax). Good for robustness, not for
  address fidelity.
- `data/32-bit.pst`: ANSI, 64 KB, one appointment (no mail), ASCII only.
- `data/support.pst`: from the Hacking Team leak, real people's
  addresses. Never use it as a fixture or quote its content.
- Private test data, outside of this repository in `../piler-ee/testdata/pst/`
  (excluded locally via piler-ee's `.git/info/exclude`, **never commit**
  it in either repository): `backup.pst` (18 MB, an Outlook.com test
  account exported by classic Outlook: sent mail without transport
  headers, Cc, Bcc, RTF-only body, 14 attachments, a 13 MB attachment,
  forwarded-as-attachment, meeting invites, a draft, `Projects/2026`
  folder, Deleted Items), `readpst-backup/` (readpst's output of it, to
  compare), folder counts of earlier runs (`gopst-backup-folders.txt` is
  the baseline of `tools/probe`). It has real addresses and a real name;
  don't paste them into commits, issues or fixtures.
  The tests using it run only with the directory in `GO_PST_TESTDATA`:
  ```bash
  GO_PST_TESTDATA=../piler-ee/testdata/pst go test ./...
  ```

Baseline on `backup.pst`: 117 items (22 messages, 2 meeting requests, 91
appointments, 2 contacts), 21 attachments, no errors; 11 messages with
transport headers. `pst2eml` converts 24 (as readpst) and skips 93
non-mail items.

## Tools

```bash
go run ./cmd/pst2eml -o out/ <file.pst>          # convert to EML, report per folder
go run ./tools/probe <file.pst> 2>folders.txt   # item/attachment counts, per folder on stderr
tools/fuzz.sh [5m]                               # all fuzz targets, 4 workers (at most 5 processes)
tools/build-release.sh v0.2.0                     # pst2eml release archives into dist/
go run ./tools/dump <file.pst> [header substring...]  # transport headers, DisplayTo/Cc, sender of matching messages
readpst -e -8 -D -j 0 -o out/ <file.pst>        # reference output (brew install libpst)
python3 -I tools/eml-check/hdrs.py out/          # header statistics of a directory of .eml
python3 -I tools/eml-check/summary.py out/       # per message: headers and MIME structure
```

Fuzzing: `tools/fuzz.sh` (about 5 minutes per target) before merging changes
to the parsing code (b-trees, heap, table/property contexts, local
descriptors, RTF, the converter); a long run (an hour or overnight per
target) before a release Piler picks up. Never more than 5 test processes at
once (`-parallel 4`). Crashing inputs stay in `testdata/fuzz/` and run with
every `go test`.

## References

- [MS-PST](https://learn.microsoft.com/openspecs/office_file_formats/ms-pst/),
  [MS-OXPROPS](https://learn.microsoft.com/openspecs/exchange_server_protocols/ms-oxprops/)
  (property tags), [MS-OXCMAIL](https://learn.microsoft.com/openspecs/exchange_server_protocols/ms-oxcmail/)
  (MAPI to MIME mapping), MS-OXRTFCP (compressed RTF), MS-OXRTFEX (HTML in
  RTF).
- microsoft/outlook-pst-rs (MIT): reference implementation, reading its
  code is fine, credit it if code is ported.
- libpst and libpff are GPL/LGPL: use them to compare behavior, never copy
  their code into this Apache-2.0 repo.

## Consumer

Piler's `pilerimport -pst` (in the private piler-ee repo, `../piler-ee`)
will use this as a Go module; during development via a `go.work` or
`replace` there, never committed. It adds the Piler-specific parts (X.500
mapping, skipping, archiving).
