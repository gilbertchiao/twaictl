# OpenAPI 來源紀錄

| 項目 | 值 |
|---|---|
| Repo | https://github.com/twcc/man-vip |
| Branch | `tws-sync` |
| Commit | `4d87b326a168ce96d345a2846a9240c76a85497e`（2023-09-13T08:32:19Z） |
| 路徑 | `doc-en/openapi/` |
| 同步日期 | 2026-08-26 |

| 檔案 | info.version | SHA-256（原始檔） | SHA-256（套用 patch 後，即本 repo 內容） |
|---|---|---|---|
| VCS.yaml | 1.5.4 | `07a8ba72b9c9f47e9549f62fe53be08f20a6cac9b8439bd44ce3ee8ff6f51166` | `56bd7c4df360f099502ba870a5d426c72ce71edd0a88180494721883d25b764f`（已依序套用 `patches/VCS-flavor-gpu-number.patch`、`patches/VCS-keypair-create-time-string.patch`、`patches/VCS-quota-usage-object.patch`、`patches/VCS-ip-occupied-resource-id-string.patch`、`patches/VCS-volume-type-fields.patch`、`patches/VCS-snapshot-volume-id-integer.patch`、`patches/VCS-secret-id-param-string.patch`、`patches/VCS-lb-pool-members-array.patch`、`patches/VCS-secret-create-project-in-body.patch`、`patches/VCS-lb-report-user-nested.patch`、`patches/VCS-asp-detail-trailing-slash.patch`、`patches/VCS-asp-create-description.patch`，見 `patches/README.md`） |
| Ceph.yaml | 1.1.0 | `6df0e1ade24f95f1bba2cee7eb2fe1586296551d08e7f4416b2bbea007b4d98f` | `f14f597c98f8a3234dd32d0d7632c0525110c4ce2744df5851e2685ab97d6c58`（已套用 `patches/Ceph-buckets-flat.patch`，見 `patches/README.md`） |
| Common.yaml | 1.2.1 | `c9703b143524f3de138114825881b126e3b9987b644835d83c399e044da86c68` | 同原始檔（未套用 patch） |

未 vendor 的檔案：`CCS.yaml`（TWAI 公告 CCS 服務 2026-08-31 終止，本專案不實作）、`Slurm.yaml`／`New_Slurm.yaml`（TWAI 公告 HPC 服務 2026-08-30 終止，本專案不實作，見 `docs/api-notes.md` A37）、`Harbor.yaml`（不實作）。

## 更新流程

1. 下載新版 yaml 覆蓋本目錄。
2. 若有 `patches/`，以 `patch -p1 < patches/<name>.patch` 重新套用。
3. `make gen`，確認 `go build ./...` 通過。
4. 更新本檔的 commit、日期、版本與 SHA-256。
