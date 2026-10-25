import sys, os, email
from email import policy
from email.header import decode_header, make_header
def dec(v):
    if v is None: return None
    try: return str(make_header(decode_header(str(v))))
    except Exception as e: return f'!{e}! {v}'
def walk(part, depth=0, out=None):
    out = [] if out is None else out
    ct = part.get_content_type(); fn = part.get_filename()
    size = len(part.get_payload(decode=True) or b'') if not part.is_multipart() and ct != 'message/rfc822' else ''
    cid = part.get('Content-ID')
    out.append('  ' * depth + f'{ct}' + (f' name={dec(fn)}' if fn else '') + (f' cid' if cid else '') + (f' {size}B' if size != '' else ''))
    if part.is_multipart():
        for p in part.get_payload(): walk(p, depth + 1, out)
    return out
for root in sys.argv[1:]:
    for d, _, fs in sorted(os.walk(root)):
        for f in sorted(fs, key=lambda x: int(x.split('.')[0]) if x.split('.')[0].isdigit() else 0):
            if not f.endswith('.eml'): continue
            p = os.path.join(d, f)
            m = email.message_from_binary_file(open(p, 'rb'), policy=policy.compat32)
            print(f'=== {os.path.relpath(p, root)}')
            for h in ['Subject', 'From', 'To', 'Cc', 'Bcc', 'Date', 'Message-ID', 'In-Reply-To', 'References', 'Importance', 'X-Priority', 'Received']:
                v = m.get(h)
                if v is not None: print(f'  {h}: {dec(v)[:160]}')
            print('  has transport headers (Received):', 'Received' in m)
            for line in walk(m)[1:]: print('   ', line[:150])
