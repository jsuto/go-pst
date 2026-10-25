package main

import (
	"fmt"
	"os"

	charsets "github.com/emersion/go-message/charset"
	pst "github.com/jsuto/go-pst/pkg"
	"github.com/jsuto/go-pst/pkg/properties"
	"golang.org/x/text/encoding"
)

func main() {
	pst.ExtendCharsets(func(name string, enc encoding.Encoding) { charsets.RegisterEncoding(name, enc) })
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	p, err := pst.New(f)
	if err != nil {
		fmt.Println("open:", err)
		return
	}
	defer p.Cleanup()
	var msgs, mails, atts, errs, hdrs int
	err = p.WalkFolders(func(folder *pst.Folder) error {
		it, err := folder.GetMessageIterator()
		if err != nil {
			return nil
		}
		n := 0
		for it.Next() {
			m := it.Value()
			msgs++
			n++
			if mp, ok := m.Properties.(*properties.Message); ok {
				mails++
				if mp.GetTransportMessageHeaders() != "" {
					hdrs++
				}
			}
			ai, err := m.GetAttachmentIterator()
			if err == nil {
				for ai.Next() {
					atts++
				}
				if ai.Err() != nil {
					errs++
				}
			}
		}
		fmt.Fprintf(os.Stderr, "%s %d\n", folder.Name, n)
		if it.Err() != nil {
			errs++
			fmt.Println("iter err:", it.Err())
		}
		return nil
	})
	fmt.Printf("%s: items=%d mails=%d with_headers=%d attachments=%d errors=%d walk=%v\n", os.Args[1], msgs, mails, hdrs, atts, errs, err)
}
