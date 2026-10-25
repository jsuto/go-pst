S/MIME samples for `smime_test.go`, generated with OpenSSL 3.6. `signer.key`
is a throwaway key of a made-up identity, only for these tests.

```bash
openssl req -x509 -newkey rsa:2048 -nodes -keyout signer.key -out signer.crt -days 36500 \
  -subj "/CN=Test Signer/emailAddress=signer@example.com"
printf 'Content-Type: text/plain; charset=utf-8\r\n\r\nThis is a signed test message.\r\nÁrvíztűrő tükörfúrógép.\r\n' > content.txt
openssl smime -sign -in content.txt -signer signer.crt -inkey signer.key -out signed.eml
openssl smime -encrypt -binary -aes256 -outform DER -in content.txt -out enveloped.p7m signer.crt
openssl smime -sign -nodetach -binary -outform DER -in content.txt -signer signer.crt -inkey signer.key -out opaque-signed.p7m
```
