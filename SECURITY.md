# 安全性政策（Security Policy）

## 回報漏洞

請**不要**在公開的 Issue 或 Pull Request 中揭露安全性問題。

請改用 GitHub 的 **Private vulnerability reporting**：到本 repo 的
[Security 分頁](https://github.com/gilbertchiao/twaictl/security/advisories/new) 建立 advisory，
內容只有維護者看得到。回報時請盡量附上：

- 受影響的版本（`twaictl version` 的輸出）
- 重現步驟或 proof of concept
- 影響範圍（例如 API key 外洩、非預期的資源刪除）

維護者會在 **7 天內**回覆確認，並視嚴重程度在合理時間內修復與發布新版本。
修復發布後會在 Release notes 與 advisory 中公開致謝（除非你不希望被提及）。

## 支援的版本

本專案只維護**最新的 minor 版本**（例如目前為 `0.3.x`），安全性修正會以新的 patch 版本發布，
不回溯到更舊的版本。請盡量使用最新版。

## 本工具處理的敏感資料

`twaictl` 會接觸以下憑證，設計上皆**不寫入 log、錯誤訊息或 `--dry-run` 輸出**（以 `***` 遮蔽）：

- 台智雲 API key（環境變數 `TWAI_API_KEY` 或設定檔 `api_key`）
- COS（S3 相容）的 access key／secret key
- `vcs create --password` 的登入密碼、`vcs secret create` 的憑證內容

設定檔（預設 `~/.config/twaictl/config.yaml`）以 `0600` 權限寫入。
若你發現任何情境下上述資料會出現在輸出、log 或網路傳輸以外的地方，請視為安全性問題回報。

## 範圍外

以下不屬於本專案的安全性問題，請直接向對應單位回報：

- 台智雲 TWCC 平台或其 API 本身的漏洞（本工具為非官方 community 工具，與台智雲無隸屬關係）
- 第三方相依套件的漏洞（會由 Dependabot 自動追蹤，也歡迎開 Issue 提醒）
