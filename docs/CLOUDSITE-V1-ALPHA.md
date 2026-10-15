# cloudsiteV1.0.0alpha release and deployment contract

Direct human authorization, 2026-10-15; Issue #98. Repository ID1391307332
is now `nathanxiangang-web/cloudsite`. The legacy CloudSite repository was
deleted by the human, not by this release tool. Its verified Git mirror/bundle
and metadata export are private backups, not a promise to restore GitHub IDs,
stars/fork relationships or release assets exactly.

## Version and compatibility

- Public product name: `cloudsite`; exact release/tag: `cloudsiteV1.0.0alpha`.
- Rebrand consumer, administrator and guest pages, titles and current Markdown
  documentation. Keep historical accepted evidence and clearly label it as such.
- Preserve internal Go module imports, `cmd/panta`, `PANTA_*` settings, cookies,
  schema14 database names and existing data. These are compatibility identifiers,
  NOT the public product brand. New container binary is named `cloudsite`.
- Alpha naming does not imply unfinished admission/usage/native115 shares,
  current-DB Copy projection or all codecs are complete. No full MVP claim.

## Public registration and reused credentials

The human explicitly requests open website registration and reuse of local
credentials on the remote deployment. Restore the existing product accounts and
password hashes unchanged; do not reset passwords, elevate roles or invent a new
administrator credential. Product credentials are separate from storage and115
credentials. Transfer protected files/dumps privately; never include them in Git.

Enable `registration_enabled=true` in the new protected deployment config.
Legacy compositions default to disabled. Public capability discovery drives the
consumer login/register UI. Registration accepts only login/password, always
creates USER, rejects duplicate/unknown fields and uses the existing unique login
constraint/bcrypt policy (12-72 password bytes). No public role provisioning or
provider credentials. Host/Origin, request size, shared8-per-minute auth budget
and2 concurrent hashing requests apply. Registration issues no session: users
log in normally afterwards. Administrator rights remain operator-only.

## Runtime modules and dependency direction

```text
Existing HTTPS Traefik -> private nginx edge -> loopback cloudsite product
                                         -> loopback IndexCore query-only
cloudsite product -> owned PostgreSQL16 + known-file AList/OpenList adapter
IndexCore query-only -> owned PostgreSQL18 catalog snapshot (read-only role)
```

Keep authenticated Q5/PRESENT identity checks, guest grants, expiry/revoke and
Range semantics. No new provider acquisition, Worker, Hint or scheduled scanner.
Private edge namespace exposes only the product via existing HTTPS host route;
never PostgreSQL, IndexCore or storage credentials. Product still binds loopback.
An explicit `https_proxy=true` permits a configured HTTPS Origin behind the
proxy; plain HTTP remote origins stay forbidden and cookies remain Secure.

The external IndexCore query composition is built separately inside the pinned
IndexCore source tree, not imported into cloudsite. Preserve its exact template
and binary hashes; do not misrepresent its additional composition source as an
unmodified upstream release. Product and collector remain independently replaceable.

## Deployment scope and rollback

Only `/opt/my-servers/cloudsite1.0.yml` workload on verified113.20.6.57 is replaced.
New compose: `/opt/my-servers/cloudsiteV1.0.0alpha.yml`, new isolated project,
new owned database volumes/config/cache. Existing AList/Traefik/Portainer/backup
containers, legacy database files, old YAML and image IDs remain intact.

Before cutover: save old compose/rendered config/image identities; consistent
legacy SQLite backups; separate product/catalog dumps. Restore only new named
databases, never original staging or legacy databases. Validate correct storage
namespace and real bytes, schema14, Q6 counts, TLS/Origin/cookie/auth, download,
preview/playback/share. No passwords, tokens or signed URLs in Git or logs.
Dedicated runtime credentials/config files are protected outside the repository.

The existing target storage user ID5 is already restricted to the CloudSite
namespace with permission0. Reuse it; do not create users, reset passwords,
replace115 Cookie/session or restart AList. Its application JWT is finite-lived:
the new product reads its protected token file for each FS access, and a dedicated
12-hour operator timer renews ONLY the same storage-application login. Verify
user ID/base_path/permission and future expiry before atomic replacement; leave
the old token on failure. No service restart, scan, native115 login or download
is part of renewal. Monitor this timer and credential expiry independently.

Start candidate without public routing; require all three exact-head CI groups
and separate same-agent review. Stop only the two legacy CloudSite containers,
enable the new existing-host route, then verify actual HTTPS. On failure disable
new route and restart old compose; do not delete volumes or original files.
No speculative "deployed" or universal codec/realtime-indexing claims.
