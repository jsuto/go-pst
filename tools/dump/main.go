package main

import (
	"fmt"
	"os"
	"strings"

	charsets "github.com/emersion/go-message/charset"
	pst "github.com/jsuto/go-pst/pkg"
	"github.com/jsuto/go-pst/pkg/properties"
	"golang.org/x/text/encoding"
)

func main() {
	pst.ExtendCharsets(func(name string, enc encoding.Encoding) { charsets.RegisterEncoding(name, enc) })
	f, _ := os.Open(os.Args[1])
	p, err := pst.New(f)
	if err != nil {
		panic(err)
	}
	var noTo, toInHdr, smtpSender, exSender, recipientErrors int
	recipientTypes := map[pst.RecipientType]int{}
	var recipientsWithSMTP, recipientsTotal int
	p.WalkFolders(func(folder *pst.Folder) error {
		it, err := folder.GetMessageIterator()
		if err != nil {
			return nil
		}
		for it.Next() {
			m, ok := it.Value().Properties.(*properties.Message)
			if !ok {
				continue
			}
			h := m.GetTransportMessageHeaders()
			if !strings.Contains(h, "\nTo:") && !strings.HasPrefix(h, "To:") {
				noTo++
			} else {
				toInHdr++
			}
			if m.GetSenderAddressType() == "SMTP" {
				smtpSender++
			} else if m.GetSenderAddressType() == "EX" {
				exSender++
			}
			recipients, err := it.Value().GetRecipients()
			if err != nil {
				recipientErrors++
				fmt.Fprintf(os.Stderr, "%s: %q: recipients: %v\n", folder.Name, m.GetSubject(), err)
			}
			for _, r := range recipients {
				recipientsTotal++
				recipientTypes[r.Type]++
				if r.SMTPAddress != "" {
					recipientsWithSMTP++
				}
			}
			for _, id := range os.Args[2:] {
				if strings.Contains(h, id) || strings.Contains(m.GetSubject(), id) {
					fmt.Printf("== %s\n--- transport headers:\n%s\n--- Subject=%q DisplayTo=%q DisplayCc=%q DisplayBcc=%q SenderAddressType=%q SenderEmailAddress=%q InternetMessageId=%q\n",
						folder.Name, h, m.GetSubject(), m.GetDisplayTo(), m.GetDisplayCc(), m.GetDisplayBcc(), m.GetSenderAddressType(), m.GetSenderEmailAddress(), m.GetInternetMessageId())
					sender, _ := it.Value().GetSender()
					from, _ := it.Value().GetSentRepresenting()
					fmt.Printf("--- sender name=%q address=%q; sent representing name=%q address=%q\n", sender.DisplayName, sender.Address(), from.DisplayName, from.Address())
					for _, r := range recipients {
						fmt.Printf("--- recipient type=%d p1=%v name=%q addrtype=%q email=%q smtp=%q\n", r.Type, r.P1, r.DisplayName, r.AddressType, r.EmailAddress, r.SMTPAddress)
					}
					break
				}
			}
		}
		return nil
	})
	fmt.Printf("headers with To: %d, without: %d; sender type SMTP=%d EX=%d\n", toInHdr, noTo, smtpSender, exSender)
	fmt.Printf("recipients: %d (To=%d Cc=%d Bcc=%d other=%d), with SMTP address: %d, errors: %d\n", recipientsTotal,
		recipientTypes[pst.RecipientTypeTo], recipientTypes[pst.RecipientTypeCc], recipientTypes[pst.RecipientTypeBcc],
		recipientsTotal-recipientTypes[pst.RecipientTypeTo]-recipientTypes[pst.RecipientTypeCc]-recipientTypes[pst.RecipientTypeBcc],
		recipientsWithSMTP, recipientErrors)
}
