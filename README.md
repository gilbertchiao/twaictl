# twaictl

[![CI](https://github.com/gilbertchiao/twaictl/actions/workflows/ci.yml/badge.svg)](https://github.com/gilbertchiao/twaictl/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/gilbertchiao/twaictl)](https://github.com/gilbertchiao/twaictl/releases)
[![License](https://img.shields.io/github/license/gilbertchiao/twaictl)](LICENSE)

> 本工具為社群開發的非官方（unofficial）命令列工具，與台智雲（Taiwan AI Cloud, TWAI）無任何隸屬或背書關係。
> This is an unofficial, community-maintained CLI and is not affiliated with or endorsed by Taiwan AI Cloud (TWAI).

> 文件、CHANGELOG 與程式碼註解中提到的 `docs/DESIGN.md`、`docs/api-notes.md`（含 A1、A2… 編號）
> 是作者的設計文件與真實環境實測筆記，**不隨本 repo 公開**；相關結論已反映在程式行為、
> `--help` 說明與本 README 中。

`twaictl` 是以 Go 重新實作的 TWCC 平台命令列工具，用來取代已停止維護、且在 Python 3.12+ 無法安裝的官方 `twcc/TWCC-CLI`。單一 static binary，不依賴系統 Python。

**目前支援 VCS（虛擬機與其網路、磁碟、LB、防火牆、auto scaling）與 COS（S3 相容物件儲存）**。可用命令：`version`、
`config init`、`config show`、`config whoami`、`project ls/get/quota/solutions`、
`vcs ls/get/create/rm`、`vcs action`、`vcs events`、`vcs metrics`、`vcs server ls/get`、
`vcs flavor ls`、`vcs image ls/get/rm/save`、`vcs keypair ls/get/create/rm`、
`vcs network ls/get/create/rm`、`vcs eip ls/get/create/rm/update`、
`vcs volume ls/get/create/rm/action`、`vcs snapshot ls/get/create/rm`、
`vcs secg ls/add-rule/rm-rule`、`vcs secret ls/get/create/rm`、
`vcs lb ls/get/create/update/rm/action/report`、
`vcs firewall ls/get/create/update/rm`、`vcs firewall rule ls/get/create/update/rm`、
`vcs asp ls/get/create/rm/attach/detach`、`cos key get/create/renew/rm`、
`cos bucket ls/create/rm`（`rm` 支援 `--purge`）、`cos ls`（支援 `--versions`）、`cos cp`、
`cos rm`（支援 `--all-versions`）、`cos sync`、`cos acl`、`cos content-type`、`cos usage`、
`api`（逃生口）、`completion`。完整命令與 flag 列表見自動產生的
[`docs/commands.md`](docs/commands.md)。

> **已知限制**：所有命令皆已於真實環境完成 read-only 或寫入型驗證（結果見
> `docs/api-notes.md`）。實測發現 `vcs network create` 建立的網路會在數秒後轉為 `ERROR`
> 狀態（API 行為，見 api-notes「2026-09-06 實測」）；`vcs eip create` 建立的 STATIC IP 會立即占用
> `static_ip` 配額並開始計費，請用完即以 `vcs eip rm` 釋放。

> 注意：台智雲已公告 **CCS（容器）服務於 2026-08-31 終止、HPC（Slurm）高速運算服務於
> 2026-08-30 終止**，本工具不實作 CCS / HPC 相關命令。

## 安裝

- **GitHub Release**：到 [Releases](https://github.com/gilbertchiao/twaictl/releases) 下載對應平台的壓縮檔，解壓後把 `twaictl` 放到 `PATH`。
- **go install**：`go install github.com/gilbertchiao/twaictl/cmd/twaictl@latest`
- **Homebrew**（macOS / Linux）：`brew install gilbertchiao/tap/twaictl`

## 快速開始

```bash
twaictl config init          # 互動輸入 API key 與 project code
twaictl config show          # 確認生效設定（key 遮蔽）
twaictl config whoami        # 驗證 key 並列出可用 project
twaictl vcs ls               # 列出目前 project 下的 VCS 實例
twaictl vcs create --name web-01 --solution "Ubuntu 22.04" \
  --image "Ubuntu 22.04" --flavor v.2xsmall --keypair mykey --eip --wait
```

## 設定檔與環境變數

設定檔：`$XDG_CONFIG_HOME/twaictl/config.yaml`（預設 `~/.config/twaictl/config.yaml`），權限 0600。

```yaml
current_profile: default
profiles:
  default:
    api_key: "..."
    project_code: "GOVxxxxxx"
    apigateway: "https://apigateway.twcc.ai"   # 選填
    api_hosts:                                 # 選填，覆寫預設平台識別字串
      vcs: "openstack-taichung-default-2"
      cos: "ceph-taichung-default"
      common: "goc"
    cos:
      endpoint: "https://cos.twcc.ai"          # 選填
      access_key: ""
      secret_key: ""
```

優先序（高 → 低）：命令列 flag → 環境變數 → 設定檔 profile → 內建預設值。

| 環境變數 | 說明 |
|---|---|
| `TWAI_API_KEY` | API key（建議用此取代寫在檔案裡） |
| `TWAI_PROJECT_CODE` | project code |
| `TWAI_PROFILE` | 使用的 profile 名稱 |
| `TWAI_APIGATEWAY` | apigateway URL |
| `TWAI_API_HOST_VCS` / `_COS` / `_COMMON` | 各服務的 `x-api-host` |
| `TWAI_COS_ENDPOINT` / `TWAI_COS_ACCESS_KEY` / `TWAI_COS_SECRET_KEY` | COS S3 設定 |
| `_TWCC_API_KEY_` / `_TWCC_PROJECT_CODE_` | 舊版 twccli 變數，仍接受但會提示改用 `TWAI_*` |

`twaictl --profile <name> config init` 會建立或更新該 profile，並把 `current_profile` 切換為它。

`config init` 互動輸入 API key 時不會回顯；非終端機（pipe）環境則讀取一行。

## 全域 flags

`--profile`、`--project`、`-o/--output table|json|yaml`、`--columns`、`--no-header`、`-v/--verbose`、`--dry-run`、`--timeout`、`--wait`/`--wait-timeout`、`-y/--yes`。詳見 `twaictl --help`。

- `--project` 可接受 project **code**（例如 `GOVxxxxxx`）或**數字 id**；命令會先呼叫
  `GET /projects/?name=<code>` 把 code 解析成 id，再用 id 呼叫實際的資源 API。若已知數字
  id，直接傳數字可省下這次額外的解析請求（例如 `--project 123`）；搭配 `--dry-run` 時，
  數字 id 完全不會觸發任何請求（純數字 ref 不查詢清單即可直接使用），只有 code 才會先印
  出一次解析用的 `GET /projects/` curl 命令。`vcs get/rm` 的 `<id|name>`、
  `vcs create --solution`、`project get <id|code>` 都適用同樣的規則：純數字直接當 id 用，
  非數字才會先查清單再以名稱完全比對（找不到、或名稱重複對到多筆都會回報錯誤）。
- `--columns NAME1,NAME2` 只在 table 模式生效，依指定順序只顯示這些欄位；欄位名稱見各命令
  `--help`（不分大小寫）——每個有表格輸出的命令，`--help` 都會列出可用欄位與預設欄位
  （`vcs get` 這類單物件輸出走「欄位: 值」直式排版，一樣遵循 `--columns`）。`-o json` /
  `-o yaml` 大多輸出 API 的原始回應（不受 `--columns`
  影響），適合寫腳本或用 `jq` 進一步處理；但有兩個例外——`config whoami` 本身沒有對應的單一
  API 回應，`-o json/yaml` 輸出的是本地組合出的物件（`api_version`、`profile`、
  `project_code`、`project_id`、`projects`）；`vcs keypair create` 的底層 API 只回傳純文字的
  private key（非 JSON），`-o json/yaml` 輸出的同樣是本地組合出的物件
  （`{"name": "...", "private_key": "..."}`），依賴「原始回應」格式的腳本請對這兩個命令另外處理。
- `--dry-run` 不會送出請求，改印出等價的 `curl` 命令（`x-api-key` 與 `vcs create --password`
  / `--password-stdin` 對應的 `x-extra-property-password` 一律遮蔽為 `***`，其餘自訂 `x-*` header 皆以小寫顯示以
  符合 API 文件慣例）；多步驟命令（例如 `vcs create --wait`
  會依序呼叫建立、輪詢、重新查詢；`vcs rm a b` 對每個目標各呼叫一次刪除）在 `--dry-run` 下
  只會印出**第一個**請求就停止，不會模擬後續步驟。
- `--wait` 讓 `vcs create` 等到實例變成 Ready、`vcs rm` 等到目標被刪除才結束，期間會在
  stderr 印進度（`-o json/yaml` 時自動靜音）；逾時由 `--wait-timeout`（預設 10 分鐘）控制，
  逾時會以 exit code 5 結束。
- `--dry-run` 對 `cos` 底下的命令有兩種語意：`cos key *`（Ceph API，走 apigateway）與其他命令
  一樣印出等價的 `curl` 命令；`cos bucket create/rm`、`cos cp`、`cos rm` 這些**資料面**命令改走
  S3 SDK（不經 apigateway，沒有對應的 `curl` 命令可印），因此改印 `dry-run: <動作>`（例如
  `dry-run: 上傳 ./file → cos://bucket/file`）且不送出任何請求；`--recursive` 時仍會先呼叫
  `ListObjects`（read-only）才能列出將被處理的每一筆檔案／物件。`cos bucket ls`、`cos ls` 是
  純查詢命令，沒有「等價的破壞性操作」可略過，`--dry-run` 下一樣照常執行並回傳結果。

## 命令總覽

以下範例假設已透過 `twaictl config init` 設定好 profile；完整 flag 說明請見對應的
`twaictl <command> --help`。

### `config`

```bash
twaictl config whoami       # 驗證 API key，顯示 API 版本、目前 profile/project、可用 project 清單
```

### `project`

```bash
twaictl project ls                  # 列出可用的 project
twaictl project get GOVxxxxxx       # 依 code 查詢單一 project；也可傳數字 id
twaictl project quota               # 查詢目前 project 的配額（RESOURCE/USAGE/QUOTA）
twaictl project quota --user        # 改列出每位使用者的配額
twaictl project solutions           # 列出目前 project 已啟用的 solution
```

`project quota` 把巢狀的 `{usage, quota}` 物件攤成表格列，`quota` 為負值（`-1`）時顯示為
`unlimited`；`memory` 單位為 MB。`project solutions` 依 solution id 對照 Common `/solutions/`
的名稱。

### `vcs`

```bash
twaictl vcs ls                              # 列出目前 project 下的 VCS 實例
twaictl vcs ls --all                        # 連同其他使用者的實例一併列出（all_users=1）
twaictl vcs get web-01                      # 依名稱查詢；也可傳數字 id
twaictl vcs rm web-01 web-02 -y --wait      # 刪除多個實例，略過確認並等待刪除完成

# 建立實例：--name/--solution/--image/--flavor 必填，--keypair 與 --password 至少擇一
twaictl vcs create --name web-01 --solution "Ubuntu 22.04" \
  --image "Ubuntu 22.04" --flavor v.2xsmall --keypair mykey --eip --wait

# --password-stdin：從 stdin 讀密碼（讀到第一個換行為止），避免密碼出現在
# shell 歷史與 `ps`；與 --password 互斥
echo "s3cret" | twaictl vcs create --name web-02 --solution "Ubuntu 22.04" \
  --image "Ubuntu 22.04" --flavor v.2xsmall --password-stdin

twaictl vcs flavor ls                       # 列出目前 project 可用的 flavor
twaictl vcs flavor ls --all-projects        # 不以 project 過濾，列出所有 flavor
twaictl vcs image ls                        # 列出目前 project 可用的 image
```

`vcs create` 沒有 `--secg`（security group）flag：VCS OpenAPI 的 `POST /sites/` 本來就沒有
對應欄位，因此無法實作。`--eip` 是布林 flag（有指定即配發浮動 IP，對應 API 的
`x-extra-property-floating-ip: floating`；不指定則送 `nofloating`，這也是 API 唯一支援的
兩個值）。`--image`、`--flavor`、`--keypair`、`--network` 的值會原樣放進對應的
`x-extra-property-*` header 送給 API，本工具不做額外轉換或驗證。

`vcs rm <id|name>...` 可一次指定多個目標：依指定順序逐一送出刪除請求，任一失敗即立即停止
（已送出的刪除請求不會回滾）；`--wait` 則是在**全部**刪除請求都送出後，才依序輪詢每個目標
直到刪除完成。`vcs get <id|name>` / `vcs rm <id|name>...` 的名稱解析只會在自己的實例中查詢
（不含 `--all` 才看得到的其他使用者實例）；要對他人的實例操作，請改用數字 id。

### `vcs server`

```bash
twaictl vcs server ls               # 列出目前 project 下的所有 server
twaictl vcs server get 5170218      # 顯示單一 server 詳細資料（只接受數字 id）
```

`GET /sites/`／`GET /sites/{id}/` 回應巢狀的 `servers[]` 不含 server id，因此新增
`vcs server` 提供 server id 的查詢入口；`vcs metrics`、`vcs secg ls`、`vcs volume action`
（attach/detach）、`vcs image save` 需要 server id 時，未指定 `--server` 就會以 site 自動
解析（僅限該 site 剛好只有一台 server 的情況）。真實 API 回應另有 spec 未定義的
`security_groups` 欄位，`vcs server get` 不顯示（見 `docs/api-notes.md`）。

> **已知的 spec 問題**：`ServerSerializer.status` 的 enum 列舉值缺少實測中最常見的 `ACTIVE`
> （且 `SHUTOFF` 重複列了兩次）；因生成型別為一般 `string`（非嚴格 enum），`ACTIVE` 仍可
> 正常解析與顯示，不影響 `vcs server ls/get` 的行為，僅記錄此 spec 疑點供參考。

### `vcs action` / `vcs events` / `vcs metrics`

```bash
twaictl vcs action web-01 stop -y           # 停止（破壞性，需確認或 -y）
twaictl vcs action web-01 start             # 啟動（不需確認）
twaictl vcs action web-01 reboot --wait     # 重開機並等待完成
twaictl vcs events web-01                   # 顯示事件紀錄
twaictl vcs metrics web-01 --meter cpu      # 查詢 CPU 使用率
twaictl vcs metrics web-01 --meter disk-read --begin 2026-08-27T00:00:00Z \
  --end 2026-08-27T12:00:00Z                # 依時間範圍查詢磁碟讀取速率（多一欄 DEVICE）
```

`vcs action <id|name> <start|stop|reboot|suspend|resume|shelve|unshelve>`：`stop`、
`reboot`、`suspend`、`shelve` 屬破壞性操作，需確認或 `-y`；`start`、`resume`、`unshelve`
不需要。`--wait` 依動作輪詢到對應的目標狀態（`start`/`resume`/`unshelve`/`reboot` →
`ACTIVE`；`stop` → `SHUTOFF`；`suspend` → `SUSPENDED`；`shelve` →
`SHELVED_OFFLOADED`/`SHELVED`）；**`reboot --wait` 必須先觀察到 server 離開 `ACTIVE`** 才會
繼續等它回到 `ACTIVE`——若整個重開過程比 `--wait-interval`（預設 5s）還快、沒有輪詢到
「已離開 `ACTIVE`」的瞬間，會持續等到 `--wait-timeout` 逾時（exit 5），此時 reboot 多半
已經完成，只是 `--wait` 沒能觀察到。

`vcs metrics <id|name> --meter cpu|memory|disk-read|disk-write|net-in|net-out
[--begin RFC3339] [--end RFC3339] [--server ID]`：`--begin`/`--end` 請用 RFC3339 格式
（例如 `2026-08-27T12:00:00Z`）；未指定時部分 meter 會回傳全部歷史資料（筆數可能很多）。
`--meter net-in` 目前伺服器端會回 HTTP 500（API 自身的已知問題，非本工具造成，見
`docs/api-notes.md` A22），twaictl 會照實回報錯誤（exit 3）。0 筆資料時 stderr 會印出提示。

### `vcs keypair`

```bash
twaictl vcs keypair ls                              # 列出金鑰對
twaictl vcs keypair get mykey                       # 顯示單一金鑰對詳細資料（含 public key）
twaictl vcs keypair create mykey                    # 由伺服器產生新金鑰對，private key 印到 stdout
twaictl vcs keypair create mykey --out mykey.pem    # private key 寫入檔案，權限自動設為 0600
twaictl vcs keypair create mykey --public-key-file id_rsa.pub  # 匯入既有公鑰（不會回傳 private key）
twaictl vcs keypair rm mykey -y                     # 刪除金鑰對
```

`--out` 不能與 `-o json` / `-o yaml` 併用（會先擋下並回報錯誤）。指定 `--out` 建立新金鑰對
時，工具會在呼叫 API **之前**先以獨占方式建立該檔案（檔案已存在就直接失敗，不會覆寫），
避免 API 呼叫成功後才發現檔案問題而遺失只會回傳一次的 private key；寫入成功後檔案權限固定
為 `0600`。

### `vcs image`

```bash
twaictl vcs image ls                                          # 列出目前 project 可用的 image
twaictl vcs image get "Ubuntu 22.04"                          # 顯示單一 image 詳細資料
twaictl vcs image save web-01 --name web-01-snapshot          # 把實例目前狀態存成新 image
twaictl vcs image rm my-custom-image -y                       # 刪除 image（破壞性操作）
```

`vcs image save <site id|name> --name <n> [--desc] [--os] [--os-version] [--server ID]`
呼叫 `PUT /images/{server_id}/save/`，屬儲存費用操作（尚未於真實環境驗證，見上方已知限制）；
未指定 `--server` 時由 site 自動解析 server id（僅限剛好一台 server 的 site）。

### `vcs network`（spec 標僅 tenant admin 可建立，實測一般使用者亦可）

```bash
twaictl vcs network ls                                        # 列出網路
twaictl vcs network get my-net                                # 顯示單一網路詳細資料
twaictl vcs network create --name my-net --cidr 10.0.0.0/24 --router
twaictl vcs network rm my-net -y                              # 刪除網路（破壞性操作）
```

`vcs network create` 呼叫 `POST /networks/`，spec 描述僅 tenant admin 可用，但真實環境一般
使用者也能成功建立（回 200）；實測建立後數秒網路即轉為 `ERROR` 狀態（API 行為，原因不明，
請向 TWCC 確認）。`vcs network rm` 為非同步，刪除後約 60～120 秒 `status` 為 `DELETING`，
之後查詢回 404。`GET /networks/{id}/` 的成功狀態碼 spec 標成 204 卻附完整 schema，實測回
200（見
`docs/api-notes.md`）。

### `vcs eip`（浮動 IP，建立僅 tenant admin）

```bash
twaictl vcs eip ls                                            # 列出浮動 IP
twaictl vcs eip get 1.2.3.4                                   # 依位址查詢（完全比對）
twaictl vcs eip create                                        # 申請新的浮動 IP
twaictl vcs eip update 1.2.3.4 --desc "for web-01"             # 更新描述
twaictl vcs eip update 1.2.3.4 --desc ""                      # 帶空字串清除描述
twaictl vcs eip rm 1.2.3.4 -y                                 # 刪除浮動 IP（破壞性操作）
```

`vcs eip create`（`POST /ips/`）spec 描述僅 tenant admin 可用；`vcs eip update` 的
`--desc` 必填，帶空字串會清除描述、不帶則報錯；`<id|address>` 以位址完全比對解析。
實測一般使用者即可建立，約 7 秒回應、建立完成即為 `AVAILABLE`（`STATIC` 類型），並立即占用
`static_ip` 配額；`rm` 後配額隨即釋放。計費金額 API 看不到，請用完即釋放。

### `vcs volume`（磁碟）

```bash
twaictl vcs volume ls                                         # 列出 volume
twaictl vcs volume get my-vol                                 # 顯示單一 volume 詳細資料
twaictl vcs volume create --name my-vol --size 10 --type hdd  # 建立 volume（hdd|ssd|LUKS-hdd|LUKS-ssd）
twaictl vcs volume action my-vol attach --server 5170218      # 掛載到指定 server
twaictl vcs volume action my-vol attach --site web-01         # 或用 site 自動解析 server（限單一 server）
twaictl vcs volume action my-vol detach --server 5170218 -y   # 卸載（破壞性操作，需確認）
twaictl vcs volume action my-vol extend --size 20             # 擴充容量
twaictl vcs volume rm my-vol -y                               # 刪除 volume（破壞性操作）
```

`vcs volume action <id|name> attach|detach|extend`：`attach`/`detach` 需要 `--site` 或
`--server` 二擇一（`--server` 必須是正整數，兩者併用會在送出請求前就被拒絕）；`detach`
屬破壞性操作，需確認或 `-y`；`extend` 需要 `--size`（必須大於 0）。`POST /volumes/` 未依
spec 送出 `platform` 欄位（實測不送亦可成功建立）。`attach` 成功後 `vcs volume get` 顯示
`IN-USE` 與掛載點（例如 `/dev/vdb`）；`detach` 成功時 API 不回內容，`-o json` 輸出
`{"volume_id","action"}` 摘要。對 `IN-USE` 的 volume 執行 `rm` 會被 API 拒絕（400）。

### `vcs snapshot`

```bash
twaictl vcs snapshot ls                                       # 列出 snapshot
twaictl vcs snapshot ls --status available                    # 依狀態篩選
twaictl vcs snapshot get my-snap                               # 顯示單一 snapshot 詳細資料
twaictl vcs snapshot create --name my-snap --volume my-vol     # 對指定 volume 建立 snapshot
twaictl vcs snapshot rm my-snap -y                             # 刪除 snapshot（破壞性操作）
```

### `vcs secg`（security group 規則）

```bash
twaictl vcs secg ls web-01                                     # 列出 server 所屬的 security group 規則
twaictl vcs secg ls web-01 --server 5170218                    # 指定 server id（略過 site 自動解析）
twaictl vcs secg add-rule <sg-id> --direction ingress --protocol tcp \
  --port-min 22 --port-max 22 --remote 0.0.0.0/0               # 新增規則
twaictl vcs secg rm-rule <rule-id> -y                          # 刪除規則（破壞性操作）
```

`GET /security_groups/` 需要同時帶 `project` 與 `server`（或 `loadbalancer`）query，因此
`vcs secg ls <id|name>` 內部會先解析出 site 對應的 server id（同 `vcs server`／`vcs metrics`
的規則，未指定 `--server` 時僅限單一 server 的 site）。`add-rule` 呼叫 `PATCH`，真實回應為
201 且無 body（已實測確認），成功後重新查詢並顯示規則清單；`rm-rule` 呼叫 `DELETE` 並帶
`project` query，回 202。注意 `rm-rule` 的參數是 **rule id**，誤傳 security group id 會得到
503「Security group rule … does not exist」。真實環境的 `protocol` 欄位曾出現大寫 `ANY`，spec 未列舉此值，因生成型別為
字串，仍可正常顯示（見 `docs/api-notes.md`）。

### `vcs secret`（TLS 憑證，供 `vcs lb create --tls-secret` 的 `TERMINATED_HTTPS` listener 使用）

```bash
twaictl vcs secret ls                                            # 列出目前 project 的 secret
twaictl vcs secret get my-cert                                    # 依名稱查詢；也可傳數字 id
twaictl vcs secret create --name my-cert --payload-file bundle.p12  # 從檔案讀 payload
twaictl vcs secret create --name my-cert --payload-stdin \
  --expire 2027-01-01T00:00:00Z < bundle.p12                      # 從 stdin 讀 payload
twaictl vcs secret rm my-cert -y                                  # 刪除（破壞性操作）
```

`payload` 必須是 **PKCS#12**（`.p12`／`.pfx`，含憑證與私鑰，可無密碼）bundle 的**原始檔案
內容**；`--payload-file`／`--payload-stdin` 都是讀入原始 bytes 後由 twaictl 自行 base64
編碼送出，**不需要**使用者事先手動 base64。真實環境實測 PEM／DER 格式的憑證會被 API 以
`Invalid certification` 拒絕（見 `docs/api-notes.md` A28）。`--expire` 需為 RFC3339，
省略時 API 回應的 `expire_time` 為 `null`；`id` 為整數。空 payload（空檔案或空 stdin）
在送出請求前就會被拒絕。`--dry-run` 印出的 curl 命令會把 JSON body 的 `payload` 遮蔽為
`***`，不會印出憑證內容。

### `vcs lb`（load balancer）

```bash
twaictl vcs lb ls                                                 # 列出目前 project 的 load balancer
twaictl vcs lb get web-lb                                         # 依名稱查詢；也可傳數字 id
twaictl vcs lb rm web-lb -y --wait                                # 刪除並等待完成（破壞性操作）

# 簡寫模式：單一 pool + 單一 listener（--member 可重複，指定 pool 內的成員）
twaictl vcs lb create --name web-lb --network default_network \
  --member 10.0.1.1:8080 --member 10.0.1.2:8080 --port 80 --wait

# --spec 模式：YAML／JSON 檔案指定完整的 pools/listeners（多 pool／多 listener 需用這種）
cat > lb-spec.yaml <<'EOF'
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
EOF
twaictl vcs lb create --name web-lb --network default_network --spec lb-spec.yaml
twaictl vcs lb update web-lb --spec lb-spec.yaml                  # 更新既有 load balancer 的組態

# action 的三種動作
twaictl vcs lb action web-lb associate-ip --eip 203.0.113.10      # 綁定浮動 IP
twaictl vcs lb action web-lb disassociate-ip                      # 解除綁定
twaictl vcs lb action web-lb update-listener --listener spec-listener \
  --timeout-client-data 60000 --wait                              # 更新 listener 逾時參數並等待完成

# report 的兩種查詢
twaictl vcs lb report                                             # project 層報表（不帶 <id|name>）
twaictl vcs lb report web-lb --member 10.0.1.1                    # 該 LB 的 member 流量報表
```

`create` 支援簡寫 flag（`--member`、`--port`、`--pool-protocol`、
`--listener-protocol`、`--method`、`--monitor-type`、`--tls-secret`）或 `--spec`，兩者
互斥，簡寫 flag 只能表達單一 pool + 單一 listener 的組態；`update` 只接受 `--spec`（整份
取代既有 pools/listeners，非合併，不支援簡寫 flag）。`--tls-secret`（`vcs secret` 的 id
或名稱）只在 `--listener-protocol TERMINATED_HTTPS` 時可用。`action` 對應 spec 的三種動作
（`associateIP`／`disassociateIP`／`updateListenerParams`），皆為 202 非同步，`--wait`
會先確認狀態離開 `ACTIVE` 才視為完成（與 `vcs action reboot` 相同的 mustLeave 語意）。
`report` 不帶 `<id|name>` 查詢 project 層報表（`GET /loadbalancers/reports/`，欄位
ID/NAME/USER/CREATED/DELETED）；帶 `<id|name>` 則改查該 LB 的 member 流量報表（`GET
/loadbalancers/{id}/reports/?member=`，需搭配 `--member`，`--method`/`--interval`/
`--unit` 為選填篩選條件），兩種模式互斥（帶 `<id|name>` 時 `--member` 為必填，不帶時任何
member 專屬 flag 都會報錯）。

真實環境實測（見 `docs/api-notes.md` A29）：狀態機為 `BUILD` → `ACTIVE` → `UPDATING` →
`ACTIVE` → `DELETING` → 404（刪除完成直接 404，未觀察到 `DELETED` 狀態）；`vip` 未指定 IP
時也會自動配置公網 IP；不帶 `project` 欄位也能成功建立 LB。

### `vcs firewall`（firewall 與 firewall rule；spec 描述僅 tenant admin 可用）

```bash
twaictl vcs firewall rule create --name allow-ssh --protocol tcp --action allow \
  --dst-port 22                                                   # 建立規則
twaictl vcs firewall rule ls                                       # 列出規則
twaictl vcs firewall rule get allow-ssh                             # 依名稱查詢；也可傳數字 id
twaictl vcs firewall rule update allow-ssh --action deny            # 只送出有指定的欄位
twaictl vcs firewall rule rm allow-ssh -y                           # 刪除（破壞性操作）

twaictl vcs firewall create --name web-fw --rule allow-ssh          # 建立（--rule／--network 可重複）
twaictl vcs firewall ls                                             # 列出
twaictl vcs firewall get web-fw                                     # 依名稱查詢；也可傳數字 id
twaictl vcs firewall update web-fw --rule allow-ssh \
  --network default_network                                        # 整份取代規則／網路清單
twaictl vcs firewall update web-fw --clear-rules --clear-networks   # 清空規則／網路清單
twaictl vcs firewall rm web-fw -y                                   # 刪除（破壞性操作）
```

spec 描述 firewall／firewall rule 皆僅 **tenant admin** 可用；真實環境 read-only 實測 `ls`
目前回 200 空清單 `[]` 而非 403，寫入型操作（`create`/`update`/`rm`）一般使用者亦可成功執行
（已實測）。`vcs firewall update` 的 `--rule`／
`--network` 是**整份取代**（API 為 `PATCH` 整份取代語意，沒有增量新增／移除），要清空對應
清單需明確指定 `--clear-rules`／`--clear-networks`（與 `--rule`／`--network` 互斥）。
`vcs firewall rule create`／`update` 的 `--protocol`／`--action`／`--src`／`--src-port`／
`--dst`／`--dst-port` 皆選填，未指定的欄位由 API 採預設值；`update` 的 `--src`／`--src-port`／
`--dst`／`--dst-port` 實測**不接受空字串**（API 回 503），無法用空字串清除既有限制，需刪除
該 rule 重建。`GET`／`PATCH /firewall_rules/{id}/` 的 spec 回應形狀標法不一致（`GET` 標陣列、
`PATCH` 標單一物件），實測皆為單一物件；twaictl 以 `twai.DecodeObjectOrFirst` 同時接受兩種
形狀以防日後變動。

### `vcs asp`（auto scaling policy，自動擴縮政策；spec 描述僅 tenant admin 可用）

```bash
twaictl vcs asp create --name web-scale --meter cpu --scale-up 80 \
  --scale-down 20 --max-size 3                                     # 建立
twaictl vcs asp ls                                                  # 列出
twaictl vcs asp get web-scale                                       # 依名稱查詢；也可傳數字 id
twaictl vcs asp rm web-scale -y                                     # 刪除（破壞性操作）

twaictl vcs asp attach web-scale --site web-01 --lb web-lb --port 8080  # 掛到 site（自動解析唯一 server）
twaictl vcs asp attach web-scale --server SERVER_ID                  # 掛到指定 server id（多台 server 的 site 須用這個）
twaictl vcs asp detach --server SERVER_ID -y                         # 移除（破壞性操作，需確認）
```

`--meter` 接受與 `vcs metrics` 相同的短名（左）或 API 名稱（右）：

| 短名 | API 名稱（`meter_name`） |
|---|---|
| `cpu` | `cpu_util` |
| `memory` | `memory.usage` |
| `disk-read` | `disk.read.bytes.rate` |
| `disk-write` | `disk.write.bytes.rate` |
| `net-in` | `network.incoming.bytes.rate` |
| `net-out` | `network.outgoing.bytes.rate` |

（注意：這裡的 API 名稱是 `POST /auto_scaling_policies/` 的 `meter_name` 列舉值，除了
`cpu` 外皆以點號分隔，與 `vcs metrics` 回應內數值鍵名的底線寫法（例如
`network_incoming_bytes_rate`，見 `docs/api-notes.md` A22）是兩套不同的命名，不要混用。）

`vcs asp attach` 的 `--site`／`--server` 二擇一（`--site` 僅在該 VCS 實例只有單一 server
時才能自動解析，多台 server 的實例請改用 `--server`）；指定 `--lb`（load balancer 的 id
或名稱）時，政策觸發擴容而新建的 server 會自動加入該 load balancer（原 server 除外），
`--port` 為新 server 監聽的 port（預設同 load balancer）。**注意：政策觸發擴容時會建立新
的 server，可能產生費用**——一旦擴容門檻（`--scale-up`）被觸發，twaictl 不會二次確認，
請自行評估門檻與實際流量再掛載。`vcs asp detach` 會移除 server 上已掛載的政策，屬破壞性
操作，需確認或 `-y`。ASP 的 create request 欄位叫 `desc`，但 response 欄位叫
`description`，CLI 統一使用 `--desc`（見 `docs/api-notes.md` A34）。

### `cos`（物件儲存）

`cos key *` 呼叫 Ceph API（走 apigateway）；`cos bucket *`、`cos ls`、`cos cp`、`cos rm`
呼叫 S3 相容 API（path-style、SigV4，直接連線 `cos.endpoint`，不經 apigateway）。Ceph 的
project id 與 VCS 的 project id 是各自獨立的編號空間，CLI 會自動另外解析，使用者不需理會。

典型流程：

```bash
twaictl cos key get --save                       # 取得目前 project 的 S3 金鑰並寫入 profile
twaictl cos bucket ls                             # 列出所有 bucket
twaictl cos bucket create my-bucket               # 建立 bucket（--versioning on|off 可選）
twaictl cos cp ./file.txt cos://my-bucket/         # 上傳（目的地以 / 結尾時沿用本地檔名）
twaictl cos ls my-bucket                          # 列出 bucket 內物件（也可用 cos://my-bucket/ 形式）
twaictl cos cp cos://my-bucket/file.txt ./         # 下載到本地目錄
twaictl cos rm cos://my-bucket/file.txt -y         # 刪除物件

twaictl cos sync ./site cos://my-bucket/site --delete       # 只上傳有變更的檔案，並刪除遠端多出的物件
twaictl cos acl cos://my-bucket/site --public --recursive -y # 前綴下所有物件設為 public-read
twaictl cos content-type cos://my-bucket/site/index.html text/html # 修改物件的 Content-Type
twaictl cos usage --per-bucket                    # 逐 bucket 列出用量與物件數
twaictl cos bucket rm my-bucket --purge -y        # 先清除 bucket 內所有物件版本再刪除 bucket
```

- `cos key get/renew` 的 `--show-secret` 只影響 table 模式；**`-o json`/`-o yaml` 一律輸出
  API 原始回應，其中即含 secret key**（無法只遮蔽 JSON 裡的一部分而不破壞結構），因此這兩種
  格式會額外在 stderr 印一次警告（`twaictl: 警告: 輸出含 secret key`），使用時請避免把輸出
  導向 log 或其他系統。`--save` 把 public 金鑰寫入**目前 profile**的 `cos.access_key`/
  `secret_key`；`cos key renew` 是破壞性操作，舊 key 送出請求後立即失效，因此一律先印出／
  儲存新金鑰的輸出，再嘗試寫入設定檔——即使寫入失敗也不會遺失這次唯一顯示的 secret。
- `cos cp` **只支援本地 ↔ COS 之間複製，不支援 cos → cos**（來源與目的地不可同時是
  `cos://` 或同時是本地路徑）。上傳目的地為 `cos://bucket/` 或 `cos://bucket/dir/`（以 `/`
  結尾）時，補上本地檔名；下載目的地是既有目錄或以路徑分隔符結尾時，用 key 的最後一段當
  檔名。`--recursive` 遞迴上傳會略過 symlink；遞迴下載會拒絕 key 內含 `..`／絕對路徑，並在
  寫入前以實體路徑（解析所有 symlink 之後）二次確認不會經由既存 symlink 逃出目的目錄。
  下載先寫入 `<file>.twaictl-part` 暫存檔（以 `O_EXCL` 建立，不會跟隨既有 symlink），成功後
  才 rename 成正式檔名；若程式中斷造成暫存檔殘留，需要手動刪除後才能重新下載。前綴以 `/`
  為界比對，`cos://bucket/dir` 等同 `cos://bucket/dir/`（不會誤命中 `dir2/...`）。
- `cos rm --recursive` 與 `cos cp --recursive` 下載一樣，會先用前綴 `ListObjects` 找出所有
  符合的物件，再逐一呼叫 `DeleteObject`（未使用批次刪除 API，避免額外的 checksum 相容性
  問題）；沒有 `--recursive` 時，每個參數都必須是完整的 key。Phase 3a 曾評估 `DeleteObjects`
  批次刪除，實測無增益（見 `docs/api-notes.md` A31），維持逐筆。
- 上傳超過 16 MiB 會自動改用 multipart（`aws-sdk-go-v2` 的 `feature/s3/transfermanager`）；
  下載目前為單一串流，不分段。`cos cp`／`cos sync` 上傳時會依副檔名（標準庫 `mime` 表）
  自動帶上 Content-Type，未知副檔名為 `application/octet-stream`；之後可用
  `cos content-type` 個別修改。
- `cos` 系列命令的 `--timeout` 是**每一次嘗試**的逾時（S3 client 的 `HTTPClient.Timeout`），
  不是整個呼叫的總時限：SDK 內建重試，S3 client 已明確固定最多重試 3 次，所以一次
  `cos` 呼叫最長可能耗時約 `3 × --timeout` 再加上重試之間的退避時間，例如 `--timeout 1s`
  實際失敗前可能等到約數秒，而不是剛好 1 秒。
- **開啟 versioning 的 bucket**：刪除物件（`cos rm`，不帶 `--all-versions`）只會留下一個
  delete marker，物件本身的舊版本仍保留在 bucket 內；`cos ls <bucket> --versions` 可看到
  所有版本與 delete marker（欄位 Key/Version/Latest/Type/Size/Modified，`Type` 區分
  `object`／`delete marker`）。`cos rm --all-versions` 會刪除該 key（或搭配 `--recursive`
  時整個前綴下）的所有版本與 delete marker。`cos bucket rm` 對非空 bucket（含只剩版本／
  delete marker、S3 API 看起來已無「目前」物件的 bucket）一律回 409 `BucketNotEmpty`，
  錯誤訊息會提示可用 `cos ls --versions` 查看殘留版本、改用 `cos bucket rm --purge` 清除
  （`--purge` 會先逐一刪除 bucket 內所有物件版本與 delete marker，未開 versioning 的
  bucket 一樣適用，版本 id 為 `null`）；確認訊息會列出每個 bucket 的版本數，`--purge`
  搭配 `-o json` 與一般的 `cos bucket rm` 一致，不輸出 JSON 摘要。
- `cos sync <local_dir> cos://bucket[/prefix]` **只支援本地 → COS 單向同步**（不支援
  COS → 本地或 COS → COS）。依序套用四條規則判斷是否需要上傳：(1) 遠端不存在的檔案一律
  上傳；(2) 大小不同則上傳；(3) 大小相同時比對 ETag（不分大小寫）與本地檔案 MD5，不同則
  上傳（僅適用單一 PUT 上傳的物件，ETag 恰為 quoted MD5）；(4) 物件是以 multipart 上傳
  （ETag 非單純 MD5）時改比較檔案的修改時間（mtime），本地較新才上傳。掃描本地目錄時會
  略過 symlink（不上傳、也不視為本地存在的檔案）；因此 `--delete` 會把 symlink 對應的遠端
  物件視為「本地不存在」而一併刪除，若目錄下有 symlink 且啟用 `--delete` 要特別留意。
  `--delete` 會刪除遠端前綴下、本地已不存在的物件，屬破壞性操作，需確認或 `-y`；沒有任何
  檔案需要上傳／刪除時印出「沒有需要同步的檔案」，`-o json` 則輸出 `[]`。
- `cos acl cos://bucket/key... (--public|--private) [--recursive]`：`--public`
  （canned ACL `public-read`，任何人可讀）屬有風險的操作，需確認或 `-y`；`--private`
  不需確認。`--recursive` 以前綴展開套用到所有符合的物件。`cos content-type` 以
  `GetObjectAcl` → `HeadObject` → `CopyObject`（`MetadataDirective=REPLACE`）實作，會
  保留 metadata、系統 header 與公開狀態（僅 `public-read`／`private` 兩種 canned ACL；
  自訂 grant 不保留），但 `LastModified` 會被更新（若該物件之後要被 `cos sync` 以 mtime
  規則比對，時間會改變）；物件超過 5 GiB 時 `cos content-type` 會回報錯誤（不支援
  multipart copy）。
- `cos usage [--per-bucket] [--key-name NAME]` 呼叫 Ceph 的用量管理 API（`GET
  /projects/{id}/buckets_util/`、`--per-bucket` 時改用 `GET /projects/{id}/buckets/`），
  **不需要 S3 金鑰**；預設查詢 public 金鑰底下的用量，`--key-name` 可改查指定的 private
  金鑰。不提供 `--all`（實測對應的 API 參數會造成伺服器端逾時，見 `docs/api-notes.md`
  A17）。`bucket_used_space`／`total_used_space` 已實測確認單位為 bytes（見
  `docs/api-notes.md` Phase 2a 實測 (a)）。查詢 public 金鑰的用量時，API 回應的
  `key_name` 為 `null`，`cos usage` 的 `Key name` 欄位會顯示空白（非錯誤）。
- **已知限制**：目的目錄若在下載過程中被其他程序同時建立/替換（symlink 競態，TOCTOU），
  目前的檢查無法保證完全阻擋。
- **已知限制**：`cos ls`（含 `--versions`）是先把整個 `ListObjects`／`ListObjectVersions`
  分頁結果收集進記憶體，才一次 render 成表格／JSON／YAML，不是邊分頁邊串流輸出；bucket
  物件數很多（實測環境最大達 1,242 萬個）時，要等到最後一頁抓完才會有任何輸出（表格需要
  完整資料才能對齊欄寬，暫不評估改為串流）。`cos rm --recursive`（含 `--all-versions`）、
  `cos bucket rm --purge` 已改為兩段式串流刪除（第一段只計數決定確認訊息，確認後第二段
  邊列出邊立即刪除，不再蒐集完整清單）；純 table 輸出（或未指定 `-o`）時千萬級物件
  （或版本）也不需要額外的記憶體成本，但 `-o json`／`-o yaml` 仍會把每筆刪除結果的摘要
  累積進記憶體，記憶體成本與實際刪除筆數成正比，並非常數。
- **已知限制**：`config init` 的密碼類參數（API key）仍只能走互動輸入或命令列參數
  （argv），沒有 `--password-stdin` 這類選項；`vcs create` 則已提供 `--password-stdin`
  （見上方 `vcs` 一節），可避免密碼出現在 shell 歷史與 `ps`。

### `api`（逃生口）

```bash
twaictl api GET /sites/                                     # 直接呼叫任意 endpoint（預設 --service vcs）
twaictl api POST /sites/ --data '{"name":"demo"}'           # body 為字面 JSON 字串
twaictl api POST /sites/ --data @body.json                  # body 讀自檔案
echo '{"name":"demo"}' | twaictl api POST /sites/ --data @- # body 讀自 stdin
twaictl api GET /usage/ --service cos                       # 呼叫 Ceph（COS）API
twaictl api GET /solutions/ --service common                # 呼叫 Common API（URL 無 host 段）
```

`api` 是繞過所有子命令封裝的逃生口，供尚未支援、或臨時需要呼叫的 endpoint 使用：**不做任何
輸入驗證、不會要求確認、也不做名稱→id 解析**。`<path>` 必須是完整的 API 路徑（query string
直接寫在 path 裡，例如 `/sites/?project=60000`）；寫入類請求（POST/PUT/PATCH/DELETE）的 body
會原封不動送出，請自行確認內容正確再執行——沒有二次確認機制，用錯 path/body 造成的後果需自行
負責。`--data` 只在 POST/PUT/PATCH 有效（GET/DELETE 帶 `--data`，即使值是空字串 `''`，也會
報錯），接受字面 JSON 字串、`@file` 讀檔或 `@-` 讀 stdin 三種形式；帶 `--data` 時預設加上
`Content-Type: application/json`（可用 `--header` 覆寫），明確指定空 body（`--data ''`／
空檔案／空 stdin）也會補上同樣的預設值。`--header k=v` 可重複附加自訂 header（同名 header
重複指定時以最後一次為準，不會附加），但 `x-api-key`／`x-api-host`／`User-Agent` 由 twaictl
依 profile 自動注入，不可透過 `--header` 指定。回應 body 為空時不印出內容，改在 stderr 顯示
真實的狀態碼（例如 `twaictl: HTTP 204 No Content（回應沒有內容）`，200/201/202 等其他狀態碼
搭配空 body 時也會印出各自的真實狀態碼，不會籠統宣稱 204）；`-o table` 等同 `-o json`（原樣
重新縮排回應，非 JSON body 原樣輸出，且結尾若沒有換行會補一個）；`-o yaml` 要求回應是合法
JSON，否則報錯。非 2xx（含 3xx）回應對應 exit code 3；`--dry-run` 印出等價的 curl 命令
（`x-api-key` 遮蔽為 `***`）；`--dry-run` 遮蔽 JSON body 內**任何深度**的 `payload`／
`password` 欄位，非 JSON body 會原樣印出。

**已知行為**：apigateway 對不存在的 path 不一定回 404——實測觀察到會回 302 導向內部登入頁
（`/login?next=/api/v3/<host>/<path>` 這類 URL，見 `docs/api-notes.md` A26）。Go 的
`net/http` client 預設會跟隨重導向，但只在跨網域時才會剝除 `Authorization`/`Cookie`
這類 header，`x-api-key` 不在剝除清單內，等於把 API key 原樣送到重導向目標——因此
`twaictl api`（`RawClient`）**不會**自動跟隨重導向：3xx 回應直接視為 API 錯誤
（**exit code 3**），錯誤訊息含狀態碼與 `Location`（只保留 scheme+host+path，
query string／fragment／userinfo 會被去除，避免重導向目標若夾帶 token 之類的敏感
參數，原樣出現在 stderr／log），方便判斷呼叫的 path／method 本身是否存在，且不會把
憑證送到未知的重導向目標。其他子命令（型別化 client，`internal/twai/client.go`）已於
Phase 3a 比照修正為不跟隨重導向。另外，`--header` 帶入的自訂 header
（例如 `Authorization: Bearer ...`）會以明文出現在 `--dry-run` 輸出的 curl 命令中——
twaictl 只會遮蔽自動注入的 `x-api-key`，不會偵測或遮蔽使用者自行加入的敏感 header，
請避免把機密值放進 `--header` 後又搭配 `--dry-run` 印出。

## Exit code

| code | 意義 |
|---|---|
| 0 | 成功 |
| 1 | 一般錯誤 |
| 2 | 設定錯誤（無 profile、缺 API key） |
| 3 | API 錯誤（HTTP 4xx / 5xx；`twaictl api` 不跟隨重導向，3xx 亦屬此類） |
| 4 | 網路 / 逾時 |
| 5 | `--wait` 逾時 |

## 從 twccli 遷移

已標示 ✅ 的命令已提供；標示「近似」代表 flag／語意與舊版不完全一致，請以
`twaictl <command> --help` 為準。

| twccli | twaictl | 狀態 |
|---|---|---|
| `twccli config init` | `twaictl config init` | ✅ |
| `twccli config whoami` | `twaictl config whoami` | ✅ |
| `twccli ls/mk/rm ccs` | （不提供：CCS 服務於 2026-08-31 終止） | — |
| `twccli info proj` | `twaictl project get <id\|code>` | ✅ |
| `twccli info quota` | `twaictl project quota` | ✅ |
| `twccli ls vcs` | `twaictl vcs ls` | ✅ |
| `twccli mk vcs ...` | `twaictl vcs create ...` | ✅ |
| `twccli rm vcs -s ID` | `twaictl vcs rm ID` | ✅ |
| `twccli ls flavor` | `twaictl vcs flavor ls` | ✅ |
| `twccli ls image` | `twaictl vcs image ls` | ✅ |
| `twccli mk image -s ID` | `twaictl vcs image save ID --name <n>` | 近似 |
| `twccli rm image -id ID` | `twaictl vcs image rm ID` | ✅ |
| `twccli ls key` | `twaictl vcs keypair ls` | ✅ |
| `twccli mk key` | `twaictl vcs keypair create` | ✅ |
| `twccli rm key -n NAME` | `twaictl vcs keypair rm NAME` | ✅ |
| `twccli ch vcs -s ID -sts Stop` | `twaictl vcs action ID stop` | ✅ |
| `twccli ch vcs -s ID -sts Start` | `twaictl vcs action ID start` | ✅ |
| `twccli ch vcs -s ID -sts Reboot` | `twaictl vcs action ID reboot` | ✅ |
| `twccli ls cos` | `twaictl cos bucket ls` | ✅ |
| `twccli mk cos -n BUCKET` | `twaictl cos bucket create BUCKET` | ✅ |
| `twccli cp cos -bkt B -dir DIR -sync` | `twaictl cos sync DIR cos://B/` | ✅ |
| `twccli ls vds` | `twaictl vcs volume ls` | ✅ |
| `twccli mk vds ...` | `twaictl vcs volume create --name .. --size .. --type ..` | 近似 |
| `twccli rm vds -id ID` | `twaictl vcs volume rm ID` | ✅ |
| `twccli ls vlb` | `twaictl vcs lb ls` | ✅ |
| `twccli mk vlb ...` | `twaictl vcs lb create ...`（簡寫 flag 或 `--spec` YAML） | 近似 |
| `twccli rm vlb -id ID` | `twaictl vcs lb rm ID` | ✅ |
| `twccli ls secg` | `twaictl vcs secg ls <id\|name>` | ✅（可用 `--server` 指定 server） |
| `twccli mk secg ...` | `twaictl vcs secg add-rule <sg-id> ...` | 近似 |
| `twccli rm secg -id ID` | `twaictl vcs secg rm-rule ID` | ✅ |
| `twccli ls eip` | `twaictl vcs eip ls` | ✅ |
| `twccli mk eip` | `twaictl vcs eip create` | 近似（僅 tenant admin） |
| `twccli rm eip -id ID` | `twaictl vcs eip rm ID` | ✅ |
| `twccli ls vnet` | `twaictl vcs network ls` | ✅ |
| `twccli mk vnet -n NAME -cidr CIDR` | `twaictl vcs network create --name NAME --cidr CIDR` | 近似（僅 tenant admin） |
| `twccli rm vnet -id ID` | `twaictl vcs network rm ID` | ✅ |
| `twccli ls snap` | `twaictl vcs snapshot ls` | ✅ |
| `twccli mk snap -vds ID` | `twaictl vcs snapshot create --volume ID --name <n>` | 近似 |
| `twccli rm snap -id ID` | `twaictl vcs snapshot rm ID` | ✅ |

## 依據的 API 版本

VCS 1.5.4 / Ceph 1.1.0 / Common 1.2.1（來源見 `api/openapi/SOURCE.md`；`twaictl version` 也會顯示）。

## 開發

```bash
make build      # bin/twaictl
make test
make lint
make gen        # 重新生成 internal/gen/
make docs       # 重新生成 docs/commands.md
make snapshot   # goreleaser 本機 snapshot
```

## 授權

Apache-2.0，見 `LICENSE`。命令語意參考了 [twcc/TWCC-CLI](https://github.com/twcc/TWCC-CLI)（Apache-2.0）的設計，未複製其程式碼。
