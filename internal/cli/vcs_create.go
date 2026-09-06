package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// vcsVolumeTypes 是 `--volume-type` 的合法值（VCS.yaml PostSitesParamsXExtraPropertyVolumeType）。
var vcsVolumeTypes = map[string]bool{
	"hdd":      true,
	"ssd":      true,
	"LUKS-hdd": true,
	"LUKS-ssd": true,
}

// vcsSystemVolumeTypes 是 `--system-volume-type` 的合法值（VCS.yaml PostSitesParamsXExtraPropertySystemVolumeType）。
var vcsSystemVolumeTypes = map[string]bool{
	"local_disk":        true,
	"block_storage-hdd": true,
}

// validateEnumFlag 確認 value 為 allowed 的其中之一；value 為空字串代表使用者未指定，視為合法（不送對應 header）。
// 錯誤訊息列出排序後的可用值，避免每次執行順序不一致。
func validateEnumFlag(flagName, value string, allowed map[string]bool) error {
	if value == "" || allowed[value] {
		return nil
	}
	values := make([]string, 0, len(allowed))
	for v := range allowed {
		values = append(values, v)
	}
	sort.Strings(values)
	return fmt.Errorf("%s 的值必須是下列其中之一：%s（目前為 %q）", flagName, strings.Join(values, ", "), value)
}

// validateEnumList 確認 value 為 allowed 的其中之一；value 為空字串代表使用者未指定，視為合法。
// 與 validateEnumFlag 的差異在於 allowed 是（依語意排序的）slice 而非 map，
// 讓 `vcs action`、`vcs metrics --meter` 這類「順序本身有意義」的合法值列表，
// 錯誤訊息能照原順序列出，而不是像 validateEnumFlag 那樣先排序成字母序。
func validateEnumList(flagName, value string, allowed []string) error {
	if value == "" {
		return nil
	}
	for _, v := range allowed {
		if v == value {
			return nil
		}
	}
	return fmt.Errorf("%s 的值必須是下列其中之一：%s（目前為 %q）", flagName, strings.Join(allowed, ", "), value)
}

// requireFlag 確認必填字串 flag 已指定非空值（去除前後空白後）；
// 不使用 cobra 的 MarkFlagRequired，因為它的錯誤訊息是英文
// （`required flag(s) "name" not set`），與本專案「使用者訊息一律繁體中文」的慣例不符，
// 且 MarkFlagRequired 只檢查 flag 是否「有被設定」，無法擋下 `--name ""` 這種空字串。
func requireFlag(flagName, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("請指定 %s", flagName)
	}
	return nil
}

// requireNonNegative 確認容量類 flag 不是負數；0 代表使用者未指定（CreateSite 只在 > 0 時
// 送出對應 header），負數則代表使用者刻意指定了不合理的值，不能被靜默忽略後送出與需求不符的請求。
func requireNonNegative(flagName string, value int) error {
	if value < 0 {
		return fmt.Errorf("%s 不可為負數（目前 %d）", flagName, value)
	}
	return nil
}

// parseServerFlag 解析 --server：空字串代表未指定（回傳 0，呼叫端改由 site 自動解析）；
// 其餘必須是正整數，且必須在送出任何 API 請求之前呼叫。`vcs metrics`、`vcs secg ls`、
// `vcs image save`、`vcs volume action` 共用這個 helper，確保「非數字」與「非正整數」
// 兩種錯誤情境下的訊息一致。
func parseServerFlag(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("--server 必須為數字 id（目前 %q）: %w", value, err)
	}
	if id <= 0 {
		return 0, fmt.Errorf("--server 必須是正整數（目前 %d）", id)
	}
	return id, nil
}

// readPasswordFromStdin 從 a.stdin 讀一行做為 --password-stdin 的密碼值：讀到第一個
// 換行字元或 EOF 為止，只去除尾端的 "\r\n" 或 "\n"（不做 TrimSpace），避免使用者刻意在
// 密碼前後留空白時被意外截斷。空字串（只有換行或完全沒輸入）視為錯誤，避免使用者
// 誤用 --password-stdin 卻忘了接管線，導致建立出沒有密碼、之後無法登入的實例。
func readPasswordFromStdin(a *app) (string, error) {
	reader := bufio.NewReader(a.stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("讀取 stdin 密碼失敗: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return "", fmt.Errorf("stdin 沒有密碼")
	}
	return line, nil
}

// newVCSCreateCmd 建立 `vcs create`。
//
// --password（或 --password-stdin 從 stdin 讀到的值）只會放進 API 請求的 header
// （由 vcs.Service.CreateSite 轉交生成 client 設定），絕不寫進 progress / log / error
// 訊息；dry-run 模式下印出的 curl 命令會把 x-extra-property-password 與 x-api-key
// 一併遮蔽為 ***（見 internal/twai/curl.go 的 maskedHeaders），避免密碼明文出現在常被
// 貼進 issue / log 的 dry-run 輸出中。
func newVCSCreateCmd(a *app) *cobra.Command {
	var (
		name, solutionRef, image, flavor     string
		keypair, password, network, az, desc string
		passwordStdin, eip                   bool
		volumeSize, systemVolumeSize         int
		volumeType, systemVolumeType         string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "建立 VCS 實例",
		Long:  "建立 VCS 實例。\n\n" + columnsHelp(siteDetailColumns),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validateColumns(a, siteDetailColumns); err != nil {
				return err
			}
			for _, req := range []struct {
				flagName, value string
			}{
				{"--name", name},
				{"--solution", solutionRef},
				{"--image", image},
				{"--flavor", flavor},
			} {
				if err := requireFlag(req.flagName, req.value); err != nil {
					return err
				}
			}
			if err := requireNonNegative("--volume-size", volumeSize); err != nil {
				return err
			}
			if err := requireNonNegative("--system-volume-size", systemVolumeSize); err != nil {
				return err
			}
			if passwordStdin {
				// 用 Changed 而非 password != "" 判斷 --password 是否「有被指定」：
				// 使用者可能刻意打 --password ""，此時 password 仍是空字串，
				// 若只看值會誤判成「沒指定 --password」而放行，讓互斥檢查失效。
				if c.Flags().Changed("password") {
					return fmt.Errorf("--password 與 --password-stdin 不可同時使用")
				}
				stdinPassword, err := readPasswordFromStdin(a)
				if err != nil {
					return err
				}
				password = stdinPassword
			}
			if keypair == "" && password == "" {
				return fmt.Errorf("請指定 --keypair 或 --password（或 --password-stdin）")
			}
			if err := validateEnumFlag("--volume-type", volumeType, vcsVolumeTypes); err != nil {
				return err
			}
			if err := validateEnumFlag("--system-volume-type", systemVolumeType, vcsSystemVolumeTypes); err != nil {
				return err
			}

			ctx := a.commandContext(c)
			svc, err := a.vcsService()
			if err != nil {
				return err
			}
			projectID, err := a.projectID(ctx)
			if err != nil {
				return err
			}
			solutionID, err := svc.ResolveSolutionID(ctx, projectID, solutionRef)
			if err != nil {
				return err
			}

			res, err := svc.CreateSite(ctx, vcs.CreateSiteInput{
				Name:             name,
				Desc:             desc,
				ProjectID:        projectID,
				SolutionID:       solutionID,
				Image:            image,
				Flavor:           flavor,
				Keypair:          keypair,
				Password:         password,
				PrivateNetwork:   network,
				FloatingIP:       eip,
				VolumeSize:       volumeSize,
				SystemVolumeSize: systemVolumeSize,
				VolumeType:       volumeType,
				SystemVolumeType: systemVolumeType,
				AvailabilityZone: az,
			})
			if err != nil {
				return err
			}

			if !a.opts.wait {
				return renderOne(a, res.Raw, res.Value, siteDetailColumns)
			}

			id := res.Value.Id
			a.progress("已建立 VCS 實例 %d，等待 Ready…", id)
			waitCtx, cancel := a.waitContext(ctx)
			defer cancel()
			if err := svc.WaitSiteReady(waitCtx, id, a.opts.waitInterval, func(st string) {
				a.progress("VCS 實例 %d 狀態：%s", id, st)
			}); err != nil {
				// 實例已經建立（且正在計費），逾時或進入 Error 狀態時仍要讓使用者知道是哪個 id，
				// 尤其 -o json/yaml 會讓上面的 progress 靜音，此時這行錯誤訊息是唯一線索。
				return fmt.Errorf("VCS 實例 %d 尚未 Ready: %w", id, err)
			}
			// WaitSiteReady 只回傳輪詢期間看到的 status，重新 GetSite 才能取得完整的最新資料
			// （public IP、servers 等在 Ready 後才會填入）。
			fresh, err := svc.GetSite(ctx, id)
			if err != nil {
				return err
			}
			return renderOne(a, fresh.Raw, fresh.Value, siteDetailColumns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "VCS 實例名稱（必填）")
	flags.StringVar(&solutionRef, "solution", "", "solution 的 id 或名稱（必填）")
	flags.StringVar(&image, "image", "", "image（必填，原樣傳遞給 API）")
	flags.StringVar(&flavor, "flavor", "", "flavor（必填，原樣傳遞給 API）")
	flags.StringVar(&keypair, "keypair", "", "SSH 金鑰對名稱（與 --password 至少擇一）")
	flags.StringVar(&password, "password", "", "登入密碼（與 --keypair 至少擇一；僅用於 API header，不會出現在任何輸出）")
	flags.BoolVar(&passwordStdin, "password-stdin", false,
		"從 stdin 讀取登入密碼（讀到第一個換行為止），避免密碼出現在 shell 歷史與 `ps`；與 --password 互斥")
	flags.StringVar(&network, "network", "", "private network 名稱")
	flags.BoolVar(&eip, "eip", false, "配發浮動 IP（floating IP）")
	flags.IntVar(&volumeSize, "volume-size", 0, "資料磁碟大小（GB）")
	flags.StringVar(&volumeType, "volume-type", "", "資料磁碟類型：hdd|ssd|LUKS-hdd|LUKS-ssd")
	flags.IntVar(&systemVolumeSize, "system-volume-size", 0, "系統磁碟大小（GB）")
	flags.StringVar(&systemVolumeType, "system-volume-type", "", "系統磁碟類型：local_disk|block_storage-hdd（flavor 的 DISK 為 0 時必填，並搭配 --system-volume-size；缺少時 API 回 500 'system_volume_type'）")
	flags.StringVar(&az, "az", "", "可用區域（availability zone）")
	flags.StringVar(&desc, "desc", "", "描述")
	// --name/--solution/--image/--flavor 為必填：故意不用 cmd.MarkFlagRequired，
	// 改在 RunE 一開始手動檢查（見 requireFlag），以便回傳繁體中文錯誤訊息並擋下空字串。
	return cmd
}
