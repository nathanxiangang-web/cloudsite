"""Launch pinned external query-only IndexCore; never logs its DSN."""
import os,stat,sys
from pathlib import Path
try:
 path=Path('/run/catalog/database.url');info=path.lstat()
 if not stat.S_ISREG(info.st_mode) or info.st_uid!=os.getuid() or info.st_mode&0o077:raise ValueError()
 value=path.read_text().strip()
 if not value or '\n' in value:raise ValueError()
 env={k:os.environ[k] for k in ('PATH','HOME','LANG') if k in os.environ};env['INDEXCORE_DATABASE_URL']=value
 os.execve('/usr/local/bin/cloudsite-index-reader',['cloudsite-index-reader'],env)
except Exception:
 sys.stderr.write('cloudsite catalog startup refused\n');sys.exit(1)
