package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	genvcs "github.com/gilbertchiao/twaictl/internal/gen/vcs"
	"github.com/gilbertchiao/twaictl/internal/twai/vcs"
)

// lbPoolProtocols 是 `--pool-protocol` 的合法值（VCS.yaml PoolData.protocol 的 example）。
var lbPoolProtocols = map[string]bool{"TCP": true, "HTTP": true, "HTTPS": true}

// lbListenerProtocols 是 `--listener-protocol` 的合法值（VCS.yaml ListenerData.protocol 的 example）。
var lbListenerProtocols = map[string]bool{"TCP": true, "HTTP": true, "HTTPS": true, "TERMINATED_HTTPS": true}

// lbMethods 是 `--method` 的合法值（VCS.yaml PoolData.method 的 example）。
var lbMethods = map[string]bool{"ROUND_ROBIN": true, "LEAST_CONNECTIONS": true, "SOURCE_IP": true}

// lbMonitorTypes 是 `--monitor-type` 的合法值（VCS.yaml PoolData.monitor_type 的 example）。
var lbMonitorTypes = map[string]bool{"HTTP": true, "HTTPS": true, "PING": true, "TCP": true}

// lbActionNames 是 `vcs lb action` 第二個位置參數的合法值，依常見操作順序排列
// （不用 map 排序，讓錯誤訊息與 --help 的順序與這裡一致）。
var lbActionNames = []string{"associate-ip", "disassociate-ip", "update-listener"}

// lbActionAPINames 把 lbActionNames 的命令列拼法對應到 API 實際要求的 action 值。
var lbActionAPINames = map[string]string{
	"associate-ip":    string(genvcs.AssociateIP),
	"disassociate-ip": string(genvcs.DisassociateIP),
	"update-listener": string(genvcs.UpdateListenerParams),
}

// lbActionFlagConstraint 描述某個 `vcs lb action` 專屬旗標只能搭配哪個動作使用。
type lbActionFlagConstraint struct {
	flag          string // cobra flag 名稱（不含 "--"）
	allowedAction string // 唯一允許搭配的動作（lbActionNames 其中之一）
}

// lbActionFlagConstraints 列出每個動作專屬旗標的限制：`--eip` 只能搭配 associate-ip，
// `--listener` 與四個 `--timeout-*` 只能搭配 update-listener。在送出請求前逐一檢查，
// 用了不屬於當前動作的旗標就直接拒絕，而不是靜默忽略（旗標說明文字雖然已經寫了
// 「僅 update-listener」之類的限制，但只有文件說明、程式沒有真的擋下來，等於使用者
// 打錯動作＋旗標組合時完全得不到提示）。
var lbActionFlagConstraints = []lbActionFlagConstraint{
	{flag: "eip", allowedAction: "associate-ip"},
	{flag: "listener", allowedAction: "update-listener"},
	{flag: "timeout-client-data", allowedAction: "update-listener"},
	{flag: "timeout-member-connect", allowedAction: "update-listener"},
	{flag: "timeout-member-data", allowedAction: "update-listener"},
	{flag: "timeout-tcp-inspect", allowedAction: "update-listener"},
}

// lbSpecExample 是 `--spec` 檔案格式的說明範例，內容對應
// testdata/fixtures/vcs/lb_spec.yaml，供 `create`／`update` 的 --help 顯示。
const lbSpecExample = `pools:
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
    protocol_port: 8080`

// readSpecFile 讀取 `--spec` 指定的檔案；`@` 前綴可省略（與不加前綴等價，
// 一律視為檔案路徑；不同於 `twaictl api --data` 的 "@file 才讀檔、否則當內嵌字串" 慣例，
// 因為 `--spec` 只接受檔案，沒有內嵌字串的用法）。
func readSpecFile(path string) ([]byte, error) {
	path = strings.TrimPrefix(path, "@")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("讀取 --spec 檔案 %s 失敗: %w", path, err)
	}
	return data, nil
}

// parseLbMember 解析 `--member ip:port[:weight]`（IPv4）或 `[ipv6]:port[:weight]`
// （IPv6，位址須以中括號包住，避免位址本身含有的冒號與 port/weight 的分隔符混淆）：
// ip 以 net.ParseIP 檢查合法性，port 必須介於 1–65535（與 --port 這個 listener port、
// `--spec` 內的 protocol_port／member port 共用 vcs.ValidatePort，同一個概念只有一套
// 標準），weight（選填）必須 ≥ 1。
func parseLbMember(s string) (genvcs.PoolMemberData, error) {
	if strings.HasPrefix(s, "[") {
		return parseLbMemberIPv6(s)
	}
	// 裸 IPv6（未加中括號）會被下面的 strings.Split(s, ":") 切成 4 段以上，
	// 落入「格式錯誤」的通用訊息，使用者不容易看出問題是「IPv6 忘了加中括號」；
	// 這裡先攔截、給出更明確的提示（IPv4 最多只有 ip:port:weight 兩個冒號）。
	if strings.Count(s, ":") > 2 {
		return genvcs.PoolMemberData{}, fmt.Errorf(
			"--member 看起來是未加中括號的 IPv6 位址，IPv6 請用 [ ] 包住位址，"+
				"格式為 [ipv6]:port[:weight]（例如 [2001:db8::1]:80）（目前 %q）", s)
	}
	parts := strings.Split(s, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return genvcs.PoolMemberData{}, fmt.Errorf("--member 格式錯誤，必須是 ip:port[:weight]（目前 %q）", s)
	}
	ipStr := parts[0]
	if net.ParseIP(ipStr) == nil {
		return genvcs.PoolMemberData{}, fmt.Errorf("--member 的 ip 不合法（目前 %q）", s)
	}
	return buildLbMember(ipStr, parts[1:], s)
}

// parseLbMemberIPv6 解析 `[ipv6]:port[:weight]`：中括號內必須是合法的 IPv6 位址
// （net.ParseIP 判定、且 To4() 必須為 nil，排除中括號內誤寫 IPv4 位址的情況），
// 中括號後面必須緊接 `:port[:weight]`；port/weight 的解析與驗證與 IPv4 共用
// buildLbMember。
func parseLbMemberIPv6(s string) (genvcs.PoolMemberData, error) {
	end := strings.Index(s, "]")
	if end < 0 {
		return genvcs.PoolMemberData{}, fmt.Errorf("--member 的 [ 缺少對應的 ]（目前 %q）", s)
	}
	ipStr := s[1:end]
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.To4() != nil {
		return genvcs.PoolMemberData{}, fmt.Errorf("--member 中括號內必須是合法的 IPv6 位址（目前 %q）", s)
	}
	rest := s[end+1:]
	if !strings.HasPrefix(rest, ":") {
		return genvcs.PoolMemberData{}, fmt.Errorf("--member 格式錯誤，] 後面必須接 :port[:weight]（目前 %q）", s)
	}
	parts := strings.Split(rest[1:], ":")
	if len(parts) != 1 && len(parts) != 2 {
		return genvcs.PoolMemberData{}, fmt.Errorf("--member 格式錯誤，必須是 [ipv6]:port[:weight]（目前 %q）", s)
	}
	return buildLbMember(ipStr, parts, s)
}

// buildLbMember 依 ip 與 portWeight（[port] 或 [port, weight]）組出 PoolMemberData；
// port/weight 的解析與驗證邏輯供 parseLbMember（IPv4）／parseLbMemberIPv6 共用，
// original 只用於錯誤訊息中回顯使用者原始輸入的 --member 值。
func buildLbMember(ipStr string, portWeight []string, original string) (genvcs.PoolMemberData, error) {
	port, err := strconv.Atoi(portWeight[0])
	if err != nil {
		return genvcs.PoolMemberData{}, fmt.Errorf("--member 的 port 不是數字（目前 %q）", original)
	}
	if err := vcs.ValidatePort("--member 的 port", int64(port)); err != nil {
		return genvcs.PoolMemberData{}, fmt.Errorf("%w（目前 %q）", err, original)
	}
	portValue := int64(port)
	member := genvcs.PoolMemberData{Ip: &ipStr, Port: &portValue}
	if len(portWeight) == 2 {
		weight, err := strconv.Atoi(portWeight[1])
		if err != nil || weight < 1 {
			return genvcs.PoolMemberData{}, fmt.Errorf("--member 的 weight 必須 ≥ 1（目前 %q）", original)
		}
		weightValue := int64(weight)
		member.Weight = &weightValue
	}
	return member, nil
}

// buildShorthandSpec 依簡寫 flag 組出單一 pool + 單一 listener 的 LoadBalancerSpec：
// pool 名為 `<name>-pool`、listener 名為 `<name>-listener`；monitorType 為空字串代表
// 不設定 health monitor；tlsSecret <= 0 代表不設定 default_tls_container_ref。
func buildShorthandSpec(name, poolProtocol, method string, members []genvcs.PoolMemberData,
	listenerProtocol string, port int32, tlsSecret int32, monitorType string,
) vcs.LoadBalancerSpec {
	poolName := name + "-pool"
	listenerName := name + "-listener"
	pool := genvcs.PoolData{Name: poolName, Protocol: poolProtocol, Method: method}
	if len(members) > 0 {
		pool.Members = &members
	}
	if monitorType != "" {
		mt := monitorType
		pool.MonitorType = &mt
	}
	protocol := listenerProtocol
	listenerPort := port
	listener := genvcs.ListenerData{
		Name:         &listenerName,
		PoolName:     &poolName,
		Protocol:     &protocol,
		ProtocolPort: &listenerPort,
	}
	if tlsSecret > 0 {
		ref := tlsSecret
		listener.DefaultTlsContainerRef = &ref
	}
	return vcs.LoadBalancerSpec{
		Pools:     []genvcs.PoolData{pool},
		Listeners: []genvcs.ListenerData{listener},
	}
}

// newVCSLbCreateCmd 建立 `vcs lb create`：可用簡寫 flag（--member/--port/...）快速建立
// 單一 pool + 單一 listener 的組態，或用 --spec 檔案指定完整的 pools/listeners。
func newVCSLbCreateCmd(a *app) *cobra.Command {
	var (
		name, networkRef, eipRef, desc, specFile string
		members                                  []string
		port                                     int32
		poolProtocol, listenerProtocol, method   string
		monitorType, tlsSecretRef                string
	)
	cmd := &cobra.Command{
		Use:   "create --name <n> --network <id|name> (--member <ip:port[:weight]> --port <port> | --spec <file>)",
		Short: "建立 load balancer",
		Long: "建立 load balancer。\n\n" +
			"簡寫模式：--member（可重複）與 --port 指定單一 pool + 單一 listener 的組態；\n" +
			"--spec 模式：以 YAML 或 JSON 檔案指定完整的 pools/listeners（`@` 前綴可省略），兩者不可併用。\n\n" +
			"--spec 檔案格式範例：\n\n" + lbSpecExample + "\n\n" +
			"未加 --wait 時輸出建立回應（POST）的欄位：\n" + columnsHelp(lbColumns) + "\n\n" +
			"加 --wait 時會等到 Active 後重新查詢，改輸出與 `vcs lb get` 相同的欄位：\n" + columnsHelp(lbDetailColumns),
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			// --wait 時最終渲染用 lbDetailColumns（重新 GetLoadBalancer 之後的型別），
			// 不加 --wait 則是 lbColumns（POST 回應的型別）；兩邊型別不同，--columns
			// 的合法值必須依實際會用到的欄位驗證，否則 --wait 才有的欄位（例如 VIP、
			// Status reason）會被誤判為不存在。
			if a.opts.wait {
				if err := validateColumns(a, lbDetailColumns); err != nil {
					return err
				}
			} else if err := validateColumns(a, lbColumns); err != nil {
				return err
			}
			if err := requireFlag("--name", name); err != nil {
				return err
			}
			if err := requireFlag("--network", networkRef); err != nil {
				return err
			}

			shorthandFlagNames := []string{"member", "port", "pool-protocol", "listener-protocol", "method", "monitor-type", "tls-secret"}
			var shorthandUsed []string
			for _, f := range shorthandFlagNames {
				if c.Flags().Changed(f) {
					shorthandUsed = append(shorthandUsed, "--"+f)
				}
			}
			specGiven := c.Flags().Changed("spec")
			if specGiven && len(shorthandUsed) > 0 {
				return fmt.Errorf("--spec 不可與簡寫 flag（%s）併用", strings.Join(shorthandUsed, ", "))
			}

			// --spec 的讀檔／解析／Validate() 在這裡（任何網路請求之前）就整個做完，
			// 與 `vcs lb update` 一致：檔案不存在或內容打錯，應該在打任何一個 GET/POST
			// 之前就報錯，而不是先解析完 project/network/eip 才發現 --spec 有問題。
			var (
				spec              vcs.LoadBalancerSpec
				parsedMembers     []genvcs.PoolMemberData
				effectiveListener string
				needSecretResolve bool
			)
			if specGiven {
				data, err := readSpecFile(specFile)
				if err != nil {
					return err
				}
				spec, err = vcs.DecodeLoadBalancerSpec(data)
				if err != nil {
					return err
				}
				if err := spec.Validate(); err != nil {
					return err
				}
			} else {
				if len(members) == 0 {
					return fmt.Errorf("請至少指定一個 --member，或改用 --spec 指定完整組態")
				}
				if !c.Flags().Changed("port") {
					return fmt.Errorf("請指定 --port，或改用 --spec 指定完整組態")
				}
				if err := vcs.ValidatePort("--port", int64(port)); err != nil {
					return err
				}
				if err := validateEnumFlag("--pool-protocol", poolProtocol, lbPoolProtocols); err != nil {
					return err
				}
				effectiveListener = listenerProtocol
				if !c.Flags().Changed("listener-protocol") {
					effectiveListener = poolProtocol
				}
				if err := validateEnumFlag("--listener-protocol", effectiveListener, lbListenerProtocols); err != nil {
					return err
				}
				if err := validateEnumFlag("--method", method, lbMethods); err != nil {
					return err
				}
				if err := validateEnumFlag("--monitor-type", monitorType, lbMonitorTypes); err != nil {
					return err
				}
				needSecretResolve = tlsSecretRef != ""
				if needSecretResolve && effectiveListener != "TERMINATED_HTTPS" {
					return fmt.Errorf("--tls-secret 只能搭配 --listener-protocol TERMINATED_HTTPS（目前為 %q）", effectiveListener)
				}
				if effectiveListener == "TERMINATED_HTTPS" && !needSecretResolve {
					return fmt.Errorf("--listener-protocol TERMINATED_HTTPS 需指定 --tls-secret")
				}
				for _, m := range members {
					pm, err := parseLbMember(m)
					if err != nil {
						return err
					}
					parsedMembers = append(parsedMembers, pm)
				}
			}

			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			networkID, err := svc.ResolveNetworkID(ctx, projectID, networkRef)
			if err != nil {
				return err
			}
			var ipID int64
			if eipRef != "" {
				ipID, err = svc.ResolveIPID(ctx, projectID, eipRef)
				if err != nil {
					return err
				}
			}

			if !specGiven {
				var tlsSecret int32
				if needSecretResolve {
					tlsSecret, err = resolveTLSContainerRef(ctx, svc, projectID, tlsSecretRef)
					if err != nil {
						return err
					}
				}
				spec = buildShorthandSpec(name, poolProtocol, method, parsedMembers, effectiveListener, port, tlsSecret, monitorType)
				if err := spec.Validate(); err != nil {
					return err
				}
			}

			res, err := svc.CreateLoadBalancer(ctx, vcs.CreateLoadBalancerInput{
				Name: name, Desc: desc, PrivateNetID: networkID, IPID: ipID, Spec: spec,
			})
			if err != nil {
				return err
			}
			a.progress("已建立 load balancer %d", res.Value.Id)

			if !a.opts.wait {
				return renderOne(a, res.Raw, res.Value, lbColumns)
			}
			waitCtx, cancel := a.waitContext(ctx)
			defer cancel()
			if err := svc.WaitLoadBalancerActive(waitCtx, res.Value.Id, a.opts.waitInterval, func(st string) {
				a.progress("load balancer %d 狀態：%s", res.Value.Id, st)
			}); err != nil {
				return fmt.Errorf("load balancer %d 尚未 Active: %w", res.Value.Id, err)
			}
			fresh, err := svc.GetLoadBalancer(ctx, res.Value.Id)
			if err != nil {
				return err
			}
			return renderOne(a, fresh.Raw, fresh.Value, lbDetailColumns)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "load balancer 名稱（必填）")
	flags.StringVar(&networkRef, "network", "", "private network 的 id 或名稱（必填）")
	flags.StringVar(&eipRef, "eip", "", "要綁定的浮動 IP（id 或位址）")
	flags.StringVar(&desc, "desc", "", "描述")
	flags.StringVar(&specFile, "spec", "", "load balancer spec 檔案（YAML 或 JSON，@ 前綴可省略；與簡寫 flag 互斥）")
	flags.StringArrayVar(&members, "member", nil, "pool member，格式 ip:port[:weight]（IPv6 請用 [addr]:port[:weight]；可重複；簡寫模式必填）")
	flags.Int32Var(&port, "port", 0, "listener 的 protocol port（簡寫模式必填）")
	flags.StringVar(&poolProtocol, "pool-protocol", "HTTP", "pool 的 protocol：TCP|HTTP|HTTPS")
	flags.StringVar(&listenerProtocol, "listener-protocol", "", "listener 的 protocol：TCP|HTTP|HTTPS|TERMINATED_HTTPS（未指定時同 --pool-protocol）")
	flags.StringVar(&method, "method", "ROUND_ROBIN", "pool 的負載平衡演算法：ROUND_ROBIN|LEAST_CONNECTIONS|SOURCE_IP")
	flags.StringVar(&monitorType, "monitor-type", "", "health monitor 類型：HTTP|HTTPS|PING|TCP（不指定則不設定 monitor）")
	flags.StringVar(&tlsSecretRef, "tls-secret", "", "TLS 憑證 secret 的 id 或名稱（僅 --listener-protocol TERMINATED_HTTPS 可用）")
	return cmd
}

// resolveTLSContainerRef 把 `--tls-secret <id|name>` 解析成 int32，供
// ListenerData.DefaultTlsContainerRef 使用（spec 標為 integer）。
func resolveTLSContainerRef(ctx context.Context, svc *vcs.Service, projectID int64, ref string) (int32, error) {
	id, err := svc.ResolveSecretID(ctx, projectID, ref)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("secret id %q 非整數，無法填入 default_tls_container_ref（spec 為 integer），請回報", id)
	}
	return int32(n), nil
}

// newVCSLbUpdateCmd 建立 `vcs lb update`：以 --spec 檔案內容整份取代 pools/listeners。
func newVCSLbUpdateCmd(a *app) *cobra.Command {
	var specFile string
	cmd := &cobra.Command{
		Use:   "update <id|name> --spec <file>",
		Short: "更新 load balancer 的 pools/listeners（整份取代）",
		Long: "更新 load balancer 的 pools/listeners。\n\n" +
			"注意：PATCH 會以 --spec 內容整份取代目前的 pools/listeners，不是合併。\n\n" +
			"--spec 檔案格式範例：\n\n" + lbSpecExample + "\n\n" + columnsHelp(lbDetailColumns),
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := validateColumns(a, lbDetailColumns); err != nil {
				return err
			}
			if err := requireFlag("--spec", specFile); err != nil {
				return err
			}
			data, err := readSpecFile(specFile)
			if err != nil {
				return err
			}
			spec, err := vcs.DecodeLoadBalancerSpec(data)
			if err != nil {
				return err
			}
			if err := spec.Validate(); err != nil {
				return err
			}
			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveLoadBalancerID(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			res, err := svc.UpdateLoadBalancer(ctx, id, spec)
			if err != nil {
				return err
			}
			return renderOne(a, res.Raw, res.Value, lbDetailColumns)
		},
	}
	cmd.Flags().StringVar(&specFile, "spec", "", "load balancer spec 檔案（YAML 或 JSON，@ 前綴可省略；必填）")
	return cmd
}

// lbActionSummary 是 `vcs lb action` 在 -o json/yaml 時輸出的單一物件摘要。
type lbActionSummary struct {
	LoadBalancerID int64  `json:"load_balancer_id"`
	Action         string `json:"action"`
}

// newVCSLbActionCmd 建立 `vcs lb action`：關聯／解除浮動 IP、更新 listener 逾時參數。
func newVCSLbActionCmd(a *app) *cobra.Command {
	var (
		eipRef, listenerName                                                          string
		timeoutClientData, timeoutMemberConnect, timeoutMemberData, timeoutTCPInspect int32
	)
	cmd := &cobra.Command{
		Use:   "action <id|name> <" + strings.Join(lbActionNames, "|") + ">",
		Short: "對 load balancer 送出動作（關聯／解除浮動 IP、更新 listener 逾時參數）",
		Long: "對 load balancer 送出動作：\n" +
			"  associate-ip：關聯浮動 IP（需 --eip）\n" +
			"  disassociate-ip：解除浮動 IP（破壞性操作，需確認或 -y）\n" +
			"  update-listener：更新 listener 逾時參數（需 --listener，四個 --timeout-* 各自選填，\n" +
			"    但至少要指定一個，否則這次動作不會改變任何逾時設定）\n\n" +
			"--eip 只能搭配 associate-ip；--listener 與四個 --timeout-* 只能搭配 update-listener，\n" +
			"用在其他動作會在送出請求前直接報錯。\n\n" +
			"--wait 的限制：動作送出後 load balancer 通常仍是 ACTIVE（更新尚未真的開始），" +
			"因此 --wait 會先等到觀察到 status 離開 ACTIVE、再等它回到 ACTIVE 才算完成；" +
			"若整個更新過程比 --wait-interval（預設 5s）還快、沒有輪詢到「已離開 ACTIVE」的瞬間，" +
			"會持續等到 --wait-timeout 逾時（exit 5），此時實際上動作多半已經完成，只是 --wait 沒能觀察到。",
		Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			action := args[1]
			if strings.TrimSpace(action) == "" {
				return fmt.Errorf("請指定動作（%s）", strings.Join(lbActionNames, ", "))
			}
			if err := validateEnumList("action", action, lbActionNames); err != nil {
				return err
			}
			for _, constraint := range lbActionFlagConstraints {
				if c.Flags().Changed(constraint.flag) && action != constraint.allowedAction {
					return fmt.Errorf("--%s 只能搭配 %s（目前動作為 %q）", constraint.flag, constraint.allowedAction, action)
				}
			}
			switch action {
			case "associate-ip":
				if err := requireFlag("--eip", eipRef); err != nil {
					return err
				}
			case "update-listener":
				if err := requireFlag("--listener", listenerName); err != nil {
					return err
				}
				// 四個 --timeout-* 都選填（各自對應要不要更新該逾時參數），但至少要有
				// 一個：一個都沒帶等於這次動作不會改變任何逾時設定，靜默送出沒有意義，
				// 使用者多半是忘了帶該帶的 --timeout-* flag。
				timeoutFlags := []string{"timeout-client-data", "timeout-member-connect", "timeout-member-data", "timeout-tcp-inspect"}
				timeoutFlagGiven := false
				for _, name := range timeoutFlags {
					if c.Flags().Changed(name) {
						timeoutFlagGiven = true
						break
					}
				}
				if !timeoutFlagGiven {
					return fmt.Errorf("update-listener 至少需指定一個 --timeout-*（%s）", strings.Join(timeoutFlags, ", "))
				}
			}

			ctx := a.commandContext(c)
			svc, projectID, err := a.vcsProject(ctx)
			if err != nil {
				return err
			}
			id, err := svc.ResolveLoadBalancerID(ctx, projectID, args[0])
			if err != nil {
				return err
			}

			if action == "disassociate-ip" {
				if err := a.confirm(fmt.Sprintf("解除 load balancer %d 的浮動 IP", id)); err != nil {
					return err
				}
			}

			in := vcs.LoadBalancerActionInput{Action: lbActionAPINames[action]}
			switch action {
			case "associate-ip":
				ipID, err := svc.ResolveIPID(ctx, projectID, eipRef)
				if err != nil {
					return err
				}
				in.IPID = ipID
			case "update-listener":
				// ListenerName／逾時參數只在 update-listener 帶入 body：上面的
				// lbActionFlagConstraints 檢查已經保證這些 flag 不會在其他動作被使用，
				// 這裡再依 action 明確收斂賦值範圍，避免日後改動順序時不小心又把這些
				// 欄位無條件塞進其他動作的 body。
				in.ListenerName = listenerName
				if c.Flags().Changed("timeout-client-data") {
					in.TimeoutClientData = &timeoutClientData
				}
				if c.Flags().Changed("timeout-member-connect") {
					in.TimeoutMemberConnect = &timeoutMemberConnect
				}
				if c.Flags().Changed("timeout-member-data") {
					in.TimeoutMemberData = &timeoutMemberData
				}
				if c.Flags().Changed("timeout-tcp-inspect") {
					in.TimeoutTCPInspect = &timeoutTCPInspect
				}
			}

			if err := svc.LoadBalancerAction(ctx, id, in); err != nil {
				return err
			}
			a.progress("已送出 %s 請求：load balancer %d", action, id)

			if a.opts.wait {
				waitCtx, cancel := a.waitContext(ctx)
				defer cancel()
				// 比照 vcs action 的 reboot 保護（WaitServerStatus 的 mustLeave）：
				// action 送出後 load balancer 通常仍是 ACTIVE，必須先觀察到離開 ACTIVE
				// 才能判定這次動作真的開始了，否則第一次輪詢就可能誤判為已完成
				// （見 WaitLoadBalancerUpdated 的說明）。`vcs lb create --wait` 是等待
				// 「首次」變成 Active，語意不同，維持 WaitLoadBalancerActive。
				if err := svc.WaitLoadBalancerUpdated(waitCtx, id, a.opts.waitInterval, func(st string) {
					a.progress("load balancer %d 狀態：%s", id, st)
				}); err != nil {
					return fmt.Errorf("load balancer %d 尚未完成 %s: %w", id, action, err)
				}
			}

			return renderSummaryOne(a, lbActionSummary{LoadBalancerID: id, Action: action})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&eipRef, "eip", "", "浮動 IP 的 id 或位址（associate-ip 必填）")
	flags.StringVar(&listenerName, "listener", "", "listener 名稱（update-listener 必填）")
	flags.Int32Var(&timeoutClientData, "timeout-client-data", 0, "listener frontend client 逾時（毫秒，僅 update-listener）")
	flags.Int32Var(&timeoutMemberConnect, "timeout-member-connect", 0, "listener backend member 連線逾時（毫秒，僅 update-listener）")
	flags.Int32Var(&timeoutMemberData, "timeout-member-data", 0, "listener backend member 逾時（毫秒，僅 update-listener）")
	flags.Int32Var(&timeoutTCPInspect, "timeout-tcp-inspect", 0, "TCP content inspection 等待時間（毫秒，僅 update-listener）")
	return cmd
}
