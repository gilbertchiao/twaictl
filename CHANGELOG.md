# Changelog

本專案遵循 [Keep a Changelog](https://keepachangelog.com/zh-TW/1.1.0/) 與 [Semantic Versioning](https://semver.org/lang/zh-TW/)。

## [Unreleased]

> 本 repo 於 2026-09-06 以整理後的內容重新建立（單一初始 commit），`v0.3.0` 以前的開發歷史
> 保存在作者的 private archive，不對外公開；各版本內容仍以本檔為準。

### Fixed
- `vcs volume action attach`：真實 API 回應的 `volume_id`／`server_id` 為字串（spec 標 integer），
  原本在生成碼內解析失敗、明明已掛載卻報錯；改用底層方法自行解析並以 `FlexibleID` 同時接受
  數字與字串。`detach` 成功時 API 回 200 空 body，原本報「解析 API 回應失敗」，改為正常結束
  （`-o json` 輸出 `{"volume_id","action"}` 摘要）。見 `docs/api-notes.md` A38。
- `vcs action --wait`：原本只等 server 狀態到位就回傳，但 site 狀態仍會停留在 `Stopping`／
  `Starting` 等過渡狀態 45～120 秒，期間 API 拒絕下一個動作（`Only Ready, NotReady site can
  be …`）；改為 server 到位後再等 site 進入 `Ready`／`NotReady`。見 A39。
- `vcs image save`：`--os`／`--os-version` 改為必填（真實 API 缺任一個回 400，spec 標選填）。見 A40。

### Changed
- `vcs create --system-volume-type` 說明補充：flavor 的 DISK 為 0 時必填（否則 API 回 500
  `'system_volume_type'`）。見 A41。
- Dependabot（gomod + github-actions，每月、合併 PR）；README 加 badge、安裝章節加 Homebrew tap。

## [0.3.0] - 2026-09-05

### Added
- `cos ls <bucket|cos://bucket[/prefix]> [prefix] --versions`：改呼叫 `ListObjectVersions`
  列出所有物件版本與 delete marker（`ListObjects`/`ListObjectVersions` 分頁正常運作，見
  下方 Fixed），欄位 Key/Version/Latest/Type（`object`/`delete marker`）/Size（delete
  marker 為空白）/Modified，`--columns etag` 可額外顯示 ETag。
- `cos rm cos://bucket/key... --all-versions`：刪除該 key（或搭配 `--recursive` 時整個
  前綴下）的所有物件版本與 delete marker；非遞迴時只刪 key 完全相符的版本（不會誤刪
  `key.bak` 這類前綴相符但 key 不同的物件）；`-o json` 摘要含 `version_id`。
- `cos bucket rm <name>... --purge`：先逐一刪除 bucket 內所有物件版本與 delete marker
  （未開版本控制的 bucket 一樣適用，`VersionId` 為 `null`）再刪除 bucket；解決開啟
  versioning 的 bucket 無法用 twaictl 刪除的問題。確認訊息列出每個 bucket 的版本數；
  `--purge -o json` 與一般 `bucket rm` 一致，不輸出 JSON 摘要。
- `cos usage [--per-bucket] [--key-name NAME]`：顯示 COS 用量，不需 S3 金鑰，走 Ceph
  管理 API `GET /projects/{id}/buckets_util/`（合計）與 `GET /projects/{id}/buckets/`
  （`--per-bucket` 逐 bucket），實測回應為扁平結構、與 spec 不符，已以
  `api/openapi/patches/Ceph-buckets-flat.patch` 修正（見 `docs/api-notes.md` A16）；不
  提供 `--all`（實測會伺服器端逾時，見 A17）。
- `cos sync <local_dir> cos://bucket[/prefix] [--delete]`：只支援本地 → COS 單向同步，
  依序以「遠端不存在」「大小不同」「ETag/MD5 不同（不分大小寫，僅適用單一 PUT 物件）」
  「multipart 物件改比較 mtime」四條規則判斷是否需要上傳；`--delete` 額外刪除遠端前綴下
  本地不存在的物件（破壞性操作，需確認或 `-y`）；沒有任何動作時印出提示、`-o json`
  輸出 `[]`。
- `cos acl cos://bucket/key... (--public|--private) [--recursive]`：設定物件 ACL，
  `--public`（canned ACL `public-read`）需確認或 `-y`，`--private` 不需；`--recursive`
  以前綴展開；`-o json` 輸出 `[{"bucket","key","acl"}]`。
- `cos content-type cos://bucket/key <type>`：以 `GetObjectAcl` → `HeadObject` →
  `CopyObject`（`MetadataDirective=REPLACE`）實作，保留使用者 metadata 與公開狀態；
  物件超過 5 GiB 回報錯誤（不支援 multipart copy）。
- `vcs server ls/get`：新增（設計文件之外），因 `GET /sites/`／`GET /sites/{id}/` 回應巢狀
  的 `servers[]` 不含 server id，`vcs metrics`、`vcs secg ls`、`vcs volume action`
  （attach/detach）、`vcs image save` 需要 server id 時皆改由 `GET /servers/` 以 `site`
  欄位對應（未指定 `--server` 時僅限單一 server 的 site 才能自動解析）。
- `vcs action <id|name> <start|stop|reboot|suspend|resume|shelve|unshelve> [--wait]`：
  `stop`/`reboot`/`suspend`/`shelve` 需確認或 `-y`，`start`/`resume`/`unshelve` 不需；
  `--wait` 依動作輪詢對應目標狀態，`reboot` 需先觀察到離開 `ACTIVE` 才視為完成（否則
  逾時 exit 5，即使重開實際上已完成）；`-o json` 輸出 `{"site_id","action"}`。
- `vcs events <id|name>`：顯示 `GET /sites/{id}/event_logs/` 事件紀錄。
- `vcs metrics <id|name> --meter cpu|memory|disk-read|disk-write|net-in|net-out
  [--begin RFC3339] [--end RFC3339] [--server ID]`：依真實回應形狀解析（disk 系列為
  每裝置一組的巢狀結構，多一欄 `DEVICE`；net-out 欄位鍵名與 net-in 不同，見下方 Fixed
  與 `docs/api-notes.md` A22）；`net-in` 端點伺服器端已知會回 500，照實回報錯誤（exit 3）；
  0 筆資料時 stderr 印出提示。
- `vcs image get/rm/save`：`save`（`PUT /images/{server_id}/save/`）需要 `--name`，可用
  `--server` 覆寫 site 自動解析出的 server id；`rm` 為破壞性操作。
- `vcs network ls/create/get/rm`：`create` 需要 `--name`/`--cidr`，`--gateway`/
  `--dns-domain`/`--router` 選填；spec 描述 `create` 僅 tenant admin 可用；`rm` 為破壞性
  操作；`get` 假設真實回應為 200 + body（spec 標成 204 卻附 schema，見
  `docs/api-notes.md`）。
- `vcs eip ls/create/get/rm/update`：`ls` 支援 `--address` 篩選；`create`（`POST /ips/`）
  spec 描述僅 tenant admin 可用；`update --desc` 必填但可帶空字串清除描述；`<id|address>`
  以位址完全比對解析；`rm` 為破壞性操作。
- `vcs volume ls/create/get/rm/action`：`create` 需要 `--name`/`--size`（大於 0），
  `--type` 為 `hdd`/`ssd`/`LUKS-hdd`/`LUKS-ssd`；`action <id|name> attach|detach|extend`
  中 `attach`/`detach` 需要 `--site` 或 `--server`（二擇一、`--server` 須正整數，於解析前
  即驗證），`detach` 需確認，`extend` 需要 `--size`；`rm` 為破壞性操作。
- `vcs snapshot ls/create/get/rm`：`ls` 支援 `--status` 篩選；`create` 需要 `--name`/
  `--volume`（來源 volume 的 id 或名稱）；`rm` 為破壞性操作。
- `vcs secg ls/add-rule/rm-rule`：`ls <id|name> [--server]` 需要 project 與 server 兩個
  query（未指定 `--server` 時由 site 自動解析）；`add-rule`（`PATCH`）需要 `--direction`/
  `--protocol`，`--port-min`/`--port-max` 皆選填但會檢查 port 範圍（`1`–`65535`）並在只給
  一端時對稱補值；`rm-rule` 為破壞性操作，帶 `project` query。
- `project quota [--user]`：`GET /project_quotas/`（`--user` 改查
  `GET /projects/{id}/user_quotas/`）；把巢狀 `{usage, quota}` 物件攤成
  RESOURCE/USAGE/QUOTA（`--user` 多一欄 USER）列表，`quota` 為負值時顯示 `unlimited`。
- `project solutions`：`GET /projects/{id}/solutions/`，依 solution id 對照 Common
  `/solutions/` 的名稱。
- `twaictl api <METHOD> <path> [--service vcs|cos|common] [--data <json>|@file|@-]
  [--header k=v]...`：逃生口命令，直接呼叫任意 endpoint，走既有 transport；不做輸入
  驗證、不要求確認、不做名稱→id 解析；`--data` 只在 POST/PUT/PATCH 有效，有 body 時
  預設由 transport 注入 `Content-Type: application/json`（可用 `--header` 覆寫，
  大小寫不分）；`x-api-key`/`x-api-host`/`User-Agent` 為保留 header，不可透過
  `--header` 覆寫；空 body 不印出內容，改在 stderr 顯示真實狀態碼；`-o yaml` 要求回應
  為合法 JSON；非 2xx（含 3xx，不跟隨重導向，見下方 Fixed）回應 exit code 3；
  `--dry-run` 印出等價 curl（`x-api-key` 遮蔽為 `***`）。
- `docs/commands.md`：由 cobra 命令樹自動生成的完整命令／flag 說明文件（`make docs`
  重生，`make docs-check` 驗證無漂移並已加入 CI）。
- `vcs create --password-stdin`：從 stdin 讀取一行作為密碼，避免密碼出現在 shell
  歷史與 `ps` 輸出；與 `--password` 互斥（依 flag 是否有指定判斷，而非值是否為空）。
- `vcs secret ls/get/create/rm`：管理 Barbican secret（TLS 憑證），供 `vcs lb create
  --tls-secret` 的 `TERMINATED_HTTPS` listener 使用；`create` 的 `payload` 須為
  PKCS#12（`.p12`／`.pfx`，含憑證與私鑰，可無密碼）bundle 的原始檔案內容
  （`--payload-file`／`--payload-stdin` 二擇一，twaictl 自行 base64 編碼送出），PEM／
  DER 憑證會被 API 拒絕；`id` 為整數（見 `docs/api-notes.md` A28）。
- `vcs lb ls/get/create/update/rm/action/report`：管理 load balancer。`create` 支援
  簡寫 flag（單一 pool + 單一 listener）或 `--spec`（YAML／JSON，多 pool／多
  listener）兩種輸入；`update` 只接受 `--spec`（整份取代既有 pools/listeners，不支援
  簡寫 flag）；`action` 對應 `associate-ip`／`disassociate-ip`／
  `update-listener` 三種動作（皆為 202 非同步，`--wait` 採 mustLeave 語意）；`report`
  依是否帶 `<id|name>` 切換 project 層報表與 member 流量報表兩種查詢（見
  `docs/api-notes.md` A29／A30）。
- `vcs firewall ls/get/create/update/rm`、`vcs firewall rule ls/get/create/update/rm`：
  管理 firewall 與 firewall rule（spec 描述皆僅 tenant admin 可用）。`firewall update`
  的 `--rule`／`--network` 為整份取代既有清單（`PATCH` 語意），`--clear-rules`／
  `--clear-networks` 用於清空；`firewall rule create`／`update` 的 `--protocol`／
  `--action`／`--src`／`--src-port`／`--dst`／`--dst-port` 皆選填。`GET`／
  `PATCH /firewall_rules/{id}/` 的 spec 回應形狀標法不一致，改用新增的
  `twai.DecodeObjectOrFirst` 相容處理（見 `docs/api-notes.md` A33）。
- `vcs asp ls/get/create/rm/attach/detach`：管理 auto scaling policy（spec 描述僅
  tenant admin 可用）。`--meter` 接受與 `vcs metrics` 相同的短名或 API 名稱；`attach`
  的 `--site`／`--server` 二擇一，`--lb` 指定時擴出的 server 會自動加入該 load
  balancer（觸發擴容會建立新 server，可能產生費用）；`detach` 為破壞性操作，需確認或
  `-y`。ASP 的 create request 為 `description`（spec 原標 `desc`，實測會被 API 靜默
  忽略，已以 `VCS-asp-create-description.patch` 修正），CLI 統一使用 `--desc`（見
  `docs/api-notes.md` A34）。

### Changed
- `cos cp`/`cos sync` 上傳依副檔名自動帶上 Content-Type（實際判斷邏輯見下方
  `ContentTypeForPath` 條目），未知副檔名為 `application/octet-stream`；先前上傳一律
  不帶 Content-Type。
- `cos bucket rm` 遇 409 `BucketNotEmpty` 時的錯誤訊息補上提示：可能是版本控制殘留的
  物件版本／delete marker 所致，可用 `cos ls --versions` 查看、`--purge` 清除。
- `cos.Service.GetUsage`/`ListBucketUsage`（`cos usage`）與 `decodeKeys`（`cos key`）對
  200 但缺必要欄位（或欄位為 `null`）的 API 回應改為報錯「API 回應缺少欄位 …」
  （`twai.RequireJSONFields`），先前會靜默顯示零值/空白。
- `cos rm --recursive`（含 `--all-versions`）、`cos bucket rm --purge`：改為兩段式串流
  刪除——第一段只計數（決定確認訊息），確認後第二段邊重新 `List`／`ListObjectVersions`
  邊立即刪除，不再把整批清單蒐集進記憶體；版本刪除以「延後一筆」的 lookbehind 處理，
  確保用來翻頁的 version-id marker 在被拿去查下一頁之前不會被刪除。純 table 輸出（或
  未指定 `-o`）時記憶體成本為常數；`-o json`/`-o yaml` 仍會累積每筆刪除結果的摘要，
  成本與實際刪除筆數成正比。
- `cos cp`/`cos rm`/`cos sync`/`cos acl`/`cos content-type`、`vcs action` 等命令的操作
  摘要：`-o yaml` 現在與 `-o json` 一樣輸出結構化摘要（`renderSummary`），先前只有
  `-o json` 有摘要。
- int32 相關參數（`vcs eip create` 呼叫的 `vcs.CreateIP` projectID、`vcs snapshot create`
  呼叫的 `vcs.CreateSnapshot` volumeID）的驗證統一改為 `<= 0 || > math.MaxInt32` 一次
  檢查，錯誤訊息改成「不是合法的 id（必須介於 1 與 2147483647 之間）」。
- `vcs metrics`：依 `--meter` 指定的欄位選對應的數值鍵（找不到才退回字典序第一個鍵），
  避免不同 meter 混用同一個猜測邏輯。
- `WaitServerStatus`（`--wait`）偵測到有 server 進入 `ERROR` 狀態時的失敗訊息改列出
  全部處於 `ERROR` 狀態的 server，先前只回報第一個（這是輪詢期間的失敗訊息，與
  `--wait-timeout` 逾時是兩回事）。
- `ContentTypeForPath`：改用內建的常見網頁副檔名對照表，取代依賴作業系統 `mime` 套件
  登錄檔的行為，讓不同平台（例如缺少 `/etc/mime.types` 的精簡映像）判斷結果一致；
  對使用者可觀察的行為變更（`cos cp`/`cos sync` 上傳的 Content-Type）：`.xml` 從標準庫
  `mime` 表的 `text/xml; charset=utf-8` 改為 `application/xml`，`.js` 固定為
  `text/javascript; charset=utf-8`（先前依作業系統登錄檔可能是 `application/javascript`
  等其他值）。
- 型別化 client（`internal/twai/client.go`）不再自動跟隨 3xx 重導向：`NewClient`／
  `NewRawClient` 改共用 `newServiceHTTPClient`，`CheckRedirect` 一律回
  `http.ErrUseLastResponse`，3xx 直接落到 `*twai.APIError`（exit code 3），行為與先前
  已修正的 `RawClient`（見下方 Fixed）一致；`make test-integration` 新增
  `TestIntegrationTypedClientDoesNotFollowRedirect` 於真實環境驗證（見
  `docs/api-notes.md` A26）。
- `internal/twai/errors.stripQuery` 組出的錯誤訊息路徑改保留 `RawPath`（`%2F` 等百分號
  編碼），先前用 `url.URL{Path: ...}.String()` 重組會把 `%2F` 正規化成 `/`，可能誤導
  使用者判斷實際請求的路徑。
- 評估 `cos rm`／`cos bucket rm --purge` 改用 S3 `DeleteObjects` 批次刪除：真實環境
  實測顯示 Ceph RGW 的刪除吞吐瓶頸在伺服器端限速，批次化沒有吞吐增益，且逾時／重試的
  失敗模式明顯更差（回應體送不完、client 斷線後伺服器端仍背景續刪導致重試重複刪除），
  **不採用**，維持逐筆 `DeleteObject`／`DeleteObjectVersion`（見 `docs/api-notes.md`
  A31）。
- `--dry-run` 印出的等價 curl 命令，JSON body 欄位遮蔽（`internal/twai/curl.go` 的
  `maskJSONBody`）改為**遞迴**：先前只遮蔽頂層的 `payload`／`password`，現在會走訪
  `map`／`array` 任意深度、命中欄位即遮蔽為 `***`（命中欄位的值不論是字串、object 或
  array，整個值一律換成 `***`，不保留其底下結構；body 不是單一合法 JSON value——例如
  尾端有多餘內容——時原樣印出）。
- COS S3 client（`internal/twai/cos/s3.go`）改用 aws-sdk-go-v2 `aws/transport/http` 的
  `awshttp.NewBuildableClient()`（非新相依，屬 aws-sdk-go-v2 根模組子套件），沿用其
  `limitedRedirect` 政策：只跟隨 307/308，301/302 一律不跟隨（不會把簽章送到別的
  host）；`retryMaxAttempts` 行為不變。
- `vcs.CreateLoadBalancer`（`internal/twai/vcs/loadbalancers.go`）改用 Phase 3b 新增的
  共用 `twai.DecodeObjectOrFirst`（見 `docs/api-notes.md` A33）解析回應，取代先前針對此
  endpoint 單獨寫的物件／陣列判斷邏輯，行為不變。

### Fixed
- `twaictl api`（`RawClient`）不再自動跟隨 3xx 重導向：Go 的 `net/http` 預設會把
  `x-api-key` 原樣複製到重導向目標（只有跨網域時才會剝除 `Authorization`/`Cookie`，
  `x-api-key` 不在剝除清單內），而 apigateway 對不存在的 path 會回 302 導向內部登入頁
  （見 `docs/api-notes.md` A26），是憑證外洩面。現在 3xx 會直接落到 `*twai.APIError`
  （exit code 3），錯誤訊息含狀態碼與 `Location`；型別化 client
  （`internal/twai/client.go`）已於 Phase 3a 比照修正，見上方 Changed。
- `--dry-run` 印出的等價 curl 命令新增 JSON body 頂層欄位遮蔽（`internal/twai/curl.go`
  的 `maskedBodyFields`）：`payload`（`vcs secret create` 的憑證內容）與 `password`
  （目前雖無 API 走 JSON body 傳密碼，先一併涵蓋）不再原樣印出，一律顯示為 `***`；
  重新 marshal 後 JSON key 順序可能與原始 body 不同，但不影響內容判讀。
- `cos.S3.ListObjects`（`cos ls`、`cos rm --recursive`、`cos cp --recursive`、
  `cos sync`、`cos acl --recursive` 等所有走 `ListObjects` 的命令）：Ceph 的
  `ListObjectsV2` 對真實 bucket 回應 `IsTruncated=true` 但 `NextContinuationToken` 為空、
  `KeyCount=0`，導致 `aws-sdk-go-v2` 的 paginator 誤判為已到最後一頁，只取得第一頁
  （1000 筆）就停止——四個真實 bucket（5.7 萬～1,242 萬個物件）都因此只列出前 1000 筆，
  未回報任何錯誤；改用 V1 `ListObjects`（`GET /{bucket}?marker=`）搭配 `Marker` 手動
  分頁後修正（見 `docs/api-notes.md` A18）。
- `project_quotas`／`user_quotas` 的 `cpu`/`gpu`/`memory` 實際為 `{usage, quota}` 浮點
  物件（例如 `-1.0` 代表無上限），且多出 `floating_ip`/`static_ip`（spec 原標的
  `ipvs_port`/`floatingip` 不存在）；spec 誤標為純 `integer`，導致解析失敗。已以
  `api/openapi/patches/VCS-quota-usage-object.patch` 新增共用 `QuotaUsageSerializer`
  修正（見 `docs/api-notes.md` A19）。
- `ips` 的 `occupied_resource.id` 實際為字串（例如 `"328291"`），但 spec 誤標
  `integer/int64`，導致 `vcs eip ls` 解析失敗。已以
  `api/openapi/patches/VCS-ip-occupied-resource-id-string.patch` 改為 `string` 修正
  （見 `docs/api-notes.md` A20）。
- `volumes` 回應多出 spec 未定義的 `volume_type`／`volume_uuid` 欄位，`vcs volume ls/get`
  需要顯示。已以 `api/openapi/patches/VCS-volume-type-fields.patch` 新增為選填欄位修正
  （見 `docs/api-notes.md` A21）。
- `vcs metrics` 四個端點（cpu/memory/disk-read/disk-write/net-out）的真實回應形狀與 spec
  不符：disk 系列實際是每裝置一組的巢狀結構（spec 只標了扁平欄位）；net-out 的數值欄位鍵名
  實際是 `network_outgoing_bytes_rate`，spec 標示與 net-in 共用同一個固定鍵名。改以
  `internal/twai/vcs.decodeMetricPoints` 依「是否有 `utils` 鍵」通用解析兩種形狀（不 patch
  spec，因四個 `Server*Serializer` 需整個重構且 net-in 端點本身於伺服器端回 500 無法驗證，
  見 `docs/api-notes.md` A22）。
- `/auto_scaling_policies/{id}` detail path 缺尾斜線，真實環境依 spec 呼叫回 301 導向
  內部 host（與其餘 detail path 如 `/secrets/{id}/`、`/loadbalancers/{id}/` 皆有尾斜線
  不一致）。已以 `api/openapi/patches/VCS-asp-detail-trailing-slash.patch` 補上尾斜線
  修正，`vcs asp get/rm/attach/detach` 依此路徑呼叫正常回應（見 `docs/api-notes.md`
  A27）。
- `vcs asp create --desc` 先前因 spec 把 `POST /auto_scaling_policies/` requestBody 的
  描述欄位標成 `desc`（回應卻是 `description`）被 API 靜默忽略（成功建立，但描述永遠
  是空字串）；已以 `api/openapi/patches/VCS-asp-create-description.patch` 修正 spec，
  改送 `description`（見 `docs/api-notes.md` A34）。

## [0.1.0] - 2026-08-27

### Added
- `internal/output`：table/json/yaml 三種輸出格式、`--columns` 欄位選擇、`--no-header`、
  API 時間（UTC）轉 `Asia/Taipei` 顯示、YAML 保留任意長度整數精度。
- `config whoami`：驗證 API key，顯示 API 版本、目前 profile/project 與可用 project 清單。
- `project ls`、`project get <id|code>`：查詢 project（改走 VCS `/projects/`，因 CCS 已終止不再提供 `/users/`）。
- `vcs ls`（`--all` 列出所有使用者）、`vcs get <id|name>`、`vcs rm <id|name>... `（`-y` 略過確認、`--wait` 等待刪除完成）。
- `vcs create`：建立 VCS 實例，`--image`/`--flavor`/`--keypair`/`--password`/`--network`/`--az`
  等值以 `x-extra-property-*` header 原樣傳遞；`--solution` 支援 id 或名稱解析；
  `--keypair`/`--password` 至少擇一；`--eip` 為布林（對應 `floating`/`nofloating`）；
  `--volume-size`/`--system-volume-size` 拒絕負數；`--wait` 等待實例 Ready 後重新查詢完整資料。
- `vcs flavor ls`（`--all-projects` 不以 project 過濾）、`vcs image ls`。
- `vcs keypair ls/get/create/rm`：`create` 支援 `--public-key-file` 匯入既有公鑰，或由伺服器
  產生新金鑰對並以 `--out` 寫入檔案（權限 0600；先以獨占方式建立檔案再呼叫 API，避免遺失
  只回傳一次的 private key）；`--out` 不可與 `-o json`/`-o yaml` 併用。
- `--project` 全域 flag 支援直接傳入數字 project id，搭配 `--dry-run` 可完全略過
  project code 的解析請求。
- `--wait`/`--wait-timeout`（預設 10 分鐘，逾時 exit code 5）套用到 `vcs create`、`vcs rm`；
  隱藏的 `--wait-interval` 供測試與進階使用者調整輪詢間隔。
- `--dry-run` 改進：多步驟命令（`vcs create --wait`、`vcs rm a b`）只印出第一個請求即停止，
  不模擬後續步驟。
- `internal/twai`：`APIError`/`ClassifyS3Error` 與其他共用邏輯抽出可跨 vcs/cos 重用的形式，
  為 cos 命令鋪路（不影響既有 vcs 行為）。
- `cos key get/create/renew/rm`：管理 Ceph 的 public/private S3 金鑰。`get`/`renew` 支援
  `--show-secret`（table 模式預設遮蔽 secret；`-o json`/`-o yaml` 一律輸出 API 原始回應含
  secret，並在 stderr 印一次警告）與 `--save`（把 public 金鑰寫入目前 profile 的
  `cos.access_key`/`secret_key`）；`renew` 為破壞性操作（`--name`/`--all` 至少擇一，舊 key
  送出請求後立即失效，需確認或 `-y`），一律先完成輸出（顯示或提示 `--save` 失敗）再嘗試
  儲存，避免遺失只回傳一次的新 secret；`create`/`rm` 需要 `--name`。Ceph 的 project id
  與 VCS 的 project id 是各自獨立編號空間，`cos.Service.ResolveProjectID` 另外向 Ceph
  `GET /projects/?name=` 解析，不沿用已解析出的 VCS project id。
- `cos bucket ls/create/rm`：`create` 支援 `--versioning on|off`；`rm` 為破壞性操作（`-y`
  略過確認），bucket 非空時回傳 409 `BucketNotEmpty`（exit code 3）。
- `cos ls <bucket|cos://bucket[/prefix]> [prefix]`：以 `ListObjectsV2` 分頁列出物件，預設欄位
  Key/Size/Modified，`--columns etag` 可額外顯示 ETag。
- `cos cp <src> <dst> [--recursive]`：僅支援本地 ↔ COS 之間複製（不支援 cos → cos）。上傳
  目的地以 `/` 結尾時沿用本地檔名；下載目的地為既有目錄或以路徑分隔符結尾時，用 key 最後一段
  當檔名。`--recursive` 遞迴上傳略過 symlink；遞迴下載拒絕 key 含 `..`／絕對路徑，並在實際
  寫入前以「解析所有 symlink 之後的實體路徑」二次確認不會逃出目的目錄；下載先寫入
  `<file>.twaictl-part`（`O_EXCL` 建立）暫存檔，成功後才 rename，中斷殘留需手動清除。
  遞迴時的前綴以 `/` 為界正規化（`cos://b/dir` 等同 `cos://b/dir/`），避免誤命中手足前綴。
  上傳超過 16 MiB 自動改用 multipart（`aws-sdk-go-v2` 的 `feature/s3/transfermanager`）；
  下載為單一串流，不分段。
- `cos rm cos://bucket/key... [--recursive]`：破壞性操作，需確認或 `-y`；逐一呼叫
  `DeleteObject`（不用批次刪除 API，避免 Content-MD5／checksum 相容性問題）；無
  `--recursive` 時每個參數都必須是完整 key。
- COS 命令的 `--dry-run`：`cos key *`（Ceph API）與其他命令一樣印出等價 `curl` 命令；
  `cos bucket create/rm`、`cos cp`、`cos rm` 改走 S3 SDK（不經 apigateway），改印
  `dry-run: <動作>` 且不送出任何請求（`--recursive` 時仍會先呼叫唯讀的 `ListObjects` 才能
  列出將被處理的項目）；`cos bucket ls`、`cos ls` 為唯讀查詢，`--dry-run` 下照常執行。

### Fixed
- `cos.ClassifyS3Error`：連線被拒絕／逾時等網路錯誤（請求根本沒送出，
  `*awshttp.ResponseError.HTTPStatusCode()` 為 0）先前會被誤判成 `*twai.APIError`
  （exit code 3，訊息含醜陋的「API 回應 0」），改為先判斷 `twai.IsNetworkError`
  （DESIGN 3.5 要求的 exit code 4），再判斷有效的 HTTP 狀態碼（≥400）才分類成 API 錯誤。
- `cos.NewS3`：明確設定 `RetryMaxAttempts: 3`；先前 `--timeout` 只涵蓋單次嘗試，
  SDK 預設重試次數未固定，導致 `--timeout 1s` 實際可能等到約 5 秒才失敗，
  使用者無法從 `--timeout` 推算總等待時間上限。
- `cos.ClassifyS3Error`：`*twai.APIError.Method` 對 S3 錯誤從單純的 `"S3"` 改為
  `"S3 <Operation>"`（例如 `"S3 CreateBucket"`），從 `*smithy.OperationError` 取得操作名稱，
  讓錯誤訊息能看出是哪一個 S3 操作失敗。
- `cos rm --recursive`：確認訊息先前用命令列參數個數當作「將刪除的物件數」，與實際
  `ListObjects` 找到的數量無關；改為先列出（read-only）符合前綴的物件，確認訊息顯示
  真實數量與前綴（空前綴顯示「整個 bucket」），0 個符合時直接印出提示並成功結束，
  不會印出確認提示。
- `cos cp --recursive` 上傳：`src` 若本身是指向目錄的 symlink，`filepath.WalkDir` 對根路徑
  用 `Lstat` 判斷型別、不會跟隨 symlink，先前會把整個上傳當成 0 個檔案、靜默以 exit 0
  結束；改為先解析根路徑的 symlink 再走訪。目錄樹內部的 symlink 仍照舊略過，
  但現在會印出「略過 symlink」提示；目錄底下沒有可上傳的檔案時也會印出提示，
  不再靜默成功。
- `cos ls -o json`：空 bucket 先前輸出 `null`（Go nil slice 被 `json.Marshal` 的結果），
  改為輸出 `[]`，對消費 JSON 輸出的腳本較友善。
- `cos bucket create --versioning`：`PutBucketVersioning` 失敗時的錯誤訊息補上「bucket
  已建立」的說明，避免使用者誤以為整個 `create` 都沒生效而重複建立；`--dry-run` 補印
  版本控制那一步的訊息。
- `cos ls`：第一個參數不是 `cos://` URL 但含 `/`（例如 `cos ls b/dir`）時，先前會把整個
  字串當成 bucket 名稱去查（因而找不到）；改為以第一個 `/` 切成 bucket 與 prefix。
- `cos cp --recursive` 下載、`cos rm --recursive`：前綴沒有符合任何物件時，先前靜默
  什麼都不做；改為印出「沒有符合的物件」提示。
- `cos cp --recursive` 下載：先前的前綴比對未以 `/` 為界，`cos://b/dir` 會誤命中
  `cos://b/directory/...`；改用 `remotePrefix` 正規化（非空前綴一律補上 `/`）後修正。
  同時補上路徑穿越防護：遠端 key 相對路徑含 `..`／絕對路徑一律拒絕（`safeLocalPath`），
  下載前再以「解析所有 symlink 之後的實體路徑」二次確認不會經由目的目錄樹內既存的
  symlink 逃出目的目錄之外（`ensureUnderDir`）。
- `cos cp` 下載暫存檔（`<file>.twaictl-part`）改以 `O_WRONLY|O_CREATE|O_EXCL` 建立，
  且 symlink 檢查移到目錄建立之前：先前用 `os.Create`（`O_TRUNC`）搭配先建目錄再檢查的
  順序，若該路徑剛好是攻擊者或殘留放置、指向系統外部檔案的 symlink，可能被截斷寫入，
  或在偵測到逃出之前就已在外部建立了空目錄的副作用。
- `cos key --save`：`get`/`renew` 先前若在成功呼叫 API 後才寫入設定檔且寫入失敗，
  使用者會什麼都看不到；改為一律先完成輸出（顯示 secret 或提示 `--show-secret` 重新取得）
  再嘗試儲存，避免 `renew`（舊 key 已立即失效）或 `get` 遺失只回傳一次的 secret。
- `internal/twai`：重構抽出 `twai.Response`/`twai.Decode` 與 `testutil.FakeAPI` 供
  vcs／cos 共用（原本各自實作類似邏輯），修正過程中一併統一 vcs 既有測試的 mock 介面
  大小寫慣例，不影響對外行為。
- `twai.IsNetworkError` 排除 `*fs.PathError`：在 Unix 上，本機檔案系統錯誤（例如
  `vcs keypair create --out` 寫檔失敗）底層的 `syscall.Errno` 恰好也實作 `net.Error` 介面，
  先前會被誤判成網路錯誤（exit code 4）；修正後改落入一般錯誤（exit code 1）。
- `twai.BuildCurl`（`--dry-run` 輸出）把所有自訂 `x-*` header 統一以小寫顯示，避免 Go
  `http.Header` 的 canonical 大小寫（例如 `X-Extra-Property-Password`）與 OpenAPI 文件慣例
  不一致。
- `vcs flavor ls`：真實 API 的 `resource.gpu` 實際回傳浮點值（例如 `0.25`），但 spec 誤標為
  `integer`，導致解析失敗；以 `api/openapi/patches/VCS-flavor-gpu-number.patch` 修正 spec
  為 `number`，並改用新的 `output.Float` 顯示。
- `vcs keypair get`：真實 API 的 `create_time` 回傳無時區的本地時間字串，但 spec 誤標為
  `date-time`（RFC3339），導致解析失敗；以
  `api/openapi/patches/VCS-keypair-create-time-string.patch` 修正 spec 為純字串，並新增
  `output.FormatTimeString` 相容解析多種時間格式。

## [0.0.1] - 2026-08-27

### Added
- 專案骨架：Go module、Makefile、golangci-lint v2、GitHub Actions CI（lint / gen-check / 三平台測試）、goreleaser。
- Vendor twcc/man-vip `tws-sync` 的 OpenAPI（VCS 1.5.4 / Ceph 1.1.0 / Common 1.2.1）並以 oapi-codegen 生成 client。
- `internal/config`：設定檔讀寫（0600）、profile、flag → 環境變數 → 檔案 → 預設值優先序、舊版 `_TWCC_*` 環境變數相容提示。
- `internal/twai`：自訂 HTTP transport（header 注入、GET 重試、`--dry-run` curl 輸出、slog debug log、secret 遮蔽）、`APIError`、各服務 client 建構。
- 命令：`version`、`config init`、`config show`、`completion`。

[Unreleased]: https://github.com/gilbertchiao/twaictl/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/gilbertchiao/twaictl/compare/v0.1.0...v0.3.0
[0.1.0]: https://github.com/gilbertchiao/twaictl/compare/v0.0.1...v0.1.0
[0.0.1]: https://github.com/gilbertchiao/twaictl/releases/tag/v0.0.1
