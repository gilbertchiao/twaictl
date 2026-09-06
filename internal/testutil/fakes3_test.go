package testutil

import (
	"encoding/xml"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listVersionsResult 是 ListObjectVersions 回應的最小 XML 解析結構，只取測試需要的欄位。
type listVersionsResult struct {
	XMLName xml.Name `xml:"ListVersionsResult"`
	Keys    []string `xml:"Version>Key"`
}

// getObjectVersionsKeys 直接以 net/http 呼叫假 S3 server 的 `GET /{bucket}?versions...`，
// 回傳依序出現的 <Version><Key> 清單（不經過 internal/twai/cos.S3 這層 SDK 包裝，
// 用意是驗證 fake server 本身對 query string 的處理，不受 SDK 呼叫慣例侷限）。
func getObjectVersionsKeys(t *testing.T, baseURL, path string) []string {
	t.Helper()
	resp, err := http.Get(baseURL + path) //nolint:noctx,gosec // 測試輔助函式，url 為測試固定值
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	var result listVersionsResult
	require.NoError(t, xml.Unmarshal(body, &result))
	return result.Keys
}

// TestListObjectVersionsKeyMarkerWithoutVersionIDSkipsAllVersionsOfThatKey 驗證 S3 的
// ListObjectVersions 語意：只帶 key-marker、不帶 version-id-marker 時，代表「這個 key
// 已經看完了」，回應必須略過該 key 的「全部」版本，從下一個 key 開始；
// 不能像帶了 version-id-marker 那樣去比對某個特定版本再往後接續
// （那種比對方式在沒有 version-id-marker 時找不到任何符合的版本，會誤判為
// 「完全沒有東西可以跳過」，等於 key-marker 沒有生效）。
func TestListObjectVersionsKeyMarkerWithoutVersionIDSkipsAllVersionsOfThatKey(t *testing.T) {
	f := NewFakeS3(t)
	f.SetVersioning("a-bucket", "Enabled")
	f.PutObject("a-bucket", "a", []byte("1"))
	f.PutObject("a-bucket", "a", []byte("2")) // key "a" 有兩個版本
	f.PutObject("a-bucket", "b", []byte("x"))
	f.PutObject("a-bucket", "c", []byte("y"))

	keys := getObjectVersionsKeys(t, f.URL(), "/a-bucket?versions&key-marker=a")
	assert.Equal(t, []string{"b", "c"}, keys, "key-marker=a 且沒有 version-id-marker 時，key a 的全部版本都要略過")
}

// TestListObjectVersionsKeyMarkerWithVersionIDResumesAfterThatVersion 驗證帶
// version-id-marker 時維持既有行為：從該 (key, versionID) 之後接續，
// 同一個 key 尚未列完的版本仍會出現。
func TestListObjectVersionsKeyMarkerWithVersionIDResumesAfterThatVersion(t *testing.T) {
	f := NewFakeS3(t)
	f.SetVersioning("a-bucket", "Enabled")
	f.PutObject("a-bucket", "a", []byte("1"))
	f.PutObject("a-bucket", "a", []byte("2"))
	f.PutObject("a-bucket", "b", []byte("x"))

	versions := f.Versions("a-bucket", "a")
	require.Len(t, versions, 2)
	// Versions() 依「舊 → 新」排列，但 ListObjectVersions 回應內同一個 key 是「新 → 舊」；
	// 用最新的一筆（Versions() 回傳的最後一筆）當 version-id-marker，之後應該接續
	// 同一個 key 剩下較舊的版本、再接 "b"。
	newest := versions[len(versions)-1].VersionID

	keys := getObjectVersionsKeys(t, f.URL(), "/a-bucket?versions&key-marker=a&version-id-marker="+newest)
	assert.Equal(t, []string{"a", "b"}, keys, "帶 version-id-marker 時應從該版本之後接續，同一個 key 剩下的版本仍會出現")
}

// TestLiveObjectEntriesSnapshotIsIndependentOfLaterWrites 是 8(b) 真正要鑑別的性質：
// liveObjectEntries 快照下來的 entries 必須是獨立的值複製（mod/etag/size 皆為值型別），
// 不能是「之後被渲染時才重新查一次 bucket 目前狀態」的鍵清單。呼叫一次快照、之後才寫入
// 修改同一個 key 的內容，斷言已經拿到手的 entries 完全不受影響——這正是舊寫法
// （鎖一次列出 key、解鎖，稍後渲染內容時才又鎖一次讀取）辦不到的：舊寫法在兩次持鎖
// 之間讀到的 key 清單與後來讀到的內容可能來自不同時間點，同一個 HTTP 回應因此可能
// 混雜兩個快照的資料；改成「一次持鎖內完成快照」之後，這個測試斷言的獨立性才會成立。
// （這裡直接呼叫套件內部函式而非透過 HTTP，因為要驗證的是快照本身的獨立性，不需要、
// 也無法單靠並發時序去穩定重現「回應混雜兩個快照」這個原始 bug；下面的
// TestFakeS3ConcurrentListAndPutDoesNotRace 則是重構後「不會有新資料競爭」的另一種保障，
// 兩者互補但驗證的不是同一件事，見該測試的說明。）
func TestLiveObjectEntriesSnapshotIsIndependentOfLaterWrites(t *testing.T) {
	f := NewFakeS3(t)
	f.PutObject("b", "k", []byte("v1"))

	entries, ok := f.liveObjectEntries("b")
	require.True(t, ok)
	require.Len(t, entries, 1)
	snapshotEtag := entries[0].etag
	snapshotMod := entries[0].mod
	snapshotSize := entries[0].size

	// 快照「之後」才發生的寫入：內容變了，etag／mod／size 理論上都會跟著變。
	f.PutObject("b", "k", []byte("v2-with-different-length"))

	assert.Equal(t, snapshotEtag, entries[0].etag, "快照後的寫入不可以回頭改動已經回傳的 entries")
	assert.Equal(t, snapshotMod, entries[0].mod)
	assert.Equal(t, snapshotSize, entries[0].size)

	// 佐證：bucket 目前的實際狀態確實已經變了（排除「其實內容根本沒變、測試沒測到東西」
	// 這種偽陽性）。
	newEntries, ok := f.liveObjectEntries("b")
	require.True(t, ok)
	require.Len(t, newEntries, 1)
	assert.NotEqual(t, snapshotEtag, newEntries[0].etag, "重新快照應該要看到最新內容的 etag")
}

// TestAllVersionEntriesSnapshotIsIndependentOfLaterWrites 是
// TestLiveObjectEntriesSnapshotIsIndependentOfLaterWrites 的 allVersionEntries
// （ListObjectVersions 用）版本，理由相同。
func TestAllVersionEntriesSnapshotIsIndependentOfLaterWrites(t *testing.T) {
	f := NewFakeS3(t)
	f.SetVersioning("b", "Enabled")
	f.PutObject("b", "k", []byte("v1"))

	entries, ok := f.allVersionEntries("b")
	require.True(t, ok)
	require.Len(t, entries, 1)
	snapshotEtag := entries[0].etag
	snapshotVersionID := entries[0].versionID

	f.PutObject("b", "k", []byte("v2-with-different-length")) // 開了 versioning，這會是新的一個版本

	require.Len(t, entries, 1, "快照後新增版本不可以讓已經回傳的 entries 切片跟著變長")
	assert.Equal(t, snapshotEtag, entries[0].etag, "快照後的寫入不可以回頭改動已經回傳的 entries")
	assert.Equal(t, snapshotVersionID, entries[0].versionID)

	newEntries, ok := f.allVersionEntries("b")
	require.True(t, ok)
	assert.Len(t, newEntries, 2, "重新快照應該要看到新增的版本")
}

// maxConcurrentRaceSpinIterations 是 TestFakeS3ConcurrentListAndPutDoesNotRace 兩個
// spin goroutine 的保底上限：正常情況下 `stop` channel 關閉時就會提前結束，這個上限
// 純粹是防呆（避免日後改動意外讓某個 goroutine 卡在無界迴圈，拖垮測試時間）。
const maxConcurrentRaceSpinIterations = 2_000_000

// TestFakeS3ConcurrentListAndPutDoesNotRace 是重構後的資料競爭保護網：並發呼叫
// ListObjectsV2／ListObjects(V1)／ListObjectVersions／ListBuckets／GetBucketVersioning
// 與並發 PutObject／PutBucketVersioning，搭配 `go test -race` 確認「快照＋渲染收在
// 同一次鎖內」的新寫法（以及 getBucketVersioning 在鎖內讀取 versioning 欄位）沒有
// 引入新的資料競爭。
//
// 注意：這個測試不能、也不是用來鑑別 8(b) 修的那個問題——修改前的寫法（鎖一次列出
// key、解鎖，稍後渲染內容時才又鎖一次讀取）本身每一次記憶體存取都還是在鎖的保護之下，
// 在 `-race` 底下本來就不會被判定為資料競爭；它是「兩次持鎖之間可能讀到不一致的快照」
// 這種邏輯上的一致性問題，不是資料競爭。真正鑑別這個問題的是上面兩個
// TestLiveObjectEntriesSnapshotIsIndependentOfLaterWrites／
// TestAllVersionEntriesSnapshotIsIndependentOfLaterWrites。
//
// `?versioning` 與並發 SetVersioning 則是真正的資料競爭鑑別測試（Minor 6）：
// getBucketVersioning 先前解鎖後才讀 b.versioning，這裡打的 GET /vb?versioning
// 若與下面的 SetVersioning 撞在一起，`-race` 應該要能抓到（修正前會抓到，修正後不會）。
//
// versioning 的競爭刻意用獨立的 bucket `vb`（PutObject 那個 goroutine 只碰 `b`），
// 不能共用同一個 bucket：早期版本讓兩個 goroutine 都打 `b`，SetVersioning 在
// Enabled/Suspended 之間快速切換會讓同一個 key 的 PutObject 在 Enabled 期間每次都
// 疊加一個新版本（而非覆蓋），tight loop 幾秒內就能讓版本數暴增到數十萬筆，
// 之後每次 `?versions` GET 都要渲染全部版本，整個測試從數秒拖慢到約 47 秒、
// 傳輸近 190 MB。`b` 維持完全不開版本控制（PutObject 覆蓋同一 key 是 O(1)），
// `vb` 沒有任何 PutObject 打進來（SetVersioning 對「有沒有物件」無感，純粹只是
// 一個 flag），兩者互不干擾，測試恢復到 `-race` 下數秒等級。
func TestFakeS3ConcurrentListAndPutDoesNotRace(t *testing.T) {
	f := NewFakeS3(t)
	f.PutBucket("b")
	f.PutBucket("vb")

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < maxConcurrentRaceSpinIterations; i++ {
			select {
			case <-stop:
				return
			default:
				f.PutObject("b", "k", []byte{byte(i)})
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < maxConcurrentRaceSpinIterations; i++ {
			select {
			case <-stop:
				return
			default:
				status := "Enabled"
				if i%2 == 1 {
					status = "Suspended"
				}
				f.SetVersioning("vb", status)
			}
		}
	}()

	get := func(path string) {
		resp, err := http.Get(f.URL() + path) //nolint:noctx,gosec // 測試輔助
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	for i := 0; i < 200; i++ {
		get("/b")
		get("/b?list-type=2")
		get("/b?versions")
		get("/vb?versioning")
		get("/")
	}
	close(stop)
	wg.Wait()
}
