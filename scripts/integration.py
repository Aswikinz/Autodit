"""Run the real PostgreSQL/RLS/golden suite in disposable Podman containers."""
import os,subprocess,uuid,json,time,shutil
from pathlib import Path
root=Path(__file__).resolve().parent.parent
podman=shutil.which('podman') or r'C:\Program Files\RedHat\Podman\podman.exe'
suffix=uuid.uuid4().hex[:10];network='autodit-test-'+suffix;pg=network+'-pg';runner=network+'-go'
def run(*args,**kwargs):return subprocess.run([podman,*args],check=True,**kwargs)
try:
 run('network','create',network)
 run('run','-d','--name',pg,'--network',network,'-e','POSTGRES_USER=autodit_owner','-e','POSTGRES_DB=autodit','-e','POSTGRES_HOST_AUTH_METHOD=trust','docker.io/library/postgres:17-bookworm')
 for _ in range(60):
  if subprocess.run([podman,'exec',pg,'pg_isready','-U','autodit_owner','-d','autodit'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0:break
  time.sleep(1)
 else:raise RuntimeError('PostgreSQL did not become ready')
 run('exec',pg,'psql','-U','autodit_owner','-d','autodit','-c','create role autodit_app login;')
 run('run','--rm','--name',runner,'--network',network,'-v',str(root)+':/workspace','-v','autodit_test_gomod:/go/pkg/mod','-w','/workspace','-e','TEST_OWNER_DATABASE_URL=postgres://autodit_owner@'+pg+'/autodit?sslmode=disable','-e','TEST_DATABASE_URL=postgres://autodit_app@'+pg+'/autodit?sslmode=disable','docker.io/library/golang:1.27-bookworm','go','test','-race','-count=1','-coverpkg=./internal/...','-coverprofile=coverage-integration.out','./internal/...')
finally:
 subprocess.run([podman,'rm','-f',runner,pg],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 subprocess.run([podman,'network','rm',network],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
