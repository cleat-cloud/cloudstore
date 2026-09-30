# Backblaze B2 API map — what CloudStore uses, and how

Reference: [B2 Native API v4](https://www.backblaze.com/apidocs/introduction-to-the-b2-native-api).
The console talks **only** to the native API (`internal/storage/b2`, plain `net/http`); the
S3-compatible endpoint is not used (bucket governance fields such as `corsRules`,
`lifecycleRules` and `defaultServerSideEncryption` live in the native API).

Auth: one application key per account (`b2_authorize_account`, Basic auth) → account token +
`apiUrl`/`downloadUrl`, cached in memory and refreshed once on a 401. Keys are sealed with
AES-256-GCM under `APP_SECRET` before they touch SQLite.

**Transaction classes** follow Backblaze's pricing table (A: list buckets/keys, bucket CRUD;
B: list files/versions, file info; C: upload/download/copy/delete). The HTTP classes below are what
the console's own calls cost, not account-wide usage — B2 does not expose account usage via the API,
so the console never displays numbers it cannot source (see "Honest limits").

## Operations

| B2 operation                                                                                 | Console feature                                                         | Status         |
| -------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- | -------------- |
| `b2_authorize_account`                                                                       | account connection (Settings → Storage Accounts), token cache           | ✅             |
| `b2_list_buckets`                                                                            | buckets overview, bucket metadata/settings, `Region` label              | ✅             |
| `b2_create_bucket`                                                                           | "Criar Bucket" (private/public, name validated client-side)             | ✅             |
| `b2_delete_bucket`                                                                           | row action "Excluir" (B2 refuses non-empty buckets → flash message)     | ✅             |
| `b2_update_bucket`                                                                           | lifecycle rules, CORS rules, default SSE, Object Lock default retention | ✅             |
| `b2_list_file_names`                                                                         | object browser (folders via `delimiter=/`, `startFileName` pagination)  | ✅             |
| `b2_list_file_versions`                                                                      | object versions (`?versions=1`): hide markers, per-version delete       | ✅             |
| `b2_get_file_info`                                                                           | object detail: SHA-1, MD5, SSE mode, retention, legal hold              | ✅             |
| `b2_get_upload_url` + `b2_upload_file`                                                       | uploads (single request, browser-side multipart)                        | ✅             |
| `b2_delete_file_version`                                                                     | delete (resolves the fileId first) and per-version delete               | ✅             |
| `b2_hide_file`                                                                               | row action "Ocultar" (soft delete; keeps the version history)           | ✅             |
| `b2_copy_file`                                                                               | row action "Copiar", version "Restaurar" (copy a version to the name)   | ✅             |
| `b2_get_download_authorization`                                                              | presigned URL (15 min / 1 h / 24 h) in the detail panel                 | ✅             |
| `b2_download_file_by_name` / `_by_id`                                                        | direct download through the console (streaming proxy, no key exposure)  | ✅             |
| `b2_update_file_retention`                                                                   | detail panel: set/extend governance or compliance retention             | ✅             |
| `b2_update_file_legal_hold`                                                                  | detail panel: legal hold on/off (needs `writeFileLegalHolds`)           | ✅             |
| `b2_list_keys`                                                                               | Settings/policies: application keys with capabilities + restrictions    | ⏳             |
| `b2_get/set_bucket_notification_rules`                                                       | audit pipeline card (event notification targets)                        | ⏳             |
| `b2_start_large_file` + `b2_get_upload_part_url` + `b2_upload_part` + `b2_finish_large_file` | uploads above the simple-upload limit (5 GB) /> `MAX_BODY_BYTES`        | ⏳             |
| `b2_list_unfinished_large_files` + `b2_cancel_large_file`                                    | upload queue / cancelled multipart parts                                | ⏳             |
| `b2_list_parts`                                                                              | part inspector for an unfinished large file                             | ⏳             |
| `b2_create_key` / `b2_delete_key`                                                            | scoped keys created from the console                                    | ❌ not planned |
| `b2_cancel_large_file`                                                                       | (see unfinished files)                                                  | ⏳             |

Legend: ✅ implemented · ⏳ next pass · ❌ deliberately out of scope.

## Capabilities the console relies on

The application key must have: `listBuckets`, `readBuckets`, `writeBuckets`, `deleteBuckets`,
`listFiles`, `readFiles`, `writeFiles`, `deleteFiles`, `shareFiles`, and — for Object Lock screens —
`readBucketRetentions`, `writeBucketRetentions`, `readFileRetentions`, `writeFileRetentions`,
`readFileLegalHolds`, `writeFileLegalHolds`. Fields the key cannot read come back as
`isClientAuthorizedToRead: false`; the UI renders "—" instead of guessing.

`bucketType` is fixed at creation: the console shows it as read-only and explains why.

## Honest limits (no invented data)

- **No account-wide metrics.** B2 exposes no usage/requests/cost API. The analytics screen shows
  what the console itself can source: daily storage/object samples from its own scans, console
  operations from the audit trail, and the $6/TB/month storage estimate derived from scanned bytes.
- **Scans are capped** (`scanObjectCap = 20000` objects per bucket, folders walked recursively) so a
  multi-million-object bucket cannot hammer the API; the UI marks a capped scan instead of pretending
  it is complete.
- **Per-account baseline.** Usage samples are keyed to the active account: connecting, activating or
  removing an account clears the local samples, so numbers from a previous account never leak into
  the next one.
- **Demo mode is opt-in** (`CLOUDSTORE_DEMO=1`). Without it and without an account, the console
  shows the onboarding state and asks for an application key instead of fabricated buckets.
- **Versioning warnings come from B2**: `b2_list_file_versions` returns hide markers and every
  version; deleting a bucket with versions still fails server-side and the flash message says so.

## Rate limits and costs

- Listing is billed per 1 000 files returned (`b2_list_file_names`/`_versions`); the console asks for
  at most 1 000 per call and caps scans.
- `b2_authorize_account` tokens last up to 24 h; the client refreshes once per expiry, never per
  request.
- Uploads stream from the browser to B2 (single request today); the body cap is `MAX_BODY_BYTES`
  (default 32 MiB) — raise it for larger files until the large-file pass lands.
