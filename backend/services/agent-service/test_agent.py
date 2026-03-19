import urllib.request as r; req = r.Request('http://127.0.0.1:8000/agent/query', data=b'{\
prompt\: \how
many
products
do
we
have?\}', headers={'Content-Type': 'application/json'}); print(r.urlopen(req).read().decode())
