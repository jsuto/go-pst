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
	var noTo, toInHdr, smtpSender, exSender int
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
			for _, id := range os.Args[2:] {
				if strings.Contains(h, id) {
					fmt.Printf("== %s\n--- transport headers:\n%s\n--- DisplayTo=%q DisplayCc=%q SenderAddressType=%q SenderEmailAddress=%q InternetMessageId=%q\n",
						folder.Name, h, m.GetDisplayTo(), m.GetDisplayCc(), m.GetSenderAddressType(), m.GetSenderEmailAddress(), m.GetInternetMessageId())
				}
			}
		}
		return nil
	})
	fmt.Printf("headers with To: %d, without: %d; sender type SMTP=%d EX=%d\n", toInHdr, noTo, smtpSender, exSender)
}
