// Package types 定义 phosche 的共享领域类型，用于各包之间的数据传递。
package types

// JobStatus 表示照片的处理状态，定义以下状态机转换：
//
//	                  ┌─────────────────────────────────┐
//	                  │                                 │
//	                  ▼                                 │
//	  ┌──────────┐  ┌──────────┐  ┌────────────────┐  ┌────────┐  │
//	  │unanalyzed│─▶│ analyzing│──▶pending_analysis│──┘        │  │
//	  └──────────┘  └──────────┘  └────────────────┘           │  │
//	                    │                                       │  │
//	                    │  ┌──────────┐                         │  │
//	                    └─▶│ analyzed │                         │  │
//	                       └──────────┘   (其他错误)            │  │
//	                             ▲            │                  │  │
//	                             │            ▼                  │  │
//	                             │      ┌──────────┐             │  │
//	                             └──────│  failed  │             │  │
//	                                    └──────────┘             │  │
//	                                                             │  │
//	                                                             └──┘
//
//   - StatusUnanalyzed：未分析，照片刚被发现尚未处理
//   - StatusAnalyzing：分析中，正在执行 AI 分析
//   - StatusAnalyzed：已分析，分析成功且结果已索引到 ES
//   - StatusFailed：失败，遇到不可恢复的错误（如图片损坏、格式不支持）
//   - StatusPendingAnalysis：等待重试，LLM 连接失败后进入重试队列
type JobStatus string

const (
	StatusUnanalyzed      JobStatus = "unanalyzed"
	StatusAnalyzing       JobStatus = "analyzing"
	StatusAnalyzed        JobStatus = "analyzed"
	StatusFailed          JobStatus = "failed"
	StatusPendingAnalysis JobStatus = "pending_analysis"
)

// FileOp 表示文件系统操作类型。
//   - OpCreate：文件创建操作
//   - OpModify：文件修改操作
//   - OpDelete：文件删除操作
type FileOp string

const (
	OpCreate FileOp = "create" // 文件创建事件
	OpModify FileOp = "modify" // 文件修改事件
	OpDelete FileOp = "delete" // 文件删除事件
)

// FileEvent 表示照片文件系统事件。
//   - Path：发生变化的文件路径
//   - Op：文件操作类型（create/modify/delete）
//   - Timestamp：事件发生时间的 Unix 时间戳（秒级）
//   - MTime：文件的修改时间（mtime）
//   - Size：文件大小（字节）
type FileEvent struct {
	Path      string `json:"path"`
	Op        FileOp `json:"op"`
	Timestamp int64  `json:"timestamp"`
	MTime     int64  `json:"mtime"`
	Size      int64  `json:"size"`
}

// EXIFInfo 存储从照片中提取的 EXIF 元数据。
// es 标签表示该字段在 Elasticsearch 中的映射类型：
//   - date：日期类型
//   - keyword：关键词类型（精确匹配）
//   - integer：整型
//   - double：双精度浮点型
type EXIFInfo struct {
	DateTimeOriginal string  `json:"date_time_original,omitempty" es:"date"`   // 拍摄时间，RFC3339 格式
	CameraModel      string  `json:"camera_model,omitempty" es:"keyword"`      // 相机型号
	LensModel        string  `json:"lens_model,omitempty" es:"keyword"`        // 镜头型号
	FocalLength      string  `json:"focal_length,omitempty" es:"keyword"`      // 焦距，如 "6.9mm"
	Aperture         string  `json:"aperture,omitempty" es:"keyword"`          // 光圈值，如 "f/1.8"
	ISO              int     `json:"iso,omitempty" es:"integer"`               // ISO 感光度
	GPSLat           float64 `json:"gps_lat,omitempty" es:"double"`            // GPS 纬度（十进制度数）
	GPSLon           float64 `json:"gps_lon,omitempty" es:"double"`            // GPS 经度（十进制度数）
}

// ColorInfo 表示一个分析出的颜色，包含中文名称和 CSS hex 值。
type ColorInfo struct {
	Name string `json:"name" es:"keyword"` // 中文颜色名，如 "蓝色"
	Hex  string `json:"hex" es:"keyword"`  // CSS hex 值，如 "#3B82F6"
}

// AnalysisResult 是 LLM 对照片分析后返回的结构化 JSON 分析结果。
//   - Description：图片的自然语言描述
//   - Tags：标签列表
//   - Objects：检测到的物体列表
//   - SceneType：场景类型（indoor/outdoor/underwater/aerial/studio/night/unknown）
//   - Colors：主要颜色列表（含 name 和 hex）
//   - PeopleCount：照片中的人数
//   - HasText：照片中是否包含文字
//   - Text：从照片中提取的文字内容
//   - Confidence：分析结果的置信度（0-1）
type AnalysisResult struct {
	Description string      `json:"description" es:"text"`            // 图片描述
	Tags        []string    `json:"tags" es:"text"`                   // 标签
	Objects     []string    `json:"objects" es:"text"`                // 检测物体
	SceneType   string      `json:"scene_type" es:"keyword"`          // 场景类型
	Colors      []ColorInfo `json:"colors" es:"nested"`               // 主要颜色
	PeopleCount int         `json:"people_count" es:"integer"`        // 人数
	HasText     bool        `json:"has_text" es:"boolean"`            // 是否有文字
	Text        string      `json:"text" es:"text"`                   // 提取的文字
	Confidence  float64     `json:"confidence,omitempty" es:"double"` // 置信度
}

// Photo 表示系统中的一个照片实体。
//   - ID：照片唯一标识，由文件路径经 SHA-256 哈希生成
//   - Path：照片在文件系统中的路径
//   - MTime：文件的修改时间（Unix 时间戳）
//   - Size：文件大小（字节）
//   - Status：当前处理状态
//   - AnalyzedAt：AI 分析完成时间（Unix 时间戳，指针类型，为空表示尚未分析）
//   - EXIF：从照片中提取的 EXIF 元数据
//   - CreatedAt：记录创建时间（Unix 时间戳）
type Photo struct {
	ID         string    `json:"id"`
	Path       string    `json:"path"`
	MTime      int64     `json:"mtime"`
	Size       int64     `json:"size"`
	Status     JobStatus `json:"status"`
	AnalyzedAt *int64    `json:"analyzed_at,omitempty"`
	EXIF       *EXIFInfo `json:"exif,omitempty"`
	CreatedAt  int64     `json:"created_at"`
	Email      string    `json:"email,omitempty" es:"keyword"`
}

// GeoInfo 存储照片的逆地理编码信息（由 GPS 坐标通过 Amap API 获取）。
type GeoInfo struct {
	Country          string `json:"country,omitempty" es:"keyword"`
	Province         string `json:"province,omitempty" es:"keyword"`
	City             string `json:"city,omitempty" es:"keyword"`
	District         string `json:"district,omitempty" es:"keyword"`
	Address          string `json:"address,omitempty" es:"text"`
	FormattedAddress string `json:"formatted_address,omitempty" es:"text"`
}

// PhotoDocument 将 Photo（照片元数据）与 AnalysisResult（AI 分析结果）、GeoInfo（逆地理编码信息）组合为扁平化文档，用于 Elasticsearch 索引。
// 通过内嵌结构体，所有字段在 ES 中处于同一层级。
type PhotoDocument struct {
	Photo
	AnalysisResult
	GeoInfo
}

// SearchRequest 是照片搜索请求参数，支持全文搜索、多条件过滤和分页。
//   - Query：全文搜索关键词，匹配描述、标签和物体
//   - DateFrom/DateTo：按拍摄日期范围过滤（格式 YYYY-MM-DD）
//   - Tags/Objects：按标签和物体列表过滤
//   - SceneType：按场景类型过滤
//   - CameraModel：按相机型号过滤
//   - Status：按处理状态过滤
//   - Page/PageSize：分页参数（从 1 开始）
type SearchRequest struct {
	Query       string   `json:"query,omitempty"`
	DateFrom    string   `json:"date_from,omitempty"`
	DateTo      string   `json:"date_to,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Objects     []string `json:"objects,omitempty"`
	SceneType   string   `json:"scene_type,omitempty"`
	CameraModel string   `json:"camera_model,omitempty"`
	Status      string   `json:"status,omitempty"`
	Page        int      `json:"page"`
	PageSize    int      `json:"page_size"`
}

// SearchResponse 是照片搜索结果的响应结构。
//   - Hits：命中的照片文档列表
//   - Total：匹配的总记录数
//   - Page：当前页码
//   - PageSize：每页数量
//   - TotalPages：总页数（根据 Total 和 PageSize 计算）
type SearchResponse struct {
	Hits       []PhotoDocument `json:"hits"`
	Total      int64           `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

// StatsResponse 包含照片库的聚合统计信息。
//   - Total：照片总数
//   - ByStatus：按处理状态（unanalyzed/analyzing/analyzed/failed/pending_analysis）统计的数量分布
//   - RecentCount：最近 1 小时内新增的照片数量
type StatsResponse struct {
	Total       int64              `json:"total"`
	ByStatus    map[JobStatus]int64 `json:"by_status"`
	RecentCount int64              `json:"recent_count"`
}

// FiltersResponse 列出搜索筛选 UI 中可用的筛选选项。
//   - Tags：所有可用的标签列表
//   - SceneTypes：所有可用的场景类型
//   - Cameras：所有可用的相机型号
type FiltersResponse struct {
	Tags       []string `json:"tags"`
	SceneTypes []string `json:"scene_types"`
	Cameras    []string `json:"cameras"`
}
