"""Back up and restore the running local stack into an isolated database and volume.

Never restores over the live deployment. The isolated database, restored
snapshots and drill containers are removed after integrity checks.
"""
import json,shutil,subprocess,tempfile,uuid
from pathlib import Path
root=Path(__file__).resolve().parent.parent
podman=shutil.which('podman') or r'C:\Program Files\RedHat\Podman\podman.exe'
suffix=uuid.uuid4().hex[:10];database='restore_'+suffix;volume='autodit_restore_'+suffix
def run(*args,**kwargs):return subprocess.run([podman,*args],check=True,**kwargs)
def sql(query):return subprocess.check_output([podman,'exec','autodit_postgres_1','psql','-U','autodit_owner','-d',database,'-At','-c',query],text=True).strip()
compose=['compose','--env-file',str(root/'.env'),'-f',str(root/'deploy/compose/compose.json')]
with tempfile.TemporaryDirectory(prefix='autodit-restore-') as temp:
 path=Path(temp)
 try:
  run(*compose,'stop','-t','120','api','worker')
  # Keep the dump in this atomically created private directory; no predictable
  # writable path is opened inside the database container.
  with (path/'database.dump').open('xb') as backup:
   run('exec','autodit_postgres_1','pg_dump','-U','autodit_owner','-d','autodit','-Fc',stdout=backup)
  run('volume','export','autodit_snapshots','--output',str(path/'snapshots.tar'))
  run('exec','autodit_postgres_1','createdb','-U','autodit_owner',database)
  with (path/'database.dump').open('rb') as backup:
   run('exec','-i','autodit_postgres_1','pg_restore','-U','autodit_owner','-d',database,'--exit-on-error',stdin=backup)
  original=subprocess.check_output([podman,'exec','autodit_postgres_1','psql','-U','autodit_owner','-d','autodit','-At','-c','select count(*) from observation'],text=True).strip()
  if sql('select count(*) from observation')!=original:raise RuntimeError('Observation count changed on restore')
  snapshots=json.loads(sql("select coalesce(json_agg(json_build_object('ref',object_ref,'hash',content_hash)),'[]') from snapshot"))
  (path/'snapshots.json').write_text(json.dumps(snapshots))
  (path/'verify.py').write_text("import json,hashlib,pathlib\nfor item in json.loads(pathlib.Path('/verify/snapshots.json').read_text()):\n p=pathlib.Path('/snapshots')/item['ref']\n assert hashlib.sha256(p.read_bytes()).hexdigest()==item['hash'], 'snapshot checksum mismatch'\nprint('Restored snapshots verified')\n")
  run('volume','create',volume)
  run('volume','import',volume,str(path/'snapshots.tar'))
  run('run','--rm','--network','none','-v',volume+':/snapshots:ro','-v',str(path)+':/verify:ro','docker.io/library/python:3.13-slim-bookworm','python','/verify/verify.py')
  print(f'Restore drill passed: {original} observations and {len(snapshots)} immutable snapshot checksums match.')
 finally:
  subprocess.run([podman,'exec','autodit_postgres_1','dropdb','-U','autodit_owner','--if-exists',database],check=False)
  subprocess.run([podman,'volume','rm',volume],check=False)
  run(*compose,'start','api','worker')
