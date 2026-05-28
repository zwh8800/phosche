package types

// JobStatus represents the processing status of a photo.
type JobStatus string

const (
	StatusUnanalyzed      JobStatus = "unanalyzed"
	StatusAnalyzing       JobStatus = "analyzing"
	StatusAnalyzed        JobStatus = "analyzed"
	StatusFailed          JobStatus = "failed"
	StatusPendingAnalysis JobStatus = "pending_analysis"
)

// FileOp represents the type of file operation.
type FileOp string

const (
	OpCreate FileOp = "create"
	OpModify FileOp = "modify"
	OpDelete FileOp = "delete"
)

// FileEvent represents a filesystem event for a photo.
type FileEvent struct {
	Path      string `json:"path"`
	Op        FileOp `json:"op"`
	Timestamp int64  `json:"timestamp"`
	MTime     int64  `json:"mtime"`
	Size      int64  `json:"size"`
}

// EXIFInfo holds parsed EXIF metadata from a photo.
type EXIFInfo struct {
	DateTimeOriginal string  `json:"date_time_original,omitempty" es:"date"`
	CameraModel      string  `json:"camera_model,omitempty" es:"keyword"`
	LensModel        string  `json:"lens_model,omitempty" es:"keyword"`
	FocalLength      string  `json:"focal_length,omitempty" es:"keyword"`
	Aperture         string  `json:"aperture,omitempty" es:"keyword"`
	ISO              int     `json:"iso,omitempty" es:"integer"`
	GPSLat           float64 `json:"gps_lat,omitempty" es:"double"`
	GPSLon           float64 `json:"gps_lon,omitempty" es:"double"`
}

// AnalysisResult holds AI analysis results for a photo.
type AnalysisResult struct {
	Description string  `json:"description" es:"text"`
	Tags        []string `json:"tags" es:"text"`
	Objects     []string `json:"objects" es:"text"`
	SceneType   string  `json:"scene_type" es:"keyword"`
	Colors      []string `json:"colors" es:"keyword"`
	PeopleCount int     `json:"people_count" es:"integer"`
	HasText     bool    `json:"has_text" es:"boolean"`
	Confidence  float64 `json:"confidence,omitempty" es:"double"`
}

// Photo represents a photo in the system.
type Photo struct {
	ID         string    `json:"id"`
	Path       string    `json:"path"`
	MTime      int64     `json:"mtime"`
	Size       int64     `json:"size"`
	Status     JobStatus `json:"status"`
	AnalyzedAt *int64    `json:"analyzed_at,omitempty"`
	EXIF       *EXIFInfo `json:"exif,omitempty"`
	CreatedAt  int64     `json:"created_at"`
}

// PhotoDocument combines Photo metadata with its AnalysisResult for search indexing.
type PhotoDocument struct {
	Photo
	AnalysisResult
}

// SearchRequest represents a photo search query.
type SearchRequest struct {
	Query       string   `json:"query,omitempty"`
	DateFrom    string   `json:"date_from,omitempty"`
	DateTo      string   `json:"date_to,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Objects     []string `json:"objects,omitempty"`
	SceneType   string   `json:"scene_type,omitempty"`
	CameraModel string   `json:"camera_model,omitempty"`
	Page        int      `json:"page"`
	PageSize    int      `json:"page_size"`
}

// SearchResponse is the response for a photo search.
type SearchResponse struct {
	Hits       []PhotoDocument `json:"hits"`
	Total      int64           `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

// StatsResponse contains aggregate statistics about the photo library.
type StatsResponse struct {
	Total       int64              `json:"total"`
	ByStatus    map[JobStatus]int64 `json:"by_status"`
	RecentCount int64              `json:"recent_count"`
}

// FiltersResponse lists available filter values for the search UI.
type FiltersResponse struct {
	Tags       []string `json:"tags"`
	SceneTypes []string `json:"scene_types"`
	Cameras    []string `json:"cameras"`
}
