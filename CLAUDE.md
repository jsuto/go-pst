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
- `data/32-bit.pst`: ANSI, fails today (#52).
- `data/support.pst`: from the Hacking Team leak, real people's
  addresses. Never use it as a fixture or quote its content.
- `testdata/private/` (gitignored, **never commit**): `backup.pst` (18 MB,
  an Outlook.com test account exported by classic Outlook: sent mail
  without transport headers, Cc, Bcc, RTF-only body, 14 attachments, a
  13 MB attachment, forwarded-as-attachment, meeting invites, a draft,
  `Projects/2026` folder, Deleted Items), `readpst-backup/` (readpst's
  output of it, to compare), folder counts of earlier runs. It has real
  addresses; don't paste them into commits, issues or fixtures.

Baseline of this code on `backup.pst`: 117 items (22 messages, 89+2
calendar items, 2 contacts...), 21 attachments, no errors; 11 messages
with transport headers. readpst counts 24 messages (meeting invites as
mail).

## Tools

```bash
go run ./tools/probe <file.pst> 2>folders.txt   # item/attachment counts, per folder on stderr
go run ./tools/dump <file.pst> [header substring...]  # transport headers, DisplayTo/Cc, sender of matching messages
readpst -e -8 -D -j 0 -o out/ <file.pst>        # reference output (brew install libpst)
python3 -I tools/eml-check/hdrs.py out/          # header statistics of a directory of .eml
python3 -I tools/eml-check/summary.py out/       # per message: headers and MIME structure
```

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
