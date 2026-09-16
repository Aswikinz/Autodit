"""Create a portable offline deployment bundle with images and SHA-256 checksums."""
import hashlib,json,shutil,subprocess,tarfile,tempfile
from datetime import datetime, timezone
from pathlib import Path
root=Path(__file__).resolve().parent.parent
podman=shutil.which('podman') or r'C:\Program Files\RedHat\Podman\podman.exe'
version=json.loads((root/'deploy/templates/stack.json').read_text())['version']
dist=root/'dist';dist.mkdir(exist_ok=True)
with tempfile.TemporaryDirectory(prefix='autodit-bundle-') as temp:
 stage=Path(temp)/'autodit';stage.mkdir()
 for directory in ['deploy','scripts','docs','rulepack','test/fixtures']:
  source=root/directory
  if source.exists():shutil.copytree(source,stage/directory,ignore=shutil.ignore_patterns('__pycache__','*.pyc'))
 for name in ['.env.example','README.md','LICENSE','THIRD_PARTY_NOTICES.md','go.mod','go.sum']:
  if (root/name).exists():shutil.copy2(root/name,stage/name)
 (stage/'web').mkdir()
 for name in ['package.json','package-lock.json']:
  shutil.copy2(root/'web'/name,stage/'web'/name)
 images=stage/'dist/images';images.mkdir(parents=True)
 stack=json.loads((stage/'deploy/compose/compose.json').read_text())
 references=sorted({s['image'] for s in stack['services'].values()})
 image_metadata=[]
 for i,ref in enumerate(references):
  info=json.loads(subprocess.check_output([podman,'image','inspect',ref],text=True))[0]
  subprocess.run([podman,'save','--format','oci-archive','--output',str(images/f'image-{i}.tar'),ref],check=True)
  image_metadata.append({'reference':ref,'id':info['Id'],'digest':info.get('Digest'),'architecture':info['Architecture'],'os':info['Os']})
 (stage/'image-manifest.json').write_text(json.dumps(image_metadata,indent=2)+'\n')
 try:
  revision=subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True,stderr=subprocess.DEVNULL).strip()
  dirty=subprocess.run(['git','diff','--quiet','HEAD'],cwd=root,check=False).returncode!=0
 except (OSError,subprocess.CalledProcessError):
  revision=None;dirty=None
 (stage/'release.json').write_text(json.dumps({'version':version,'created_at':datetime.now(timezone.utc).isoformat(),'source_revision':revision,'tracked_source_modified':dirty,'integrity':'sha256','signed':False},indent=2)+'\n')
 hashes=[]
 for file in sorted(stage.rglob('*')):
  if file.is_file():
   h=hashlib.file_digest(file.open('rb'),'sha256').hexdigest();hashes.append(h+'  '+file.relative_to(stage).as_posix())
 (stage/'checksums.txt').write_text('\n'.join(hashes)+'\n')
 target=dist/f'autodit-{version}-offline.tar.gz'
 def permissions(info):
  info.mode=0o755 if info.isdir() or info.name.endswith('.sh') else 0o644
  return info
 with tarfile.open(target,'w:gz') as archive:archive.add(stage,arcname='autodit',filter=permissions)
 digest=hashlib.file_digest(target.open('rb'),'sha256').hexdigest()
 target.with_suffix(target.suffix+'.sha256').write_text(digest+'  '+target.name+'\n')
 print(f'Created {target.name} ({target.stat().st_size//1048576} MiB). Development bundle; checksum integrity only.')
