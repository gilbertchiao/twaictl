# OpenAPI patches

| Patch | 針對 | 原因 |
|---|---|---|
| `VCS-flavor-gpu-number.patch` | `FlavorSerializer.resource.gpu` | 真實 API（2026-08-27 實測）回傳浮點值（如 `0.25`），spec 誤標 `integer/int64`，改為 `number/double` |
| `VCS-keypair-create-time-string.patch` | `GET /keypairs/{key_name}/` 回應的 `create_time` | 真實 API（2026-08-27 實測）回傳無時區字串（如 `"2023-01-23T16:17:14"`），不符合 spec 標的 `date-time` 格式，改為純 `string` |
| `VCS-quota-usage-object.patch` | `ProjectQuotaSerializer`、`UserQuotaSerializer` 的 `cpu`/`gpu`/`memory`/`ipvs_port`/`floatingip` | 真實 API（2026-08-27 實測）`GET /project_quotas/?project=`、`GET /projects/{id}/user_quotas/` 回應 `cpu`/`gpu`/`memory` 為 `{usage, quota}` 浮點物件（含 `-1` 代表無上限），且沒有 `ipvs_port`/`floatingip`、多了 `floating_ip`/`static_ip`；新增共用 schema `QuotaUsageSerializer` 並改為 `$ref` |
| `VCS-ip-occupied-resource-id-string.patch` | `OccupiedResourceSerializer.id` | 真實 API（2026-08-27 實測）`GET /ips/?project=` 的 `occupied_resource.id` 為字串（如 `"328291"`），spec 誤標 `integer/int64`，改為純 `string` |
| `VCS-volume-type-fields.patch` | `VolumeSerializer` | 真實 API（2026-08-27 實測）`GET /volumes/?project=` 回應多了 spec 未定義的 `volume_type`（如 `"hdd"`）、`volume_uuid` 欄位，新增為選填 `string` |
| `VCS-snapshot-volume-id-integer.patch` | `listSnapShotVolSerializer.id` | 真實 API（2026-08-27 寫入型實測）`POST /snapshots/`、`GET /snapshots/` 回應的 `volume.id` 為數字（volume id），spec 誤標 `string`，改為 `integer/int64` |
| `VCS-secret-id-param-string.patch` | `SecretIDParam.schema.type`、`SecretSerializer.id` | 真實環境（2026-08-28 read-only 實測）`GET /secrets/1/` 回 404、`GET /secrets/<uuid>/` 回 302，而 spec `SecretSerializer.id` 標 `string`、`SecretIDParam` 標 `integer` 自相矛盾；path 參數改為 `string` 以同時容納兩種可能。另外 `SecretSerializer.id` 維持嚴格 `string` 時，若真實 id 是數字，oapi-codegen 生成的 `...WithResponse` 會在 eager unmarshal 階段直接失敗（連原始 body 都拿不到），故放寬為無型別限制（生成 `interface{}`），回應改用手寫 `vcs.Secret` + `FlexibleID` 寬鬆解析。詳見 `docs/api-notes.md` A28 |
| `VCS-lb-pool-members-array.patch` | `PoolDetailSerializer.members` | spec 標為單一 `PoolMemberSerializer`，但 request 端 `PoolData.members` 為 array、欄位名為複數，語意為 pool 內多個 member；改為 array 以免真實回應解析失敗。已於 2026-08-28 Phase 3a 寫入型實測確認正確，見 `docs/api-notes.md` A29 |
| `VCS-secret-create-project-in-body.patch` | `POST /secrets/` requestBody | 真實環境（2026-08-28 寫入型實測）`POST /secrets/?project=<id>` 的 body 缺 `project` 會回 400 `{'project': ['This field is required.']}`；spec 只把 `project` 標在 query，未標在 requestBody，補上必填 `project`（integer）。詳見 `docs/api-notes.md` A28 |
| `VCS-lb-report-user-nested.patch` | `ReportDetailObject.user` | 真實環境（2026-08-28 寫入型實測）`GET /loadbalancers/reports/` 的 `detail.LB[].user` 實際為雙層巢狀 `{"user": {...}}`，spec 只標單層 `UserSerializer`，導致依 spec 解析時 USER 欄位永遠空白；改為巢狀 object。詳見 `docs/api-notes.md` A29 |
| `Ceph-buckets-flat.patch` | `ProjectBucketSerializer`、`ProjectBucketUtilSerializer`、`BucketSerializer.last_modified_time` | 真實 API（2026-08-27 實測）`GET /buckets/`、`GET /buckets_util/` 回應是扁平結構，而非 spec 宣告的 `{public, private}`；`last_modified_time` 的字串格式未確認時區，改為純 `string` |
| `VCS-asp-detail-trailing-slash.patch` | `/auto_scaling_policies/{auto_scaling_policy_id}` path | 真實環境（2026-08-28 read-only 實測）照 spec 的無尾斜線路徑呼叫回 301 導向內部 host，補尾斜線後路由正常（404 not found）；其餘 detail path 皆有尾斜線。詳見 `docs/api-notes.md` A27 |
| `VCS-asp-create-description.patch` | `POST /auto_scaling_policies/` requestBody 的描述欄位 | 真實環境（2026-08-29 寫入型實測）帶 spec 標的 `desc` 會被伺服器靜默忽略（回應 `description: ""`），改帶 `description` 才會生效（回應 `description: "via description"`）；spec 的 request 欄位名本身是筆誤，改為與 response（`AutoScalingPolicySerializer.description`）一致的 `description`。詳見 `docs/api-notes.md` A34 |

`api/openapi/VCS.yaml` 目前已依序套用以上十二個 patch（即 vendor 進本 repo 的版本）；`api/openapi/Ceph.yaml` 已套用 `Ceph-buckets-flat.patch`。`SOURCE.md` 的 SHA-256 分別記錄「原始檔」與「套用 patch 後」兩個雜湊。**本表的排列順序僅為分組閱讀方便，實際套用順序以 `SOURCE.md` 記錄的 patch 清單為準**（新 patch 是否需要排在既有 patch 之前或之後，取決於它們是否修改同一段 yaml，而非依新增時間排列）。詳見 `docs/api-notes.md`「已確認」。

若未來 yaml 有 oapi-codegen 無法處理的錯誤，或實測發現 spec 有誤，請：
1. 不要直接改 vendor 的 yaml「來源」；以 `diff -u` 產生 `<服務>-<簡述>.patch` 放在本目錄。
2. 在 patch 檔頂端以 `#` 註解說明原因與實測日期（`docs/api-notes.md` 為作者私人實測筆記，不隨 repo 公開）。
3. `SOURCE.md` 的更新流程會重新套用這些 patch。
