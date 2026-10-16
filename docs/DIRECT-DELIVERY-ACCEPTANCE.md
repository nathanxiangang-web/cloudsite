# Direct delivery acceptance and local/public difference

Verified2026-10-15/16, Issue105 / D-052 / PR106.

## Accepted code

- Reviewed HEAD:2f873bfa03a4bf86218a3d4d5117f721e8af009e.
- Exact-head CI38762045631: all3 SUCCESS.
- Same-agent review5535806545, NOT independent approval.
- Squash/deployed code:0b29ecc88faff5f90f03b2feacd4065ea4acaaff.
- Merge CI38762378328: all3 SUCCESS.
- Binary SHA256:dbdfb6e5baebd0fb352e756b24bd691283a41573b9860a4930158dfeb7baa922.
- Full local Go/race/vet,32 Node tests; exact-head PG16/PG18 CI pass.

## Difference and correction

The .50 d5d056b and initial public68d9457 builds BOTH streamed source bytes via
CloudSite200/206. Their content delivery branch was unchanged; local functional
playback was not302 evidence. The public server hop made this design expensive.
This was not an IndexCore removal or a reverse proxy swallowing302.

Protected exact HTTPS host configuration now enables product auth/guest grant
-> exact IndexCore Q5/PRESENT identity -> OpenList known-file URL ->302 -> CDN.
Private one-hour document preview/cache/rendering remains server-side. IndexCore
API/schema/index/ownership and283-file snapshot are unchanged.

## Actual runtime evidence

Candidate and public HTTP verification pass: anonymous denied, wrong parameters
and invalid Range denied without Location; ordinary/guest GET and HEAD302;
CDN200 document and206 video; matching full/sampled hashes. Code/no-code shares
both tested and revoked; new requests after revoke denied. No native StartDownload.

Windows same-video2MiB samples: old proxy21.140/22.850s; post-fix2.297s and next-day
2.937s, both302->206 and matching hash. These observations include link resolution,
are NOT a sustained bandwidth guarantee or a controlled simultaneous A/B test.

Real user Chrome: network trace explicitly records cloud.netioi.com302 to115 CDN
and CDN206. Bilby plays with readyState4/error=null; native keyboard seek,
pause/resume work, observed through149.027s. This proves this browser/file, not all
codecs. Playback was paused and detail closed after the test.

Native download click saved a NEW15423-byte test DOCX in the user's Downloads on
2026-10-16. SHA2562bc8090eca36718b3c682ba66f9b3f1a97b88be0d850a1f7c55f1ffedf298925
matches the full source and HTTP verification. Automated download event was NOT
observed; filesystem timestamp/size/hash provide completion evidence. Text-only
DOCX preview still renders without returning a source URL.

The legacy .50 content endpoint separately returned503/OpenList metadata500 with
a refresh marker during comparison. Its app JWT was not expired. No specific
native-cookie failure is proven and no account reset/relogin was performed.
Do not describe .50 as a currently healthy baseline using old acceptance results.

## Deployment and rollback

Only public product image and protected file-access config changed. Candidate
stopped. IndexCore/catalog/DB/edge/OpenList/Traefik/Portainer/backup identities
unchanged at cutover; application-token timer active and latest result success.
Protected receipt/config/image backup retained in direct-issue105 on the server.
Rollback retags the retained previous product image, restores old protected
access config, recreates ONLY cloudsite. No DB/data restore is required.

The existing cloudsiteV1.0.0alpha release tag remains the immutable original
baseline; the deployed hotfix is explicitly identified above, not relabelled as
the old release artifact. Issued CDN bearer URLs survive product revoke until
upstream expiry. No realtime indexing/full-MVP/all-codec claim is made.
