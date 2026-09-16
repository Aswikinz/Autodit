"""Enforce the supplied backend coverage floors against the merged integration profile."""
import argparse,collections,sys
from pathlib import Path
from safe_paths import confined_path
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('profile',nargs='?',choices=['coverage-integration.out','coverage.out'],default='coverage-integration.out')
profile=confined_path(Path(__file__).resolve().parent.parent,parser.parse_args().profile)
blocks={}
for line in profile.read_text().splitlines()[1:]:
 location,statements,count=line.rsplit(' ',2)
 old=blocks.get(location,(int(statements),0));blocks[location]=(int(statements),max(old[1],int(count)))
totals=collections.defaultdict(lambda:[0,0])
for location,(statements,count) in blocks.items():
 path=location.split('/internal/',1)[1].split('/',1)[0]
 for group in (path,'total'):
  totals[group][0]+=statements;totals[group][1]+=statements if count else 0
floors={'domain':95,'exceptions':90,'rules':85,'analytics':85,'ingest':80,'api':80,'storage':75,'platform':70,'total':82}
failed=False
for group,floor in floors.items():
 total,covered=totals[group];percent=100*covered/total if total else 0
 print(f'{group}: {percent:.1f}% (minimum {floor}%)')
 failed|=percent<floor
sys.exit(1 if failed else 0)
