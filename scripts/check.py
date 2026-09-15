"""Deterministic repository hygiene checks; never inspect or print client data."""
import re,subprocess,sys
from pathlib import Path
root=Path(__file__).resolve().parent.parent
errors=[]
for p in (root/'internal').rglob('*.go'):
 if p.name.endswith('_test.go'):continue
 content=p.read_text()
 for n,line in enumerate(content.splitlines(),1):
  if re.search(r'\.(Info|Warn|Error|Debug)(Context)?\(',line) and re.search(r'"(amount|balance|vendor_name|bank_account|employee_name|password|token)"',line):errors.append(f'{p.relative_to(root)}:{n}: prohibited log field')
 if '/domain/' in p.as_posix() and re.search(r'\bfloat(32|64)\b',content):errors.append(f'{p.name}: floating-point money is prohibited')
for p in (root/'cmd').rglob('*.go'):
 for n,line in enumerate(p.read_text().splitlines(),1):
  if 'logger.' in line and re.search(r'"(amount|password|token|vendor_name)"',line):errors.append(f'{p.name}:{n}: prohibited log field')
if '--logs' not in sys.argv:
 for p in [root/'deploy/compose/compose.json',root/'internal/api/openapi.json']:
  import json
  json.loads(p.read_text())
 for p in [root/'Containerfile',root/'Containerfile.proxy']:
  if 'alpine' in p.read_text().lower():errors.append(f'{p.name}: unsupported musl base image')
if errors:
 print('\n'.join(errors));sys.exit(1)
print('Repository checks passed.')
