# twaictl 命令列表

本文件由 `tools/gendocs` 從 cobra 命令樹自動產生（`make docs`），請勿手動修改。

## 目錄

- [twaictl](#twaictl)
- [twaictl api](#twaictl-api)
- [twaictl config](#twaictl-config)
- [twaictl config init](#twaictl-config-init)
- [twaictl config show](#twaictl-config-show)
- [twaictl config whoami](#twaictl-config-whoami)
- [twaictl cos](#twaictl-cos)
- [twaictl cos acl](#twaictl-cos-acl)
- [twaictl cos bucket](#twaictl-cos-bucket)
- [twaictl cos bucket create](#twaictl-cos-bucket-create)
- [twaictl cos bucket ls](#twaictl-cos-bucket-ls)
- [twaictl cos bucket rm](#twaictl-cos-bucket-rm)
- [twaictl cos content-type](#twaictl-cos-content-type)
- [twaictl cos cp](#twaictl-cos-cp)
- [twaictl cos key](#twaictl-cos-key)
- [twaictl cos key create](#twaictl-cos-key-create)
- [twaictl cos key get](#twaictl-cos-key-get)
- [twaictl cos key renew](#twaictl-cos-key-renew)
- [twaictl cos key rm](#twaictl-cos-key-rm)
- [twaictl cos ls](#twaictl-cos-ls)
- [twaictl cos rm](#twaictl-cos-rm)
- [twaictl cos sync](#twaictl-cos-sync)
- [twaictl cos usage](#twaictl-cos-usage)
- [twaictl project](#twaictl-project)
- [twaictl project get](#twaictl-project-get)
- [twaictl project ls](#twaictl-project-ls)
- [twaictl project quota](#twaictl-project-quota)
- [twaictl project solutions](#twaictl-project-solutions)
- [twaictl vcs](#twaictl-vcs)
- [twaictl vcs action](#twaictl-vcs-action)
- [twaictl vcs asp](#twaictl-vcs-asp)
- [twaictl vcs asp attach](#twaictl-vcs-asp-attach)
- [twaictl vcs asp create](#twaictl-vcs-asp-create)
- [twaictl vcs asp detach](#twaictl-vcs-asp-detach)
- [twaictl vcs asp get](#twaictl-vcs-asp-get)
- [twaictl vcs asp ls](#twaictl-vcs-asp-ls)
- [twaictl vcs asp rm](#twaictl-vcs-asp-rm)
- [twaictl vcs create](#twaictl-vcs-create)
- [twaictl vcs eip](#twaictl-vcs-eip)
- [twaictl vcs eip create](#twaictl-vcs-eip-create)
- [twaictl vcs eip get](#twaictl-vcs-eip-get)
- [twaictl vcs eip ls](#twaictl-vcs-eip-ls)
- [twaictl vcs eip rm](#twaictl-vcs-eip-rm)
- [twaictl vcs eip update](#twaictl-vcs-eip-update)
- [twaictl vcs events](#twaictl-vcs-events)
- [twaictl vcs firewall](#twaictl-vcs-firewall)
- [twaictl vcs firewall create](#twaictl-vcs-firewall-create)
- [twaictl vcs firewall get](#twaictl-vcs-firewall-get)
- [twaictl vcs firewall ls](#twaictl-vcs-firewall-ls)
- [twaictl vcs firewall rm](#twaictl-vcs-firewall-rm)
- [twaictl vcs firewall rule](#twaictl-vcs-firewall-rule)
- [twaictl vcs firewall rule create](#twaictl-vcs-firewall-rule-create)
- [twaictl vcs firewall rule get](#twaictl-vcs-firewall-rule-get)
- [twaictl vcs firewall rule ls](#twaictl-vcs-firewall-rule-ls)
- [twaictl vcs firewall rule rm](#twaictl-vcs-firewall-rule-rm)
- [twaictl vcs firewall rule update](#twaictl-vcs-firewall-rule-update)
- [twaictl vcs firewall update](#twaictl-vcs-firewall-update)
- [twaictl vcs flavor](#twaictl-vcs-flavor)
- [twaictl vcs flavor ls](#twaictl-vcs-flavor-ls)
- [twaictl vcs get](#twaictl-vcs-get)
- [twaictl vcs image](#twaictl-vcs-image)
- [twaictl vcs image get](#twaictl-vcs-image-get)
- [twaictl vcs image ls](#twaictl-vcs-image-ls)
- [twaictl vcs image rm](#twaictl-vcs-image-rm)
- [twaictl vcs image save](#twaictl-vcs-image-save)
- [twaictl vcs keypair](#twaictl-vcs-keypair)
- [twaictl vcs keypair create](#twaictl-vcs-keypair-create)
- [twaictl vcs keypair get](#twaictl-vcs-keypair-get)
- [twaictl vcs keypair ls](#twaictl-vcs-keypair-ls)
- [twaictl vcs keypair rm](#twaictl-vcs-keypair-rm)
- [twaictl vcs lb](#twaictl-vcs-lb)
- [twaictl vcs lb action](#twaictl-vcs-lb-action)
- [twaictl vcs lb create](#twaictl-vcs-lb-create)
- [twaictl vcs lb get](#twaictl-vcs-lb-get)
- [twaictl vcs lb ls](#twaictl-vcs-lb-ls)
- [twaictl vcs lb report](#twaictl-vcs-lb-report)
- [twaictl vcs lb rm](#twaictl-vcs-lb-rm)
- [twaictl vcs lb update](#twaictl-vcs-lb-update)
- [twaictl vcs ls](#twaictl-vcs-ls)
- [twaictl vcs metrics](#twaictl-vcs-metrics)
- [twaictl vcs network](#twaictl-vcs-network)
- [twaictl vcs network create](#twaictl-vcs-network-create)
- [twaictl vcs network get](#twaictl-vcs-network-get)
- [twaictl vcs network ls](#twaictl-vcs-network-ls)
- [twaictl vcs network rm](#twaictl-vcs-network-rm)
- [twaictl vcs rm](#twaictl-vcs-rm)
- [twaictl vcs secg](#twaictl-vcs-secg)
- [twaictl vcs secg add-rule](#twaictl-vcs-secg-add-rule)
- [twaictl vcs secg ls](#twaictl-vcs-secg-ls)
- [twaictl vcs secg rm-rule](#twaictl-vcs-secg-rm-rule)
- [twaictl vcs secret](#twaictl-vcs-secret)
- [twaictl vcs secret create](#twaictl-vcs-secret-create)
- [twaictl vcs secret get](#twaictl-vcs-secret-get)
- [twaictl vcs secret ls](#twaictl-vcs-secret-ls)
- [twaictl vcs secret rm](#twaictl-vcs-secret-rm)
- [twaictl vcs server](#twaictl-vcs-server)
- [twaictl vcs server get](#twaictl-vcs-server-get)
- [twaictl vcs server ls](#twaictl-vcs-server-ls)
- [twaictl vcs snapshot](#twaictl-vcs-snapshot)
- [twaictl vcs snapshot create](#twaictl-vcs-snapshot-create)
- [twaictl vcs snapshot get](#twaictl-vcs-snapshot-get)
- [twaictl vcs snapshot ls](#twaictl-vcs-snapshot-ls)
- [twaictl vcs snapshot rm](#twaictl-vcs-snapshot-rm)
- [twaictl vcs volume](#twaictl-vcs-volume)
- [twaictl vcs volume action](#twaictl-vcs-volume-action)
- [twaictl vcs volume create](#twaictl-vcs-volume-create)
- [twaictl vcs volume get](#twaictl-vcs-volume-get)
- [twaictl vcs volume ls](#twaictl-vcs-volume-ls)
- [twaictl vcs volume rm](#twaictl-vcs-volume-rm)
- [twaictl version](#twaictl-version)

## twaictl

台智雲 TWCC 平台的非官方命令列工具

twaictl 是台智雲（Taiwan AI Cloud, TWAI）TWCC 平台的非官方（community）命令列工具，
以 Go 重新實作、取代已停止維護的 twcc/TWCC-CLI。

**Usage**

```
twaictl
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--columns` |  |  | table 模式只顯示指定欄位（逗號分隔） |
| `--dry-run` |  | `false` | 不送出請求，改印出等價的 curl 命令 |
| `--no-header` |  | `false` | table 模式不印表頭 |
| `--output` | `-o` | `table` | 輸出格式：table\|json\|yaml |
| `--profile` |  |  | 使用設定檔中的哪個 profile（預設 default，或 TWAI_PROFILE） |
| `--project` |  |  | 覆寫 profile 中的 project code |
| `--timeout` |  | `30s` | 單次 HTTP 請求逾時 |
| `--verbose` | `-v` | `false` | 輸出 debug 等級 log 到 stderr |
| `--wait` |  | `false` | 對非同步操作輪詢直到完成 |
| `--wait-timeout` |  | `10m0s` | --wait 的逾時 |
| `--yes` | `-y` | `false` | 略過破壞性操作的確認提示 |

## twaictl api

直接呼叫任意 API endpoint（逃生口，不驗證、不確認、不做名稱解析）

直接呼叫任意 API endpoint，做為其他子命令都無法涵蓋（或尚未支援）時的逃生口。

本命令不做任何輸入驗證、不會要求確認、也不做名稱→id 解析：&lt;path&gt; 必須是實際的 API 路徑
（query string 直接寫在 path 裡，例如 /sites/?project=60000）；寫入類請求
（POST/PUT/PATCH/DELETE）的 body 會原封不動送出，請自行確認內容正確再執行——
沒有二次確認機制，用錯 path/body 造成的後果需自行負責。

--dry-run 遮蔽 JSON body 內任何深度的 payload／password 欄位；非 JSON body
會原樣印出，請自行確認機密內容不會外洩。

範例：
  twaictl api GET /sites/
  twaictl api POST /sites/ --data '{"name":"demo"}'
  twaictl api POST /sites/ --data @body.json
  echo '{"name":"demo"}' | twaictl api POST /sites/ --data @-
  twaictl api GET /usage/ --service cos
  twaictl api GET /solutions/ --service common

**Usage**

```
twaictl api <METHOD> <path> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--data` |  |  | request body：字面 JSON 字串、@file 讀檔、或 @- 讀 stdin（僅 POST/PUT/PATCH 可用） |
| `--header` |  | `[]` | 附加自訂 header（k=v，可重複；重複指定同名 header 以最後一次為準，不會附加；不可指定 x-api-key/x-api-host/User-Agent） |
| `--service` |  | `vcs` | 呼叫哪個服務：vcs、cos、common |

## twaictl config

管理設定檔與 profile

**Usage**

```
twaictl config
```

**Flags**：無

## twaictl config init

建立或更新 profile（API key、project code、選填的 apigateway / ApiHost）

互動式建立 profile。以 --profile 指定名稱；未指定時為目前生效的 profile
（TWAI_PROFILE 或設定檔 current_profile，皆未設定時為 default）。
API key 可在 TWCC 網站「使用者資訊 → API 金鑰」取得；學研用戶（iService）與企業用戶的 key 用法相同。
API key、project code 未以 flag 指定時，依序改用環境變數（TWAI_API_KEY / TWAI_PROJECT_CODE，
或舊版 _TWCC_API_KEY_ / _TWCC_PROJECT_CODE_）、再互動詢問。
提示：互動輸入 API key 時不會回顯（非終端機環境例如 pipe 則以一般方式讀取一行）；
若在意，可改用 --api-key flag 或 TWAI_API_KEY 環境變數。

**Usage**

```
twaictl config init [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--api-host-common` |  |  | 覆寫 Common API 的 x-api-host（預設 goc） |
| `--api-host-cos` |  |  | 覆寫 COS 的 x-api-host（預設 ceph-taichung-default） |
| `--api-host-vcs` |  |  | 覆寫 VCS 的 x-api-host（預設 openstack-taichung-default-2） |
| `--api-key` |  |  | API key（未指定則互動詢問） |
| `--apigateway` |  |  | 覆寫 apigateway URL（預設 https://apigateway.twcc.ai） |
| `--cos-access-key` |  |  | COS S3 access key（選填） |
| `--cos-endpoint` |  |  | 覆寫 COS S3 endpoint（預設 https://cos.twcc.ai） |
| `--cos-secret-key` |  |  | COS S3 secret key（選填） |
| `--project-code` |  |  | 預設 project code（未指定則互動詢問） |

## twaictl config show

顯示目前生效的設定（API key 與 secret 遮蔽）

**Usage**

```
twaictl config show
```

**Flags**：無

## twaictl config whoami

驗證 API key 並顯示可用的 project

驗證 API key，並顯示 API 版本、目前 profile/project、可用的 project 清單。

下方 project 清單的可用欄位（--columns，不分大小寫）：ID, NAME, PLATFORM, TRANSPARENT
預設欄位：ID, NAME, PLATFORM, TRANSPARENT

**Usage**

```
twaictl config whoami
```

**Flags**：無

## twaictl cos

管理 COS（S3 相容）：金鑰、bucket、物件、用量

**Usage**

```
twaictl cos
```

**Flags**：無

## twaictl cos acl

設定物件 ACL：--public 為 public-read（匿名可讀，需確認或 -y）、--private 為 private

**Usage**

```
twaictl cos acl cos://bucket/key... (--public | --private) [--recursive] [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--private` |  | `false` | 設為 private |
| `--public` |  | `false` | 設為 public-read（任何人可讀） |
| `--recursive` |  | `false` | 對前綴下的所有物件套用 |

## twaictl cos bucket

管理 COS bucket

**Usage**

```
twaictl cos bucket
```

**Flags**：無

## twaictl cos bucket create

建立 bucket

**Usage**

```
twaictl cos bucket create <name> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--versioning` |  |  | 建立後立即設定版本控制：on\|off |

## twaictl cos bucket ls

列出所有 bucket

列出所有 bucket。

可用欄位（--columns，不分大小寫）：Name, Created
預設欄位：Name, Created

**Usage**

```
twaictl cos bucket ls
```

**Flags**：無

## twaictl cos bucket rm

刪除 bucket（破壞性操作，需確認或 -y；bucket 非空時會失敗）

**Usage**

```
twaictl cos bucket rm <name>... [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--purge` |  | `false` | 先刪除 bucket 內所有物件版本與 delete marker 再刪除 bucket |

## twaictl cos content-type

修改物件的 Content-Type（例如 text/html）

**Usage**

```
twaictl cos content-type cos://bucket/key <type>
```

**Flags**：無

## twaictl cos cp

在本地檔案與 COS 物件之間複製（僅支援本地 ↔ COS）

**Usage**

```
twaictl cos cp <src> <dst> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--recursive` |  | `false` | 遞迴複製整個目錄／前綴 |

## twaictl cos key

管理 COS S3 金鑰（public / private）

**Usage**

```
twaictl cos key
```

**Flags**：無

## twaictl cos key create

建立新的 private S3 金鑰

**Usage**

```
twaictl cos key create [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--name` |  |  | 新金鑰的名稱（必填） |
| `--show-secret` |  | `false` | 顯示新建立金鑰的 secret |

## twaictl cos key get

顯示目前 project 的 COS S3 金鑰

顯示目前 project 的 COS S3 金鑰。

可用欄位（--columns，不分大小寫）：Public access key, Public secret key, Private keys
預設欄位：Public access key, Public secret key, Private keys

**Usage**

```
twaictl cos key get [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--save` |  | `false` | 把 public 金鑰寫入目前 profile 的 cos.access_key/secret_key |
| `--show-secret` |  | `false` | 顯示 secret key（table 模式預設遮蔽；-o json/yaml 一律含 secret） |

## twaictl cos key renew

輪替 S3 金鑰（破壞性操作：舊 key 立即失效，需確認或 -y）

**Usage**

```
twaictl cos key renew [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--all` |  | `false` | 輪替所有 private 金鑰 |
| `--name` |  |  | 只輪替指定名稱的 private 金鑰 |
| `--save` |  | `false` | 把輪替後的 public 金鑰寫入目前 profile 的 cos.access_key/secret_key |
| `--show-secret` |  | `false` | 顯示輪替後的新 secret（table 模式預設遮蔽；-o json/yaml 一律含 secret） |

## twaictl cos key rm

刪除指定名稱的 private S3 金鑰（破壞性操作，需確認或 -y）

**Usage**

```
twaictl cos key rm [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--name` |  |  | 要刪除的金鑰名稱（必填） |

## twaictl cos ls

列出 bucket 內的物件

列出 bucket 內的物件。

可用欄位（--columns，不分大小寫）：Key, Size, Modified, ETag
預設欄位：Key, Size, Modified

--versions 時：
可用欄位（--columns，不分大小寫）：Key, Version, Latest, Type, Size, Modified, ETag
預設欄位：Key, Version, Latest, Type, Size, Modified

**Usage**

```
twaictl cos ls <bucket|cos://bucket[/prefix]> [prefix] [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--versions` |  | `false` | 列出所有物件版本與 delete marker（versioning bucket 用） |

## twaictl cos rm

刪除 COS 物件（破壞性操作，需確認或 -y）

**Usage**

```
twaictl cos rm cos://bucket/key... [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--all-versions` |  | `false` | 刪除該 key（或 --recursive 前綴下）的所有物件版本與 delete marker（versioning bucket 用） |
| `--recursive` |  | `false` | 遞迴刪除前綴下的所有物件 |

## twaictl cos sync

把本地目錄同步到 COS 前綴（只上傳有變更的檔案；--delete 一併刪除遠端多出的物件）

**Usage**

```
twaictl cos sync <local_dir> cos://bucket[/prefix] [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--delete` |  | `false` | 刪除遠端前綴底下本地不存在的物件（破壞性操作，需確認或 -y） |

## twaictl cos usage

顯示 COS 用量（總用量，或 --per-bucket 逐 bucket）

顯示 COS 用量。預設顯示 public 金鑰底下所有 bucket 的合計，--key-name 可改查指定的 private 金鑰。

可用欄位（--columns，不分大小寫）：Key name, Used space, Objects
預設欄位：Key name, Used space, Objects

--per-bucket 時：
可用欄位（--columns，不分大小寫）：Name, Used space, Objects, Modified
預設欄位：Name, Used space, Objects, Modified

**Usage**

```
twaictl cos usage [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--key-name` |  |  | 查詢指定 private 金鑰底下的 bucket（預設為 public 金鑰） |
| `--per-bucket` |  | `false` | 逐 bucket 列出用量與物件數 |

## twaictl project

查詢 project

**Usage**

```
twaictl project
```

**Flags**：無

## twaictl project get

顯示單一 project 的詳細資料

顯示單一 project 的詳細資料。

可用欄位（--columns，不分大小寫）：ID, Name, Status, Description, UUID, Transparent
預設欄位：ID, Name, Status, Description, UUID, Transparent

**Usage**

```
twaictl project get <id|code>
```

**Flags**：無

## twaictl project ls

列出可用的 project

列出可用的 project。

可用欄位（--columns，不分大小寫）：ID, NAME, PLATFORM, TRANSPARENT
預設欄位：ID, NAME, PLATFORM, TRANSPARENT

**Usage**

```
twaictl project ls
```

**Flags**：無

## twaictl project quota

查詢 project 配額

查詢 project 配額。

可用欄位（--columns，不分大小寫）：RESOURCE, USAGE, QUOTA
預設欄位：RESOURCE, USAGE, QUOTA

--user 時改列出每位使用者的配額，欄位為 USER、RESOURCE、USAGE、QUOTA。

**Usage**

```
twaictl project quota [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--user` |  | `false` | 列出每位使用者的配額（而非整個 project） |

## twaictl project solutions

列出 project 已啟用的 solution

列出 project 已啟用的 solution。

可用欄位（--columns，不分大小寫）：ID, NAME
預設欄位：ID, NAME

**Usage**

```
twaictl project solutions
```

**Flags**：無

## twaictl vcs

管理 VCS 虛擬機實例

**Usage**

```
twaictl vcs
```

**Flags**：無

## twaictl vcs action

對 VCS 實例送出動作（啟動／停止／重開機等）

對 VCS 實例送出動作（啟動／停止／重開機等）。

--wait 對 reboot 的限制：送出 reboot 後 server 通常仍是 ACTIVE（重開尚未開始），因此 --wait 會先等到觀察到 server 離開 ACTIVE、再等它回到 ACTIVE 才算完成；若整個重開過程比 --wait-interval（預設 5s）還快、沒有輪詢到「已離開 ACTIVE」的瞬間，會持續等到 --wait-timeout 逾時（exit 5），此時實際上 reboot 多半已經完成，只是 --wait 沒能觀察到。

**Usage**

```
twaictl vcs action <id|name> <start|stop|reboot|suspend|resume|shelve|unshelve>
```

**Flags**：無

## twaictl vcs asp

管理 auto scaling policy（自動擴縮政策）

**Usage**

```
twaictl vcs asp
```

**Flags**：無

## twaictl vcs asp attach

把 auto scaling policy 掛到 server（觸發時會自動建立新 server）

把 auto scaling policy 掛到 server。--site 為 VCS 實例（自動解析成其唯一的 server；多台 server 的實例請改用 --server）。
指定 --lb 時，擴出的 server 會自動加入該 load balancer（原 server 除外），--port 為 server 監聽的 port（預設同 load balancer）；--port 與 --lb 是彼此獨立的選填欄位，未指定 --lb 時 --port 的用途由 API 決定。
--scale-up-action／--scale-down-action 為觸發擴／縮時以 HTTP POST 通知的 URL。

注意：政策觸發擴容時會建立新的 server，可能產生費用。

可用欄位（--columns，不分大小寫）：Auto scaling policy, Load balancer
預設欄位：Auto scaling policy, Load balancer

**Usage**

```
twaictl vcs asp attach <id|name> (--site <id|name> | --server <id>) [--lb <id|name>] [--port <n>] [--scale-up-action <url>] [--scale-down-action <url>] [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--lb` |  |  | 擴出的 server 要加入的 load balancer id 或名稱 |
| `--port` |  | `0` | server 監聽的 port（預設同 load balancer；未指定 --lb 時 port 的用途由 API 決定） |
| `--scale-down-action` |  |  | 縮容時通知的 URL（HTTP POST） |
| `--scale-up-action` |  |  | 擴容時通知的 URL（HTTP POST） |
| `--server` |  |  | server id（與 --site 擇一） |
| `--site` |  |  | VCS 實例 site 的 id 或名稱（與 --server 擇一） |

## twaictl vcs asp create

建立 auto scaling policy

建立 auto scaling policy。--meter 接受與 `vcs metrics` 相同的短名（cpu|memory|disk-read|disk-write|net-in|net-out）或 API 名稱（cpu_util、memory.usage、…）；--scale-up／--scale-down 為觸發擴／縮的門檻值，--max-size 為擴到最多幾台。

可用欄位（--columns，不分大小寫）：ID, Name, Meter, Scale up threshold, Scale down threshold, Max size, Desc, Project, User, Platform
預設欄位：ID, Name, Meter, Scale up threshold, Scale down threshold, Max size, Desc, Project, User

**Usage**

```
twaictl vcs asp create --name <n> --meter <m> --scale-up <pct> --max-size <n> [--scale-down <pct>] [--desc <d>] [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--desc` |  |  | 描述 |
| `--max-size` |  | `0` | 最多擴到幾台 server（必填，正整數） |
| `--meter` |  |  | 監控指標（必填）：cpu\|memory\|disk-read\|disk-write\|net-in\|net-out，或 API 名稱 |
| `--name` |  |  | 政策名稱（必填） |
| `--scale-down` |  | `0` | 縮容門檻（選填，須小於 --scale-up） |
| `--scale-up` |  | `0` | 擴容門檻（必填；例如 cpu 使用率 80） |

## twaictl vcs asp detach

移除 server 上的 auto scaling policy（需確認或 -y）

**Usage**

```
twaictl vcs asp detach (--site <id|name> | --server <id>) [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--server` |  |  | server id（與 --site 擇一） |
| `--site` |  |  | VCS 實例 site 的 id 或名稱（與 --server 擇一） |

## twaictl vcs asp get

顯示單一 auto scaling policy 的詳細資料

顯示單一 auto scaling policy 的詳細資料。

可用欄位（--columns，不分大小寫）：ID, Name, Meter, Scale up threshold, Scale down threshold, Max size, Desc, Project, User, Platform
預設欄位：ID, Name, Meter, Scale up threshold, Scale down threshold, Max size, Desc, Project, User

**Usage**

```
twaictl vcs asp get <id|name>
```

**Flags**：無

## twaictl vcs asp ls

列出 auto scaling policy

列出 auto scaling policy。

可用欄位（--columns，不分大小寫）：ID, NAME, METER, SCALE UP, SCALE DOWN, MAX SIZE, DESC, USER, PLATFORM
預設欄位：ID, NAME, METER, SCALE UP, SCALE DOWN, MAX SIZE

**Usage**

```
twaictl vcs asp ls
```

**Flags**：無

## twaictl vcs asp rm

刪除 auto scaling policy（破壞性操作，需確認或 -y）

**Usage**

```
twaictl vcs asp rm <id|name>...
```

**Flags**：無

## twaictl vcs create

建立 VCS 實例

建立 VCS 實例。

可用欄位（--columns，不分大小寫）：ID, Name, Status, Public IP, Solution, Project, User, Created, Servers, Progress, Termination protection
預設欄位：ID, Name, Status, Public IP, Solution, Project, User, Created, Servers, Progress, Termination protection

**Usage**

```
twaictl vcs create [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--az` |  |  | 可用區域（availability zone） |
| `--desc` |  |  | 描述 |
| `--eip` |  | `false` | 配發浮動 IP（floating IP） |
| `--flavor` |  |  | flavor（必填，原樣傳遞給 API） |
| `--image` |  |  | image（必填，原樣傳遞給 API） |
| `--keypair` |  |  | SSH 金鑰對名稱（與 --password 至少擇一） |
| `--name` |  |  | VCS 實例名稱（必填） |
| `--network` |  |  | private network 名稱 |
| `--password` |  |  | 登入密碼（與 --keypair 至少擇一；僅用於 API header，不會出現在任何輸出） |
| `--password-stdin` |  | `false` | 從 stdin 讀取登入密碼（讀到第一個換行為止），避免密碼出現在 shell 歷史與 `ps`；與 --password 互斥 |
| `--solution` |  |  | solution 的 id 或名稱（必填） |
| `--system-volume-size` |  | `0` | 系統磁碟大小（GB） |
| `--system-volume-type` |  |  | 系統磁碟類型：local_disk\|block_storage-hdd（flavor 的 DISK 為 0 時必填，並搭配 --system-volume-size；缺少時 API 回 500 'system_volume_type'） |
| `--volume-size` |  | `0` | 資料磁碟大小（GB） |
| `--volume-type` |  |  | 資料磁碟類型：hdd\|ssd\|LUKS-hdd\|LUKS-ssd |

## twaictl vcs eip

管理 VCS 浮動 IP（Elastic IP）

**Usage**

```
twaictl vcs eip
```

**Flags**：無

## twaictl vcs eip create

申請新的浮動 IP

申請新的浮動 IP（STATIC EIP）。

注意：僅 tenant admin 可用，一般使用者呼叫可能得到 403。

可用欄位（--columns，不分大小寫）：ID, Address, Type, Status, Status reason, Resource, Desc, Created
預設欄位：ID, Address, Type, Status, Status reason, Resource, Desc, Created

**Usage**

```
twaictl vcs eip create
```

**Flags**：無

## twaictl vcs eip get

顯示單一浮動 IP 的詳細資料

顯示單一浮動 IP 的詳細資料。

可用欄位（--columns，不分大小寫）：ID, Address, Type, Status, Status reason, Resource, Desc, Created
預設欄位：ID, Address, Type, Status, Status reason, Resource, Desc, Created

**Usage**

```
twaictl vcs eip get <id|address>
```

**Flags**：無

## twaictl vcs eip ls

列出浮動 IP

列出浮動 IP。

可用欄位（--columns，不分大小寫）：ID, ADDRESS, TYPE, STATUS, RESOURCE, DESC, CREATED
預設欄位：ID, ADDRESS, TYPE, STATUS, RESOURCE, DESC, CREATED

**Usage**

```
twaictl vcs eip ls [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--address` |  |  | 以 IP 位址篩選 |

## twaictl vcs eip rm

刪除浮動 IP（破壞性操作，需確認或 -y）

**Usage**

```
twaictl vcs eip rm <id|address>...
```

**Flags**：無

## twaictl vcs eip update

更新浮動 IP 的描述

更新浮動 IP 的描述。

可用欄位（--columns，不分大小寫）：ID, Address, Type, Status, Status reason, Resource, Desc, Created
預設欄位：ID, Address, Type, Status, Status reason, Resource, Desc, Created

**Usage**

```
twaictl vcs eip update <id|address> --desc <text> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--desc` |  |  | 描述（必填；可為空字串以清除描述） |

## twaictl vcs events

顯示 VCS 實例的事件紀錄

顯示 VCS 實例的事件紀錄。

可用欄位（--columns，不分大小寫）：TIME, STATUS, RESOURCE, REASON
預設欄位：TIME, STATUS, RESOURCE, REASON

**Usage**

```
twaictl vcs events <id|name>
```

**Flags**：無

## twaictl vcs firewall

管理 firewall 與 firewall rule（tenant admin 限定）

**Usage**

```
twaictl vcs firewall
```

**Flags**：無

## twaictl vcs firewall create

建立 firewall（可同時指定規則與要套用的私有網路）

建立 firewall。--rule／--network 可重複指定，接受 id 或名稱。

可用欄位（--columns，不分大小寫）：ID, Name, Status, Desc, User
預設欄位：ID, Name, Status, Desc, User

**Usage**

```
twaictl vcs firewall create --name <n> [--desc <d>] [--rule <id|name>]... [--network <id|name>]... [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--desc` |  |  | 描述 |
| `--name` |  |  | firewall 名稱（必填） |
| `--network` |  | `[]` | 要套用此 firewall 的私有網路 id 或名稱（可重複） |
| `--rule` |  | `[]` | firewall rule 的 id 或名稱（可重複） |

## twaictl vcs firewall get

顯示單一 firewall 的詳細資料

顯示單一 firewall 的詳細資料。

可用欄位（--columns，不分大小寫）：ID, Name, Status, Status reason, Desc, Rules, Networks, Created, Project, User, Platform
預設欄位：ID, Name, Status, Status reason, Desc, Rules, Networks, Created, Project, User

**Usage**

```
twaictl vcs firewall get <id|name>
```

**Flags**：無

## twaictl vcs firewall ls

列出 firewall

列出 firewall。

可用欄位（--columns，不分大小寫）：ID, NAME, STATUS, DESC, USER, PLATFORM
預設欄位：ID, NAME, STATUS, DESC

**Usage**

```
twaictl vcs firewall ls
```

**Flags**：無

## twaictl vcs firewall rm

刪除 firewall（破壞性操作，需確認或 -y）

**Usage**

```
twaictl vcs firewall rm <id|name>...
```

**Flags**：無

## twaictl vcs firewall rule

管理 firewall rule（tenant admin 限定）

**Usage**

```
twaictl vcs firewall rule
```

**Flags**：無

## twaictl vcs firewall rule create

建立 firewall rule

建立 firewall rule；未指定的欄位不送出，由 API 採預設值。

可用欄位（--columns，不分大小寫）：ID, Name, Protocol, Action, Source, Destination
預設欄位：ID, Name, Protocol, Action, Source, Destination

**Usage**

```
twaictl vcs firewall rule create --name <n> [--protocol icmp|tcp|udp] [--action allow|deny|reject] [--src <ip|cidr>] [--src-port <p>] [--dst <ip|cidr>] [--dst-port <p>] [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--action` |  |  | 動作：allow\|deny\|reject |
| `--dst` |  |  | 目的 IPv4／IPv6 位址或 CIDR |
| `--dst-port` |  |  | 目的 port 或範圍（例如 22 或 80:90） |
| `--name` |  |  | 規則名稱（必填） |
| `--protocol` |  |  | 協定：icmp\|tcp\|udp |
| `--src` |  |  | 來源 IPv4／IPv6 位址或 CIDR |
| `--src-port` |  |  | 來源 port 或範圍（例如 22 或 80:90） |

## twaictl vcs firewall rule get

顯示單一 firewall rule 的詳細資料

顯示單一 firewall rule 的詳細資料。

可用欄位（--columns，不分大小寫）：ID, Name, Protocol, Action, Source IP, Source port, Destination IP, Destination port, IP version, Created, Project, Platform
預設欄位：ID, Name, Protocol, Action, Source IP, Source port, Destination IP, Destination port, IP version, Created, Project

**Usage**

```
twaictl vcs firewall rule get <id|name>
```

**Flags**：無

## twaictl vcs firewall rule ls

列出 firewall rule

列出 firewall rule。

可用欄位（--columns，不分大小寫）：ID, NAME, PROTOCOL, ACTION, SOURCE, DESTINATION, IP VERSION, PLATFORM
預設欄位：ID, NAME, PROTOCOL, ACTION, SOURCE, DESTINATION

**Usage**

```
twaictl vcs firewall rule ls
```

**Flags**：無

## twaictl vcs firewall rule rm

刪除 firewall rule（破壞性操作，需確認或 -y）

**Usage**

```
twaictl vcs firewall rule rm <id|name>...
```

**Flags**：無

## twaictl vcs firewall rule update

更新 firewall rule（只送出有指定的欄位）

更新 firewall rule；只送出有指定的 flag，其餘欄位維持原值。實測（2026-08-29）source/destination 的 IP 與 port 四個欄位都不接受空字串（API 回 503），無法用空字串清除，需刪除重建；twaictl 仍照原樣送出由 API 決定。

可用欄位（--columns，不分大小寫）：ID, Name, Protocol, Action, Source IP, Source port, Destination IP, Destination port, IP version, Created, Project, Platform
預設欄位：ID, Name, Protocol, Action, Source IP, Source port, Destination IP, Destination port, IP version, Created, Project

**Usage**

```
twaictl vcs firewall rule update <id|name> [--name <n>] [--protocol ...] [--action ...] [--src ...] [--src-port ...] [--dst ...] [--dst-port ...] [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--action` |  |  | 動作：allow\|deny\|reject |
| `--dst` |  |  | 目的 IPv4／IPv6 位址或 CIDR |
| `--dst-port` |  |  | 目的 port 或範圍（例如 22 或 80:90） |
| `--name` |  |  | 規則名稱 |
| `--protocol` |  |  | 協定：icmp\|tcp\|udp |
| `--src` |  |  | 來源 IPv4／IPv6 位址或 CIDR |
| `--src-port` |  |  | 來源 port 或範圍（例如 22 或 80:90） |

## twaictl vcs firewall update

更新 firewall（規則／網路清單為整份取代）

更新 firewall。--rule／--network 會以指定的清單整份取代既有設定（API 為 PATCH 整份取代語意，沒有增量新增／移除）；--clear-rules／--clear-networks 清空對應清單；未指定的欄位維持原值。

可用欄位（--columns，不分大小寫）：ID, Name, Status, Status reason, Desc, Rules, Networks, Created, Project, User, Platform
預設欄位：ID, Name, Status, Status reason, Desc, Rules, Networks, Created, Project, User

**Usage**

```
twaictl vcs firewall update <id|name> [--desc <d>] [--rule <id|name>]... [--network <id|name>]... [--clear-rules] [--clear-networks] [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--clear-networks` |  | `false` | 清空套用的私有網路（與 --network 互斥） |
| `--clear-rules` |  | `false` | 清空規則清單（與 --rule 互斥） |
| `--desc` |  |  | 描述（可指定空字串清除） |
| `--network` |  | `[]` | 以此清單整份取代套用的私有網路（id 或名稱，可重複） |
| `--rule` |  | `[]` | 以此清單整份取代 firewall 的規則（id 或名稱，可重複） |

## twaictl vcs flavor

查詢 VCS flavor（虛擬機規格）

**Usage**

```
twaictl vcs flavor
```

**Flags**：無

## twaictl vcs flavor ls

列出 flavor

列出 flavor。

可用欄位（--columns，不分大小寫）：ID, NAME, CPU, MEMORY (MB), DISK (GB), GPU, GPU TYPE, PUBLIC, ENABLED, PLATFORM
預設欄位：ID, NAME, CPU, MEMORY (MB), DISK (GB), GPU, GPU TYPE, PUBLIC

**Usage**

```
twaictl vcs flavor ls [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--all-projects` |  | `false` | 列出所有 project 可用的 flavor（不以 project 過濾） |

## twaictl vcs get

顯示單一 VCS 實例的詳細資料

顯示單一 VCS 實例的詳細資料。

可用欄位（--columns，不分大小寫）：ID, Name, Status, Public IP, Solution, Project, User, Created, Servers, Progress, Termination protection
預設欄位：ID, Name, Status, Public IP, Solution, Project, User, Created, Servers, Progress, Termination protection

**Usage**

```
twaictl vcs get <id|name>
```

**Flags**：無

## twaictl vcs image

管理 VCS image

**Usage**

```
twaictl vcs image
```

**Flags**：無

## twaictl vcs image get

顯示單一 image 的詳細資料

顯示單一 image 的詳細資料。

可用欄位（--columns，不分大小寫）：ID, Name, Status, OS, OS version, Size, Public, Enabled, Server, Created, Desc
預設欄位：ID, Name, Status, OS, OS version, Size, Public, Enabled, Server, Created, Desc

**Usage**

```
twaictl vcs image get <id|name>
```

**Flags**：無

## twaictl vcs image ls

列出 image

列出 image。

可用欄位（--columns，不分大小寫）：ID, NAME, STATUS, SIZE, PUBLIC, CREATED, SERVER, DESCRIPTION, USER
預設欄位：ID, NAME, STATUS, SIZE, PUBLIC, CREATED

**Usage**

```
twaictl vcs image ls
```

**Flags**：無

## twaictl vcs image rm

刪除 image（破壞性操作，需確認或 -y）

**Usage**

```
twaictl vcs image rm <id|name>...
```

**Flags**：無

## twaictl vcs image save

把 VCS 實例目前的狀態存成新 image

把 VCS 實例目前的狀態存成新 image。

可用欄位（--columns，不分大小寫）：ID, Name, Platform, Public, Created
預設欄位：ID, Name, Platform, Public, Created

**Usage**

```
twaictl vcs image save <site id|name> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--desc` |  |  | image 描述 |
| `--name` |  |  | image 名稱（必填） |
| `--os` |  |  | 作業系統（必填，例如 Linux；spec 標選填但 API 實際要求） |
| `--os-version` |  |  | 作業系統版本（必填，例如 "Ubuntu 24.04"；spec 標選填但 API 實際要求） |
| `--server` |  |  | 指定 server id（未指定時由 site 自動解析，僅限剛好一台 server 的 site） |

## twaictl vcs keypair

管理 VCS SSH 金鑰對

**Usage**

```
twaictl vcs keypair
```

**Flags**：無

## twaictl vcs keypair create

建立金鑰對（或以 --public-key-file 匯入既有公鑰）

**Usage**

```
twaictl vcs keypair create <name> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--out` |  |  | 把 private key 寫入指定檔案（權限 0600），而非印到 stdout；檔案已存在時不覆寫，且不可與 -o json/yaml 併用 |
| `--public-key-file` |  |  | 匯入既有公鑰的檔案路徑；不指定則由伺服器產生新的金鑰對 |

## twaictl vcs keypair get

顯示單一金鑰對的詳細資料

顯示單一金鑰對的詳細資料。

可用欄位（--columns，不分大小寫）：Name, Fingerprint, Created, User, Public key
預設欄位：Name, Fingerprint, Created, User, Public key

**Usage**

```
twaictl vcs keypair get <name>
```

**Flags**：無

## twaictl vcs keypair ls

列出金鑰對

列出金鑰對。

可用欄位（--columns，不分大小寫）：NAME, FINGERPRINT, USER, PLATFORM
預設欄位：NAME, FINGERPRINT, USER

**Usage**

```
twaictl vcs keypair ls
```

**Flags**：無

## twaictl vcs keypair rm

刪除金鑰對（破壞性操作，需確認或 -y）

**Usage**

```
twaictl vcs keypair rm <name>...
```

**Flags**：無

## twaictl vcs lb

管理 load balancer

**Usage**

```
twaictl vcs lb
```

**Flags**：無

## twaictl vcs lb action

對 load balancer 送出動作（關聯／解除浮動 IP、更新 listener 逾時參數）

對 load balancer 送出動作：
  associate-ip：關聯浮動 IP（需 --eip）
  disassociate-ip：解除浮動 IP（破壞性操作，需確認或 -y）
  update-listener：更新 listener 逾時參數（需 --listener，四個 --timeout-* 各自選填，
    但至少要指定一個，否則這次動作不會改變任何逾時設定）

--eip 只能搭配 associate-ip；--listener 與四個 --timeout-* 只能搭配 update-listener，
用在其他動作會在送出請求前直接報錯。

--wait 的限制：動作送出後 load balancer 通常仍是 ACTIVE（更新尚未真的開始），因此 --wait 會先等到觀察到 status 離開 ACTIVE、再等它回到 ACTIVE 才算完成；若整個更新過程比 --wait-interval（預設 5s）還快、沒有輪詢到「已離開 ACTIVE」的瞬間，會持續等到 --wait-timeout 逾時（exit 5），此時實際上動作多半已經完成，只是 --wait 沒能觀察到。

**Usage**

```
twaictl vcs lb action <id|name> <associate-ip|disassociate-ip|update-listener> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--eip` |  |  | 浮動 IP 的 id 或位址（associate-ip 必填） |
| `--listener` |  |  | listener 名稱（update-listener 必填） |
| `--timeout-client-data` |  | `0` | listener frontend client 逾時（毫秒，僅 update-listener） |
| `--timeout-member-connect` |  | `0` | listener backend member 連線逾時（毫秒，僅 update-listener） |
| `--timeout-member-data` |  | `0` | listener backend member 逾時（毫秒，僅 update-listener） |
| `--timeout-tcp-inspect` |  | `0` | TCP content inspection 等待時間（毫秒，僅 update-listener） |

## twaictl vcs lb create

建立 load balancer

建立 load balancer。

簡寫模式：--member（可重複）與 --port 指定單一 pool + 單一 listener 的組態；
--spec 模式：以 YAML 或 JSON 檔案指定完整的 pools/listeners（`@` 前綴可省略），兩者不可併用。

--spec 檔案格式範例：

pools:
  - name: spec-pool
    protocol: HTTP
    method: ROUND_ROBIN
    members:
      - ip: 10.0.1.1
        port: 8080
listeners:
  - name: spec-listener
    pool_name: spec-pool
    protocol: HTTP
    protocol_port: 8080

未加 --wait 時輸出建立回應（POST）的欄位：
可用欄位（--columns，不分大小寫）：ID, NAME, STATUS, NETWORK, LISTENERS, POOLS, CREATED, DESC, USER
預設欄位：ID, NAME, STATUS, NETWORK, LISTENERS, POOLS, CREATED

加 --wait 時會等到 Active 後重新查詢，改輸出與 `vcs lb get` 相同的欄位：
可用欄位（--columns，不分大小寫）：ID, Name, Status, Status reason, VIP, Network, Listeners, Pools, Total connections, Active connections, Created, Desc, User
預設欄位：ID, Name, Status, Status reason, VIP, Network, Listeners, Pools, Total connections, Active connections, Created, Desc, User

**Usage**

```
twaictl vcs lb create --name <n> --network <id|name> (--member <ip:port[:weight]> --port <port> | --spec <file>) [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--desc` |  |  | 描述 |
| `--eip` |  |  | 要綁定的浮動 IP（id 或位址） |
| `--listener-protocol` |  |  | listener 的 protocol：TCP\|HTTP\|HTTPS\|TERMINATED_HTTPS（未指定時同 --pool-protocol） |
| `--member` |  | `[]` | pool member，格式 ip:port[:weight]（IPv6 請用 [addr]:port[:weight]；可重複；簡寫模式必填） |
| `--method` |  | `ROUND_ROBIN` | pool 的負載平衡演算法：ROUND_ROBIN\|LEAST_CONNECTIONS\|SOURCE_IP |
| `--monitor-type` |  |  | health monitor 類型：HTTP\|HTTPS\|PING\|TCP（不指定則不設定 monitor） |
| `--name` |  |  | load balancer 名稱（必填） |
| `--network` |  |  | private network 的 id 或名稱（必填） |
| `--pool-protocol` |  | `HTTP` | pool 的 protocol：TCP\|HTTP\|HTTPS |
| `--port` |  | `0` | listener 的 protocol port（簡寫模式必填） |
| `--spec` |  |  | load balancer spec 檔案（YAML 或 JSON，@ 前綴可省略；與簡寫 flag 互斥） |
| `--tls-secret` |  |  | TLS 憑證 secret 的 id 或名稱（僅 --listener-protocol TERMINATED_HTTPS 可用） |

## twaictl vcs lb get

顯示單一 load balancer 的詳細資料

顯示單一 load balancer 的詳細資料。

可用欄位（--columns，不分大小寫）：ID, Name, Status, Status reason, VIP, Network, Listeners, Pools, Total connections, Active connections, Created, Desc, User
預設欄位：ID, Name, Status, Status reason, VIP, Network, Listeners, Pools, Total connections, Active connections, Created, Desc, User

**Usage**

```
twaictl vcs lb get <id|name>
```

**Flags**：無

## twaictl vcs lb ls

列出 load balancer

列出 load balancer。

可用欄位（--columns，不分大小寫）：ID, NAME, STATUS, NETWORK, LISTENERS, POOLS, CREATED, DESC, USER
預設欄位：ID, NAME, STATUS, NETWORK, LISTENERS, POOLS, CREATED

**Usage**

```
twaictl vcs lb ls
```

**Flags**：無

## twaictl vcs lb report

查詢 load balancer 的建立／刪除報表（project）或 member 流量報表

查詢 load balancer 報表，依是否指定 &lt;id|name&gt; 分成兩種：

不帶 &lt;id|name&gt;：查詢目前 project 下所有 load balancer 的建立／刪除報表，
期間與 total_lb_num 會印在 stderr（進度訊息）：
可用欄位（--columns，不分大小寫）：ID, NAME, USER, CREATED, DELETED
預設欄位：ID, NAME, USER, CREATED, DELETED

帶 &lt;id|name&gt;：查詢該 load balancer 底下指定 pool member（以 IP 表示）的流量報表，
必須搭配 --member：
可用欄位（--columns，不分大小寫）：TIME, BYTES READ, RAW BYTES
預設欄位：TIME, BYTES READ

--interval／--unit 必須同時指定或同時省略，--interval 指定時須大於 0。

--begin/--end 請用 RFC3339 格式（例如 2026-08-27T12:00:00Z）；未指定時 API 預設 end=now、begin=一個月前。

注意：API 對不存在的 load balancer 查詢 member 報表會回 503（實測，非本工具造成），twaictl 照實回報錯誤（exit 3）。

**Usage**

```
twaictl vcs lb report [<id|name>] [--member <ip>] [--begin <time>] [--end <time>] [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--begin` |  |  | 查詢開始時間（RFC3339，例如 2026-08-27T12:00:00Z） |
| `--end` |  |  | 查詢結束時間（RFC3339，例如 2026-08-27T12:00:00Z） |
| `--interval` |  | `0` | 計算區間長度（僅 member 報表，須大於 0，需搭配 --unit） |
| `--member` |  |  | pool member 的 ip（帶 &lt;id\|name&gt; 時必填） |
| `--method` |  |  | 計算方式（僅 member 報表）：sum\|avg |
| `--unit` |  |  | 計算區間單位（僅 member 報表，需搭配 --interval）：s\|m\|h\|d\|w\|M\|y |

## twaictl vcs lb rm

刪除 load balancer（破壞性操作，需確認或 -y）

**Usage**

```
twaictl vcs lb rm <id|name>...
```

**Flags**：無

## twaictl vcs lb update

更新 load balancer 的 pools/listeners（整份取代）

更新 load balancer 的 pools/listeners。

注意：PATCH 會以 --spec 內容整份取代目前的 pools/listeners，不是合併。

--spec 檔案格式範例：

pools:
  - name: spec-pool
    protocol: HTTP
    method: ROUND_ROBIN
    members:
      - ip: 10.0.1.1
        port: 8080
listeners:
  - name: spec-listener
    pool_name: spec-pool
    protocol: HTTP
    protocol_port: 8080

可用欄位（--columns，不分大小寫）：ID, Name, Status, Status reason, VIP, Network, Listeners, Pools, Total connections, Active connections, Created, Desc, User
預設欄位：ID, Name, Status, Status reason, VIP, Network, Listeners, Pools, Total connections, Active connections, Created, Desc, User

**Usage**

```
twaictl vcs lb update <id|name> --spec <file> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--spec` |  |  | load balancer spec 檔案（YAML 或 JSON，@ 前綴可省略；必填） |

## twaictl vcs ls

列出 VCS 實例

列出 VCS 實例。

可用欄位（--columns，不分大小寫）：ID, NAME, STATUS, PUBLIC IP, HOSTNAME, CREATED, SOLUTION, PROJECT, USER, PROGRESS
預設欄位：ID, NAME, STATUS, PUBLIC IP, HOSTNAME, CREATED

**Usage**

```
twaictl vcs ls [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--all` |  | `false` | 列出所有使用者的 VCS 實例（all_users=1） |

## twaictl vcs metrics

查詢 VCS 實例底下 server 的用量指標

查詢 VCS 實例底下 server 的用量指標。

可用欄位（--columns，不分大小寫）：TIME, VALUE, UNIT
預設欄位：TIME, VALUE, UNIT

--meter disk-read/disk-write 時（多一個 DEVICE 欄位）：
可用欄位（--columns，不分大小寫）：DEVICE, TIME, VALUE, UNIT
預設欄位：DEVICE, TIME, VALUE, UNIT

--begin/--end 請用 RFC3339 格式（例如 2026-08-27T12:00:00Z）；未指定時部分 meter 會回傳全部歷史資料（筆數可能很多）。

注意：net-in 目前伺服器端會回 500（API 自身的已知問題，非本工具造成，見 docs/api-notes.md A22），twaictl 會照實回報錯誤。

**Usage**

```
twaictl vcs metrics <id|name> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--begin` |  |  | 查詢開始時間（RFC3339，例如 2026-08-27T12:00:00Z） |
| `--end` |  |  | 查詢結束時間（RFC3339，例如 2026-08-27T12:00:00Z） |
| `--meter` |  | `cpu` | 指標類型：cpu\|memory\|disk-read\|disk-write\|net-in\|net-out |
| `--server` |  |  | 指定 server id（未指定時由 site 自動解析，僅限剛好一台 server 的 site） |

## twaictl vcs network

管理 VCS 網路

**Usage**

```
twaictl vcs network
```

**Flags**：無

## twaictl vcs network create

建立網路

建立網路。

注意：僅 tenant admin 可用，一般使用者呼叫可能得到 403。

可用欄位（--columns，不分大小寫）：ID, NAME, CIDR, GATEWAY, STATUS, ROUTER, CREATED, NAMESERVERS, EXT NET, DNS DOMAIN
預設欄位：ID, NAME, CIDR, GATEWAY, STATUS, ROUTER, CREATED

**Usage**

```
twaictl vcs network create --name <n> --cidr <cidr> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--cidr` |  |  | CIDR（必填，例如 10.0.0.0/24） |
| `--dns-domain` |  |  | DNS domain |
| `--gateway` |  |  | gateway IP |
| `--name` |  |  | 網路名稱（必填） |
| `--router` |  | `false` | 是否建立路由器 |

## twaictl vcs network get

顯示單一網路的詳細資料

顯示單一網路的詳細資料。

可用欄位（--columns，不分大小寫）：ID, Name, CIDR, Gateway, Status, Router, Nameservers, Ext net, Firewall, Created
預設欄位：ID, Name, CIDR, Gateway, Status, Router, Nameservers, Ext net, Firewall, Created

**Usage**

```
twaictl vcs network get <id|name>
```

**Flags**：無

## twaictl vcs network ls

列出網路

列出網路。

可用欄位（--columns，不分大小寫）：ID, NAME, CIDR, GATEWAY, STATUS, ROUTER, CREATED, NAMESERVERS, EXT NET, DNS DOMAIN
預設欄位：ID, NAME, CIDR, GATEWAY, STATUS, ROUTER, CREATED

**Usage**

```
twaictl vcs network ls
```

**Flags**：無

## twaictl vcs network rm

刪除網路（破壞性操作，需確認或 -y）

**Usage**

```
twaictl vcs network rm <id|name>...
```

**Flags**：無

## twaictl vcs rm

刪除 VCS 實例（破壞性操作，需確認或 -y）

**Usage**

```
twaictl vcs rm <id|name>...
```

**Flags**：無

## twaictl vcs secg

管理 VCS security group 規則

**Usage**

```
twaictl vcs secg
```

**Flags**：無

## twaictl vcs secg add-rule

對 security group 新增一條規則

**Usage**

```
twaictl vcs secg add-rule <security-group-id> --direction <ingress|egress> --protocol <tcp|udp|icmp|udplite|sctp|dccp> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--direction` |  |  | 規則方向（必填）：ingress\|egress |
| `--port-max` |  | `0` | port range 上限（1-65535，省略時等於 --port-min） |
| `--port-min` |  | `0` | port range 下限（1-65535，省略代表不限） |
| `--protocol` |  |  | 協定（必填）：tcp\|udp\|icmp\|udplite\|sctp\|dccp |
| `--remote` |  |  | 來源／目的 CIDR（例如 0.0.0.0/0） |

## twaictl vcs secg ls

列出 VCS 實例的 security group 規則

列出 VCS 實例的 security group 規則。

可用欄位（--columns，不分大小寫）：SECURITY GROUP, RULE ID, DIRECTION, PROTOCOL, PORTS, REMOTE, ETHERTYPE
預設欄位：SECURITY GROUP, RULE ID, DIRECTION, PROTOCOL, PORTS, REMOTE, ETHERTYPE

**Usage**

```
twaictl vcs secg ls <id|name> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--server` |  |  | 指定 server id（未指定時由 site 自動解析，僅限剛好一台 server 的 site） |

## twaictl vcs secg rm-rule

刪除 security group 規則（破壞性操作，需確認或 -y）

**Usage**

```
twaictl vcs secg rm-rule <rule-id>...
```

**Flags**：無

## twaictl vcs secret

管理 secret（TLS 憑證等，供 load balancer 使用）

**Usage**

```
twaictl vcs secret
```

**Flags**：無

## twaictl vcs secret create

建立 secret

建立 secret。

payload 由 twaictl 讀入後以 base64 編碼送出；--dry-run 的 curl 會把 payload 遮蔽為 ***。

實測：payload 必須是 PKCS#12 bundle，PEM／DER 憑證會被 API 以 `Invalid certification` 拒絕；--expire 可省略（回應 expire_time 為 null）。

可用欄位（--columns，不分大小寫）：ID, Name, Status, Desc, Expires, Created, Project, User
預設欄位：ID, Name, Status, Desc, Expires, Created, Project, User

**Usage**

```
twaictl vcs secret create --name <n> (--payload-file <path> | --payload-stdin) [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--desc` |  |  | 描述 |
| `--expire` |  |  | 到期時間（RFC3339，例如 2027-01-01T00:00:00Z） |
| `--name` |  |  | secret 名稱（必填） |
| `--payload-file` |  |  | PKCS#12（.p12／.pfx，含憑證與私鑰，可無密碼）檔案內容；twaictl 讀入後以 base64 編碼送出（與 --payload-stdin 擇一） |
| `--payload-stdin` |  | `false` | PKCS#12（.p12／.pfx，含憑證與私鑰，可無密碼）檔案內容；twaictl 讀入後以 base64 編碼送出（從 stdin 原樣讀入，不去除結尾換行；與 --payload-file 擇一） |

## twaictl vcs secret get

顯示單一 secret 的詳細資料

顯示單一 secret 的詳細資料。

可用欄位（--columns，不分大小寫）：ID, Name, Status, Desc, Expires, Created, Project, User
預設欄位：ID, Name, Status, Desc, Expires, Created, Project, User

**Usage**

```
twaictl vcs secret get <id|name>
```

**Flags**：無

## twaictl vcs secret ls

列出 secret

列出 secret。

可用欄位（--columns，不分大小寫）：ID, NAME, STATUS, EXPIRES, CREATED, DESC, USER
預設欄位：ID, NAME, STATUS, EXPIRES, CREATED

**Usage**

```
twaictl vcs secret ls
```

**Flags**：無

## twaictl vcs secret rm

刪除 secret（破壞性操作，需確認或 -y）

**Usage**

```
twaictl vcs secret rm <id|name>...
```

**Flags**：無

## twaictl vcs server

查詢 VCS 實例底下的 server

**Usage**

```
twaictl vcs server
```

**Flags**：無

## twaictl vcs server get

顯示單一 server 的詳細資料

顯示單一 server 的詳細資料。

可用欄位（--columns，不分大小寫）：ID, Hostname, Site, Status, Flavor, Image, OS, OS version, Keypair, Availability zone, FQDN, Private nets, Public nets
預設欄位：ID, Hostname, Site, Status, Flavor, Image, OS, OS version, Keypair, Availability zone, FQDN, Private nets, Public nets

**Usage**

```
twaictl vcs server get <id>
```

**Flags**：無

## twaictl vcs server ls

列出 project 下的所有 server

列出 project 下的所有 server。

可用欄位（--columns，不分大小寫）：ID, HOSTNAME, SITE, STATUS, FLAVOR, IMAGE, PRIVATE IP, PUBLIC IP, KEYPAIR, OS, AZ, FQDN
預設欄位：ID, HOSTNAME, SITE, STATUS, FLAVOR, IMAGE, PRIVATE IP, PUBLIC IP

**Usage**

```
twaictl vcs server ls
```

**Flags**：無

## twaictl vcs snapshot

管理 VCS volume snapshot

**Usage**

```
twaictl vcs snapshot
```

**Flags**：無

## twaictl vcs snapshot create

建立 snapshot

建立 snapshot。

可用欄位（--columns，不分大小寫）：ID, Name, Status, Volume, Volume ID, Created, Desc
預設欄位：ID, Name, Status, Volume, Volume ID, Created, Desc

**Usage**

```
twaictl vcs snapshot create --name <n> --volume <id|name> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--desc` |  |  | 描述 |
| `--name` |  |  | snapshot 名稱（必填） |
| `--volume` |  |  | 來源 volume 的 id 或名稱（必填） |

## twaictl vcs snapshot get

顯示單一 snapshot 的詳細資料

顯示單一 snapshot 的詳細資料。

可用欄位（--columns，不分大小寫）：ID, Name, Status, Volume, Volume ID, Created, Desc
預設欄位：ID, Name, Status, Volume, Volume ID, Created, Desc

**Usage**

```
twaictl vcs snapshot get <id|name>
```

**Flags**：無

## twaictl vcs snapshot ls

列出 snapshot

列出 snapshot。

可用欄位（--columns，不分大小寫）：ID, NAME, STATUS, VOLUME, CREATED, DESC
預設欄位：ID, NAME, STATUS, VOLUME, CREATED, DESC

**Usage**

```
twaictl vcs snapshot ls [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--status` |  |  | 以狀態篩選 |

## twaictl vcs snapshot rm

刪除 snapshot（破壞性操作，需確認或 -y）

**Usage**

```
twaictl vcs snapshot rm <id|name>...
```

**Flags**：無

## twaictl vcs volume

管理 VCS volume（磁碟）

**Usage**

```
twaictl vcs volume
```

**Flags**：無

## twaictl vcs volume action

對 volume 執行 attach／detach／extend

對 volume 執行 attach／detach／extend。

可用欄位（--columns，不分大小寫）：Volume ID, Server ID, Mountpoint
預設欄位：Volume ID, Server ID, Mountpoint

**Usage**

```
twaictl vcs volume action <id|name> <attach|detach|extend> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--server` |  |  | server id（attach/detach 用，與 --site 擇一） |
| `--site` |  |  | VCS 實例 site 的 id 或名稱（attach/detach 用，與 --server 擇一） |
| `--size` |  | `0` | 擴充後容量（GB，extend 用，必須大於 0） |

## twaictl vcs volume create

建立 volume

建立 volume。

可用欄位（--columns，不分大小寫）：ID, Name, Platform, Size (GB), Type
預設欄位：ID, Name, Platform, Size (GB), Type

**Usage**

```
twaictl vcs volume create --name <n> --size <GB> [flags]
```

**Flags**

| 名稱 | 簡寫 | 預設 | 說明 |
|---|---|---|---|
| `--desc` |  |  | 描述 |
| `--name` |  |  | volume 名稱（必填） |
| `--size` |  | `0` | 容量（GB，必填，必須大於 0） |
| `--type` |  |  | volume 類型：hdd\|ssd\|LUKS-hdd\|LUKS-ssd |

## twaictl vcs volume get

顯示單一 volume 的詳細資料

顯示單一 volume 的詳細資料。

可用欄位（--columns，不分大小寫）：ID, Name, Size (GB), Type, Status, Attached, Mountpoint, Bootable, UUID, Project, Created
預設欄位：ID, Name, Size (GB), Type, Status, Attached, Mountpoint, Bootable, UUID, Project, Created

**Usage**

```
twaictl vcs volume get <id|name>
```

**Flags**：無

## twaictl vcs volume ls

列出 volume

列出 volume。

可用欄位（--columns，不分大小寫）：ID, NAME, SIZE (GB), TYPE, STATUS, ATTACHED, BOOTABLE, CREATED
預設欄位：ID, NAME, SIZE (GB), TYPE, STATUS, ATTACHED, BOOTABLE, CREATED

**Usage**

```
twaictl vcs volume ls
```

**Flags**：無

## twaictl vcs volume rm

刪除 volume（破壞性操作，需確認或 -y）

**Usage**

```
twaictl vcs volume rm <id|name>...
```

**Flags**：無

## twaictl version

顯示 twaictl 版本、commit、build 時間與依據的 OpenAPI 版本

**Usage**

```
twaictl version
```

**Flags**：無

