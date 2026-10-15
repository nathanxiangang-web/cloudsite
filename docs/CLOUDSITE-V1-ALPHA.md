# cloudsiteV1.0.0alpha release and deployment contract

Direct human authorization, 2026-10-15; Issue #98. Repository ID1391307332
is now `nathanxiangang-web/cloudsite`. The legacy CloudSite repository was
deleted by the human, not by this release tool. Its verified Git mirror/bundle
and metadata export are private backups, not a promise to restore GitHub IDs,
stars/fork relationships or release assets exactly.

## Latest human-authorized runtime changes — supersede initial AList boundary

The human explicitly authorizes root-compose AList removal/replacement with
OpenList, using LOCAL OpenList configuration/credentials and abandoning server
AList credential migration. This supersedes the initial preserve-AList scope
below. Only the alist service in `/opt/my-servers/docker-compose.yml` is replaced;
Traefik/Portainer/rclone-backup IDs/images/PIDs remain unchanged. The old alist
container was stopped and removed, not its original bind data or rollback image.

Current OpenList: v4.2.6, image digest
`sha256:c555c6e1c8af2aead38ed12ec761ac077fdf046d19cf033414be8e056aec6b64`,
container `openlist`, copied private data `/opt/my-servers/openlist/data`, existing
HTTPS route https://pan.netioi.com. User0:0 matches the local instance; no host
port is added. Internal alias alist preserves network compatibility only; it is
NOT an alist service/container. Product source is explicitly `http://openlist:5244`.

Local config/database were copied consistently, with4 historical transfer-queue
rows omitted ONLY from the new copy before startup. Original local data and
historical acquisition reservations unchanged. Copied JWT failed validation;
the existing local OpenList app username/password successfully log in, with no
native115 login or account/password reset. Storage user3, base_path
`/115open/CloudSite`, permission65341 preserved exactly; do NOT call this
permission0 or claim a new permission-hardening pass. Product still only exposes
known-file byte reads after canonical/auth checks, not storage write APIs.

PR103 reviewedcfc283178bb5, CI38754182448 all3 SUCCESS, same-agent review
5535047887, merge7bfe63d7af4c: protected renewal specifies container_name openlist
and expected_identity id/base_path/permission; default AList identity retained
for old deployments only. Renewal uses APPLICATION login, not native115 login.
The existing timer is re-enabled after the migrated identity is verified.

PR102 reviewed22705d031adc, CI38748673995 all3 SUCCESS, review5534538189,
merge68d945793b8f, merge CI38749342436 SUCCESS. Actual public HTTPS tests verify
6-character signup/login,5-character rejection and unchanged original account.
Post-OpenList-switch real regression verifies283 canonical files, DOCX15423-byte
attachment/text preview, video206 Range and both share-code modes; temporary
shares revoked. The canonical snapshot and schema14 remain unchanged.

Protected rollback directory:
`/opt/my-servers/cloudsiteV1.0.0alpha/private/backups/openlist-replacement`.
It contains the exact previous root YAML, consistent AList SQLite/config, old
image/container identities and previous product access config/reader binding.
Original `/opt/my-servers/alist/data` stays intact; image tag
`alist:rollback-before-openlist-20261015` remains. Restore only that service with
the original project directory and `--pull never`, never run a whole-stack down
or delete database volumes. Product/token binding must roll back together.

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
constraint/bcrypt policy (at least6 Unicode characters, at most72 UTF-8 bytes,
no complexity requirement). No public role provisioning or
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

## Verified initial deployment — 2026-10-15

PR99 reviewed HEAD93606e67ccf2, CI38745269165 all3 SUCCESS, same-agent review
5534142444, merge7368671aa0af. PR100 fixes only a quoted tmpfs flow-list item:
reviewed HEAD7a8ab8fe1694, CI38746403395 all3 SUCCESS, same-agent review5534272835,
merge841c4d14fb54, merge CI38746768026 all3 SUCCESS. These are same-agent reviews,
not independent review. Issue101 subsequently changes the minimum password from
12 bytes to6 characters on direct human request; it does not reset existing hashes.

The target was rechecked before cutover: legacy cloudsite1 containers and original
YAML had disappeared externally after backup. Do NOT claim the agent stopped
them. Legacy data, backed-up YAML/rendered config/SQLite databases and exact
rollback image tags remain. Existing AList/Traefik/Portainer/backup IDs, images,
PIDs were checked unchanged. Only the new isolated alpha project was started.

Candidate and actual public HTTPS tests passed, with normal TLS verification:
original local USER/password login; ordinary admin denial; registration/duplicate
rejection/login;283 unique canonical files;15423-byte DOCX attachment and text
preview; video206 Range; code/no-code guest download matching the document hash.
Document SHA2562bc8090eca36718b3c682ba66f9b3f1a97b88be0d850a1f7c55f1ffedf298925.
Four temporary test shares were revoked (two candidate/two public). One public
release-test USER is retained with a private random password, not silently purged.
No source file, provider reservation, original staging DB or native115 session was
changed. Normal public website account/share records exist only in the new DB.

Public URL https://cloud.netioi.com; listener/query/database ports stay private.
Runtime config/backups/receipts are under the protected release directory;
`private/local-product-user.json` is an operator-only copy of the local login,
not a new administrator or password reset. Existing legacy SQLite accounts are
not automatically compatible with the Go product auth model. Local product auth
hashes/roles are copied unchanged. The storage account is the target's existing
same-namespace restricted reader, not a copied/relogged native115 session.

Formal restart must include the protected env file, otherwise routing defaults
OFF: `docker compose --env-file /opt/my-servers/cloudsiteV1.0.0alpha/private/formal.env
-f /opt/my-servers/cloudsiteV1.0.0alpha.yml up -d`. Recreating edge requires
recreating its namespace-sharing catalog/product processes in order. Token
timer is `cloudsite-alpha-storage-token.timer`; only application login is renewed.
No scan daemon is installed: newly changed drive files require a separate bounded
canonical refresh, not a claim of real-time indexing. No UI/API file deletion.
