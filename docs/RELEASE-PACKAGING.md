# Reproducible embedded migration packaging

The migration module owns append-only SQL and raw SHA-256 compatibility. Release
packaging must preserve the published bytes; it must not repair compatibility
by changing schema_migrations or bypassing the startup check.

## Ownership and dependency boundary

- Git/release packaging owns transport and line endings, not schema semantics.
- migrations embeds the exact SQL bytes; postgres validates recorded history.
- productserver must reject incompatible history before starting its worker.
- A packaging-only change must not introduce a migration or reset any Job.

## Build and admission

Use the reviewed commit, including on Windows:

```sh
git -c core.autocrlf=false archive --format=tar.gz --output=release.tgz <reviewed-head>
```

The repository pins SQL to LF with .gitattributes. Before a live replacement,
compare every archived SQL checksum to the existing applied history using a
read-only connection, then run the existing preflight-product-acquisition
action against protected runtime configuration. It constructs dependencies but
does not Claim, Submit, StartDownload or migrate.

After switching only the owned product image in a quiet worker interval, require
container health AND the expected public asset/API response. Roll back the image
automatically on failure; retaining a deployment receipt alone is not success.
Keep the original binary/compose and failed attempt evidence.

## Incident and extension constraints

On 2026-10-18 a Windows archive with core.autocrlf=true converted SQL to CRLF.
All 15 checksums differed from the deployed LF history. The candidate refused
startup, and the prior healthy product image was restored. An explicit LF archive
matched all 15 recorded checksums without changing the database. SQL content and
schema version15 were unchanged. Preserve this admission for future releases;
new migrations need their own reviewed append-only change, not an exception here.
