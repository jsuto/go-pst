import sys, os, email, collections
from email import policy
from email.utils import getaddresses, parsedate_to_datetime
root = sys.argv[1]
c = collections.Counter(); ex = collections.defaultdict(list)
files = [os.path.join(d, f) for d, _, fs in os.walk(root) for f in fs if f.endswith('.eml')]
for p in files:
    m = email.message_from_binary_file(open(p, 'rb'), policy=policy.compat32)
    c['total'] += 1
    for h in ['From', 'To', 'Cc', 'Bcc', 'Message-ID', 'Date', 'Subject', 'Received', 'X-From', 'X-To', 'MIME-Version', 'Content-Type']:
        if m.get(h) is not None: c['has ' + h] += 1
    for h in ['From', 'To', 'Cc']:
        v = m.get_all(h) or []
        addrs = getaddresses([str(x) for x in v])
        if v and not any('@' in a for _, a in addrs):
            c[h + ' without an @ address'] += 1; ex[h].append((p, str(v[0])[:100]))
    if m.get('From') and len(m.get_all('From')) > 1: c['multiple From'] += 1
    mid = m.get('Message-ID')
    if mid and not (mid.strip().startswith('<') and '@' in mid): c['odd Message-ID'] += 1; ex['mid'].append((p, mid))
    if not mid: ex['nomid'].append(p)
    try:
        if m.get('Date'): parsedate_to_datetime(m['Date'])
    except Exception: c['bad Date'] += 1; ex['date'].append((p, m['Date']))
    if m.defects: c['parser defects'] += 1; ex['defects'].append((p, [type(d).__name__ for d in m.defects]))
    c['ctype ' + m.get_content_type()] += 1
for k, v in sorted(c.items()): print(f'{v:6d}  {k}')
for k, v in ex.items():
    print('--', k, len(v)); [print('   ', x) for x in v[:4]]
