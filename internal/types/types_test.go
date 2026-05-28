package types

import (
	"encoding/json"
	"testing"
)

func TestPhotoRoundTrip(t *testing.T) {
	analyzedAt := int64(200)
	original := Photo{
		ID:         "photo-001",
		Path:       "/photos/001.jpg",
		MTime:      100,
		Size:       1024,
		Status:     StatusAnalyzed,
		AnalyzedAt: &analyzedAt,
		EXIF: &EXIFInfo{
			CameraModel: "Canon EOS R5",
			ISO:         400,
		},
		CreatedAt: 50,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded Photo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID: got %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Path != original.Path {
		t.Errorf("Path: got %q, want %q", decoded.Path, original.Path)
	}
	if decoded.MTime != original.MTime {
		t.Errorf("MTime: got %d, want %d", decoded.MTime, original.MTime)
	}
	if decoded.Size != original.Size {
		t.Errorf("Size: got %d, want %d", decoded.Size, original.Size)
	}
	if decoded.Status != original.Status {
		t.Errorf("Status: got %q, want %q", decoded.Status, original.Status)
	}
	if decoded.AnalyzedAt == nil {
		t.Fatal("AnalyzedAt is nil")
	}
	if *decoded.AnalyzedAt != *original.AnalyzedAt {
		t.Errorf("AnalyzedAt: got %d, want %d", *decoded.AnalyzedAt, *original.AnalyzedAt)
	}
	if decoded.EXIF == nil {
		t.Fatal("EXIF is nil")
	}
	if decoded.EXIF.CameraModel != original.EXIF.CameraModel {
		t.Errorf("CameraModel: got %q, want %q", decoded.EXIF.CameraModel, original.EXIF.CameraModel)
	}
	if decoded.EXIF.ISO != original.EXIF.ISO {
		t.Errorf("ISO: got %d, want %d", decoded.EXIF.ISO, original.EXIF.ISO)
	}
	if decoded.CreatedAt != original.CreatedAt {
		t.Errorf("CreatedAt: got %d, want %d", decoded.CreatedAt, original.CreatedAt)
	}
}

func TestSearchRequest_OptionalFields(t *testing.T) {
	data := `{"page": 1, "page_size": 20}`
	var req SearchRequest
	if err := json.Unmarshal([]byte(data), &req); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if req.Query != "" {
		t.Errorf("Query: got %q, want empty", req.Query)
	}
	if req.DateFrom != "" {
		t.Errorf("DateFrom: got %q, want empty", req.DateFrom)
	}
	if req.Page != 1 {
		t.Errorf("Page: got %d, want 1", req.Page)
	}
	if req.PageSize != 20 {
		t.Errorf("PageSize: got %d, want 20", req.PageSize)
	}
	if req.Tags != nil {
		t.Errorf("Tags: got %v, want nil", req.Tags)
	}
}

func TestJobStatus_Values(t *testing.T) {
	cases := []struct {
		status JobStatus
		want   string
	}{
		{StatusUnanalyzed, "unanalyzed"},
		{StatusAnalyzing, "analyzing"},
		{StatusAnalyzed, "analyzed"},
		{StatusFailed, "failed"},
		{StatusPendingAnalysis, "pending_analysis"},
	}
	for _, c := range cases {
		if got := string(c.status); got != c.want {
			t.Errorf("JobStatus(%q): got %q, want %q", c.status, got, c.want)
		}
	}
}

func TestPhotoDocument_Embeds(t *testing.T) {
	doc := PhotoDocument{
		Photo: Photo{
			ID:   "p1",
			Path: "/p1.jpg",
		},
		AnalysisResult: AnalysisResult{
			Description: "a cat",
			Tags:        []string{"cat", "animal"},
		},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var decoded PhotoDocument
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded.ID != "p1" {
		t.Errorf("ID: got %q, want %q", decoded.ID, "p1")
	}
	if decoded.Description != "a cat" {
		t.Errorf("Description: got %q, want %q", decoded.Description, "a cat")
	}
	if len(decoded.Tags) != 2 || decoded.Tags[0] != "cat" || decoded.Tags[1] != "animal" {
		t.Errorf("Tags: got %v, want [cat animal]", decoded.Tags)
	}
}
