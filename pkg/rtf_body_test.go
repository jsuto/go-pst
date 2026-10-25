package pst_test

import (
	"strings"
	"testing"

	pst "github.com/jsuto/go-pst/pkg"
)

func TestConvertRTFText(t *testing.T) {
	for _, test := range []struct {
		name string
		rtf  string
		want string
	}{
		{
			"code pages of fonts, unicode",
			`{\rtf1\ansi\ansicpg1252\deff0{\fonttbl{\f0\fswiss\fcharset0 Arial;}{\f1\fswiss\fcharset238 Arial CE;}}` +
				`{\colortbl;\red0\green0\blue255;}\pard\f0 Caf\'e9\par \f1 \'e1rv\'edzt\'fbr\'f5\par\uc1\u337?\u369?}`,
			"Café\nárvíztűrő\nőű",
		},
		{
			"from text, ignored destinations",
			"{\\rtf1\\ansi\\ansicpg1252\\fromtext \\fbidis \\deff0{\\fonttbl\r\n{\\f0\\fmodern Courier New;}}\r\n" +
				`{\*\generator Microsoft Exchange Server;}{\info{\author x}}{\*\pnseclvl1\pnucrm}` +
				`\uc1\pard\plain\deftab360 \f0\fs20 Hello\tab world\par {\pict\wmetafile8 0102}line 2\line {\*\unknown x}end}`,
			"Hello\tworld\nline 2\nend",
		},
		{
			"escaped characters, fields, unicode skip count",
			`{\rtf1\ansi {\field{\*\fldinst HYPERLINK "https://example.com"}{\fldrslt link}} \{x\}\\ \uc2\u8364\'80\'80 euro\~sign}`,
			"link {x}\\ € euro\u00A0sign",
		},
	} {
		got := pst.ConvertRTF(test.rtf)

		if got.IsHTML() {
			t.Errorf("%s: got HTML %q", test.name, got.HTML)
		} else if got.Text != test.want {
			t.Errorf("%s: got %q, want %q", test.name, got.Text, test.want)
		}
	}
}

// TestConvertRTFHTML de-encapsulates HTML, references [MS-OXRTFEX].
func TestConvertRTFHTML(t *testing.T) {
	rtf := "{\\rtf1\\ansi\\ansicpg1252\\fromhtml1 \\fbidis \\deff0{\\fonttbl\r\n" +
		"{\\f0\\fswiss\\fcharset0 Arial;}\r\n" +
		"{\\f1\\fmodern Courier New;}\r\n" +
		"{\\f2\\fnil\\fcharset2 Symbol;}\r\n" +
		"{\\f3\\fmodern\\fcharset0 Courier New;}\r\n" +
		"{\\f4\\fswiss\\fcharset238 Arial CE;}}\r\n" +
		"{\\colortbl\\red0\\green0\\blue0;\\red0\\green0\\blue255;}\r\n" +
		"\\uc1\\pard\\plain\\deftab360 \\f0\\fs24 \r\n" +
		"{\\*\\htmltag19 <html>}\r\n" +
		"{\\*\\htmltag34 <head>}\r\n" +
		"{\\*\\htmltag1 \\par }\r\n" +
		"{\\*\\htmltag241 <style>}\r\n" +
		"{\\*\\htmltag241 p \\{ color: red; \\}}\r\n" +
		"{\\*\\htmltag249 </style>}\r\n" +
		"{\\*\\htmltag41 </head>}\r\n" +
		"{\\*\\htmltag50 <body>}\\htmlrtf {\\htmlrtf0 \r\n" +
		"{\\*\\htmltag64 <p>}\\htmlrtf {\\htmlrtf0 Caf\\'e9 \\f4 \\'f5\\'fb &amp; \\u8364?\\htmlrtf \\par }\\htmlrtf0 \r\n" +
		"{\\*\\htmltag72 </p>}\r\n" +
		"{\\*\\mhtmltag84 <img src=\"cid:image001.png@01D00000.00000000\">}{\\*\\htmltag84 <img src=\"cid:image001.png@01D00000.00000000\">}\\htmlrtf {\\pict\\pngblip 89504e47}\\htmlrtf0 \r\n" +
		"{\\*\\htmltag58 </body>}\r\n" +
		"{\\*\\htmltag27 </html>}}"

	want := "<html><head>\n<style>p { color: red; }</style></head><body><p>Café őű &amp; €</p>" +
		"<img src=\"cid:image001.png@01D00000.00000000\"></body></html>"

	got := pst.ConvertRTF(rtf)

	if !got.IsHTML() {
		t.Fatalf("got text %q, want HTML", got.Text)
	} else if got.HTML != want {
		t.Errorf("got  %q\nwant %q", got.HTML, want)
	}
}

// TestConvertRTFMalformed checks input that used to overflow the stack or the position.
func TestConvertRTFMalformed(t *testing.T) {
	for name, rtf := range map[string]string{
		"repeated \\*":      "{\\rtf1 {" + strings.Repeat("\\*", 1000000) + "\\fonttbl x} text}",
		"huge \\bin":        "{\\rtf1 a\\bin9223372036854775807 b}",
		"negative \\bin":    "{\\rtf1 a\\bin-5 b}",
		"huge \\u and \\uc": "{\\rtf1 \\uc99999999999\\u99999999999 a}",
	} {
		_ = pst.ConvertRTF(rtf)
		t.Logf("%s: ok", name)
	}

	if got := pst.ConvertRTF("{\\rtf1 {\\*\\*\\unknown hidden}shown}").Text; got != "shown" {
		t.Errorf("got %q, want %q", got, "shown")
	}
}

func FuzzConvertRTF(f *testing.F) {
	f.Add(`{\rtf1\ansi\fromhtml1 {\*\htmltag19 <html>}\htmlrtf {\htmlrtf0 x\'e9\u8364?}}`)
	f.Add(`{\rtf1{\fonttbl{\f1\fcharset238 x;}}\f1\'f5\bin3 abc\uc0\u-3}`)

	f.Fuzz(func(t *testing.T, rtf string) {
		_ = pst.ConvertRTF(rtf)
	})
}
