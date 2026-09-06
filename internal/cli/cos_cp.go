package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gilbertchiao/twaictl/internal/output"
	"github.com/gilbertchiao/twaictl/internal/twai/cos"
)

// cpSummary 是 `cos cp` 在 -o json/yaml 時輸出的單筆操作摘要。
type cpSummary struct {
	Src  string `json:"src"`
	Dst  string `json:"dst"`
	Size int64  `json:"size"`
}

// newCosCpCmd 建立 `cos cp`：僅支援本地 ↔ COS 之間複製。
func newCosCpCmd(a *app) *cobra.Command {
	var recursive bool
	cmd := &cobra.Command{
		Use:   "cp <src> <dst>",
		Short: "在本地檔案與 COS 物件之間複製（僅支援本地 ↔ COS）",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			src, dst := args[0], args[1]
			srcRemote, dstRemote := cos.IsRemote(src), cos.IsRemote(dst)
			if srcRemote == dstRemote {
				return fmt.Errorf("phase 1 僅支援本地 ↔ COS 之間複製")
			}
			ctx := a.commandContext(c)
			var summaries []cpSummary
			var err error
			if dstRemote {
				summaries, err = a.cosCpUpload(ctx, src, dst, recursive)
			} else {
				summaries, err = a.cosCpDownload(ctx, src, dst, recursive)
			}
			if err != nil {
				return err
			}
			if a.opts.dryRun {
				return nil
			}
			return renderSummary(a, summaries)
		},
	}
	cmd.Flags().BoolVar(&recursive, "recursive", false, "遞迴複製整個目錄／前綴")
	return cmd
}

// cosCpUpload 處理本地 → COS 的複製：src 為單一檔案或（--recursive 時）目錄。
//
// src 若是指向目錄的 symlink，交給 cosCpUploadDir 走訪的「根路徑」會先解析成實際路徑：
// filepath.WalkDir 對「根」路徑是用 Lstat 判斷型別（不會跟隨 symlink），若直接把 src
// 這個 symlink 傳給它，WalkDir 會把它當成 symlink 型別的單一項目、既不視為目錄也不
// 遞迴走訪，導致遞迴上傳靜默上傳 0 個檔案、exit 0（使用者可能誤以為真的上傳完成）。
// 只有「src 本身是 symlink 且目標是目錄」才解析，且只影響傳給 WalkDir 的根路徑；
// 目錄樹內部的 symlink 仍照舊略過（見 cosCpUploadDir 的說明），避免符號連結迴圈。
//
// src 若是指向一般檔案的 symlink（單檔上傳分支），刻意不解析：key 推導
// （filepath.Base(src)）與檔案內容讀取都必須沿用使用者輸入的原始路徑（即 symlink
// 本身的檔名），而不是它指向的實體檔名，否則 `cp link.txt cos://b/dir/` 這種情境會
// 意外把 key 換成 `real.txt`，改變使用者原本預期的 key 推導語意。
func (a *app) cosCpUpload(ctx context.Context, src, dstURL string, recursive bool) ([]cpSummary, error) {
	loc, ok := cos.ParseURL(dstURL)
	if !ok {
		return nil, fmt.Errorf("%s 不是合法的 cos:// URL", dstURL)
	}
	info, err := os.Stat(src)
	if err != nil {
		return nil, fmt.Errorf("讀取 %s 失敗: %w", src, err)
	}
	if info.IsDir() {
		if !recursive {
			return nil, fmt.Errorf("%s 是目錄，請加 --recursive", src)
		}
		walkRoot := src
		if fi, lerr := os.Lstat(src); lerr == nil && fi.Mode()&fs.ModeSymlink != 0 {
			resolved, everr := filepath.EvalSymlinks(src)
			if everr != nil {
				return nil, fmt.Errorf("解析 symlink %s 失敗: %w", src, everr)
			}
			walkRoot = resolved
		}
		return a.cosCpUploadDir(ctx, walkRoot, loc)
	}

	key := loc.Key
	if key == "" || strings.HasSuffix(key, "/") {
		key += filepath.Base(src)
	}
	dst := cos.Location{Bucket: loc.Bucket, Key: key}.String()
	if a.opts.dryRun {
		_, _ = fmt.Fprintf(a.stdout, "dry-run: 上傳 %s → %s\n", src, dst)
		return nil, nil
	}
	client, err := a.s3Client()
	if err != nil {
		return nil, err
	}
	if err := client.Upload(ctx, loc.Bucket, key, src); err != nil {
		return nil, err
	}
	a.progress("上傳 %s → %s（%s）", src, dst, output.Bytes(info.Size()))
	return []cpSummary{{Src: src, Dst: dst, Size: info.Size()}}, nil
}

// cosUploadFile 是 walkLocalFiles 走訪目錄時蒐集到的單一檔案。
type cosUploadFile struct {
	path    string
	key     string
	size    int64
	modTime time.Time
}

// walkLocalFiles 遞迴走訪 srcDir，回傳每個一般檔案對應的上傳項目：key = prefix + 以 `/`
// 分隔的相對路徑（供 Windows 呼叫端也能得到一致的 key）；略過目錄與 symlink（略過 symlink
// 時印出進度提示）。`cos cp --recursive` 與 `cos sync` 共用。
func (a *app) walkLocalFiles(srcDir, prefix string) ([]cosUploadFile, error) {
	var files []cosUploadFile
	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			a.progress("略過 symlink %s", path)
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("計算 %s 相對路徑失敗: %w", path, err)
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("讀取 %s 中繼資料失敗: %w", path, err)
		}
		files = append(files, cosUploadFile{
			path: path, key: prefix + filepath.ToSlash(rel),
			size: info.Size(), modTime: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("走訪目錄 %s 失敗: %w", srcDir, err)
	}
	return files, nil
}

// cosCpUploadDir 遞迴走訪本地目錄，把每個檔案上傳到 loc.Key 正規化後（見 remotePrefix）
// 為前綴的 key。目錄底下一個可上傳的檔案都沒有時（例如空目錄，或整個目錄底下只有
// symlink），印出提示而不是靜默成功，避免使用者誤以為真的上傳了東西。
func (a *app) cosCpUploadDir(ctx context.Context, srcDir string, loc cos.Location) ([]cpSummary, error) {
	prefix := remotePrefix(loc.Key)

	files, err := a.walkLocalFiles(srcDir, prefix)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		a.progress("沒有可上傳的檔案")
		return nil, nil
	}

	if a.opts.dryRun {
		for _, f := range files {
			dst := cos.Location{Bucket: loc.Bucket, Key: f.key}.String()
			_, _ = fmt.Fprintf(a.stdout, "dry-run: 上傳 %s → %s\n", f.path, dst)
		}
		return nil, nil
	}

	client, err := a.s3Client()
	if err != nil {
		return nil, err
	}
	summaries := make([]cpSummary, 0, len(files))
	for _, f := range files {
		if err := client.Upload(ctx, loc.Bucket, f.key, f.path); err != nil {
			return nil, err
		}
		dst := cos.Location{Bucket: loc.Bucket, Key: f.key}.String()
		a.progress("上傳 %s → %s（%s）", f.path, dst, output.Bytes(f.size))
		summaries = append(summaries, cpSummary{Src: f.path, Dst: dst, Size: f.size})
	}
	return summaries, nil
}

// isDirLikeDst 判斷 cp 下載目的地是否應視為目錄（存在且是目錄，或以路徑分隔符結尾）。
func isDirLikeDst(dst string) bool {
	if strings.HasSuffix(dst, "/") || strings.HasSuffix(dst, string(os.PathSeparator)) {
		return true
	}
	info, err := os.Stat(dst)
	return err == nil && info.IsDir()
}

// lastKeySegment 回傳 key 以 `/` 分隔的最後一段（不含分隔符本身）。
func lastKeySegment(key string) string {
	if idx := strings.LastIndex(key, "/"); idx >= 0 {
		return key[idx+1:]
	}
	return key
}

// safeLocalPath 把以 `/` 分隔的 S3 key 相對路徑 rel 安全地接到本地目錄 dst 之下。
// rel 的來源是遠端 key（TrimPrefix 之後的剩餘部分，或 key 的最後一段），不可信任：
// 拒絕絕對路徑、Windows volume name，以及任何 `.`／`..`／空 路徑段，避免惡意或異常的
// key（例如 "dir/../../evil.txt"）讓下載結果逃出 dst 之外。
func safeLocalPath(dst, rel string) (string, error) {
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("路徑會逃出 %s", dst)
		}
	}
	local := filepath.FromSlash(rel)
	if filepath.IsAbs(local) || filepath.VolumeName(local) != "" {
		return "", fmt.Errorf("路徑會逃出 %s", dst)
	}
	joined := filepath.Join(dst, local)
	relCheck, err := filepath.Rel(dst, joined)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("路徑會逃出 %s", dst)
	}
	return joined, nil
}

// ensureUnderDir 在實際寫入前，用「解析所有 symlink 之後的實體路徑」確認 local 真的落在
// dst 之下。safeLocalPath 只做字面路徑檢查（拒絕 `..` 之類的片段），但如果 dst 目錄樹裡
// 已經存在 symlink（例如 dst/sub -> /outside），字面上「乾淨」的相對路徑仍可能經由該
// symlink 實際寫到 dst 之外；另外若 local 本身已經是既存的 symlink，直接寫入會覆寫
// symlink 指向的目標而不是建立新檔案，同樣拒絕。
//
// dst 與 local 的父目錄若尚未存在會先建立（Download 本來就需要這些目錄存在，這裡先建立
// 純粹是為了能對它們呼叫 EvalSymlinks；只在真正要下載時才呼叫此函式，dry-run 不受影響）。
func ensureUnderDir(dst, local string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("建立目錄 %s 失敗: %w", dst, err)
	}
	dstReal, err := filepath.EvalSymlinks(dst)
	if err != nil {
		return fmt.Errorf("解析 %s 的實體路徑失敗: %w", dst, err)
	}

	// 在呼叫 MkdirAll(parent) 之前，先確認 parent「目前已存在」的最深祖先目錄沒有
	// 逃出 dst：如果先建目錄再檢查，key 若有多層（例如 "sub/nested/evil.txt"）且
	// dst/sub 是指向外部的 symlink，會在偵測到逃出之前就已經在外部建立了
	// "outside/nested" 這個空目錄——即使最後仍然拒絕下載，也留下了不必要的副作用。
	parent := filepath.Dir(local)
	if err := verifyUnderRealDir(dst, dstReal, parent); err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("建立目錄 %s 失敗: %w", parent, err)
	}
	// 建立後再驗證一次（雙重檢查）：MkdirAll 只會建立原本不存在的目錄，不可能建出
	// symlink，理論上这次一定會通過；保留這一步是為了在往後的程式碼變動不小心
	// 弱化了建立前的檢查時，仍有第二道防線。
	if err := verifyUnderRealDir(dst, dstReal, parent); err != nil {
		return err
	}

	if info, err := os.Lstat(local); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s 已存在且是 symlink，拒絕覆寫", local)
	}
	return nil
}

// deepestExistingAncestor 從 path 往上找，回傳第一個確實存在（os.Lstat 不出錯）的路徑。
// EvalSymlinks 對不存在的路徑會直接出錯，而且必須在呼叫 MkdirAll 建立任何目錄之前，
// 先只針對「已經存在」的部分確認沒有逃出 dst，才能安全地建立其餘尚不存在的目錄。
func deepestExistingAncestor(path string) (string, error) {
	for {
		if _, err := os.Lstat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("檢查 %s 失敗: %w", path, err)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path, nil // 已經到根目錄
		}
		path = parent
	}
}

// verifyUnderRealDir 確認 dir（可能尚未完全存在）目前已存在的最深祖先目錄，其實體路徑
// （解析所有 symlink 之後）仍落在 dstReal（dst 的實體路徑）之下。
func verifyUnderRealDir(dst, dstReal, dir string) error {
	existing, err := deepestExistingAncestor(dir)
	if err != nil {
		return err
	}
	existingReal, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return fmt.Errorf("解析 %s 的實體路徑失敗: %w", existing, err)
	}
	rel, err := filepath.Rel(dstReal, existingReal)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("目標目錄經 symlink 指向 %s 之外", dst)
	}
	return nil
}

// cosCpDownload 處理 COS → 本地的複製：srcURL 為單一物件 key 或（--recursive 時）前綴。
func (a *app) cosCpDownload(ctx context.Context, srcURL, dst string, recursive bool) ([]cpSummary, error) {
	loc, ok := cos.ParseURL(srcURL)
	if !ok {
		return nil, fmt.Errorf("%s 不是合法的 cos:// URL", srcURL)
	}
	if recursive {
		return a.cosCpDownloadPrefix(ctx, loc, dst)
	}
	if loc.Key == "" || strings.HasSuffix(loc.Key, "/") {
		return nil, fmt.Errorf("%s 不是完整的 key，請加 --recursive", srcURL)
	}

	target := dst
	// 只有「目的地是目錄、檔名取自 key」這個分支的 target 是從遠端 key 衍生出來的，
	// 才需要 safeLocalPath／ensureUnderDir 的保護；使用者自己指定的完整檔名不受此限。
	derivedFromKey := isDirLikeDst(dst)
	if derivedFromKey {
		local, err := safeLocalPath(dst, lastKeySegment(loc.Key))
		if err != nil {
			return nil, fmt.Errorf("拒絕下載 %s：%w", loc.String(), err)
		}
		target = local
	}

	if a.opts.dryRun {
		_, _ = fmt.Fprintf(a.stdout, "dry-run: 下載 %s → %s\n", loc.String(), target)
		return nil, nil
	}
	if derivedFromKey {
		if err := ensureUnderDir(dst, target); err != nil {
			return nil, fmt.Errorf("拒絕下載 %s：%w", loc.String(), err)
		}
	}
	client, err := a.s3Client()
	if err != nil {
		return nil, err
	}
	if err := client.Download(ctx, loc.Bucket, loc.Key, target); err != nil {
		return nil, err
	}
	var size int64
	if info, statErr := os.Stat(target); statErr == nil {
		size = info.Size()
	}
	a.progress("下載 %s → %s（%s）", loc.String(), target, output.Bytes(size))
	return []cpSummary{{Src: loc.String(), Dst: target, Size: size}}, nil
}

// cosDownloadItem 是 cosCpDownloadPrefix 列出待下載物件時蒐集到的單筆資料。
type cosDownloadItem struct {
	key  string
	size int64
}

// cosCpDownloadPrefix 以 loc.Key 正規化後（見 remotePrefix）的前綴列出物件並逐一下載到
// dst 目錄；跳過以 `/` 結尾的「目錄標記」物件。即使是 --dry-run，仍會先呼叫 ListObjects
// （read-only、不送出任何刪除／上傳／下載請求）才知道要列出哪些將被處理的檔案。
// 正規化前綴避免 "dir" 誤把 "directory/other.txt" 也列進來；每個 key 去掉前綴後的
// 相對路徑都會經 safeLocalPath 做字面檢查，避免 "dir/../../evil.txt" 這類 key 讓下載
// 結果逃出 dst 之外；真正下載前再經 ensureUnderDir 用實體路徑（解析 symlink 之後）
// 二次確認，防止 dst 樹內已存在的 symlink 讓字面上「乾淨」的路徑實際寫到 dst 之外
// （發現逃出時整個命令中止，該筆之前已下載成功的檔案不會被回復）。
func (a *app) cosCpDownloadPrefix(ctx context.Context, loc cos.Location, dst string) ([]cpSummary, error) {
	client, err := a.s3Client()
	if err != nil {
		return nil, err
	}

	prefix := remotePrefix(loc.Key)
	var items []cosDownloadItem
	err = client.ListObjects(ctx, loc.Bucket, prefix, func(o cos.Object) error {
		if strings.HasSuffix(o.Key, "/") {
			return nil
		}
		items = append(items, cosDownloadItem{key: o.Key, size: o.Size})
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		a.progress("沒有符合的物件")
		return nil, nil
	}

	localPathFor := func(key string) (string, error) {
		rel := strings.TrimPrefix(key, prefix)
		local, err := safeLocalPath(dst, rel)
		if err != nil {
			return "", fmt.Errorf("拒絕下載 cos://%s/%s：%w", loc.Bucket, key, err)
		}
		return local, nil
	}

	if a.opts.dryRun {
		for _, it := range items {
			local, err := localPathFor(it.key)
			if err != nil {
				return nil, err
			}
			_, _ = fmt.Fprintf(a.stdout, "dry-run: 下載 cos://%s/%s → %s\n", loc.Bucket, it.key, local)
		}
		return nil, nil
	}

	summaries := make([]cpSummary, 0, len(items))
	for _, it := range items {
		local, err := localPathFor(it.key)
		if err != nil {
			return nil, err
		}
		if err := ensureUnderDir(dst, local); err != nil {
			return nil, fmt.Errorf("拒絕下載 cos://%s/%s：%w", loc.Bucket, it.key, err)
		}
		if err := client.Download(ctx, loc.Bucket, it.key, local); err != nil {
			return nil, err
		}
		src := cos.Location{Bucket: loc.Bucket, Key: it.key}.String()
		a.progress("下載 %s → %s（%s）", src, local, output.Bytes(it.size))
		summaries = append(summaries, cpSummary{Src: src, Dst: local, Size: it.size})
	}
	return summaries, nil
}
