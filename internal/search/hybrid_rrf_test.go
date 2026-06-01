package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zwh8800/phosche/internal/types"
)

func newTestService(rankConstant int) *SearchService {
	return &SearchService{hybridCfg: HybridConfig{RRFRankConstant: rankConstant}}
}

func makeHits(ids ...string) []msearchHit {
	hits := make([]msearchHit, len(ids))
	for i, id := range ids {
		hits[i] = msearchHit{
			ID:     id,
			Source: types.PhotoDocument{Photo: types.Photo{ID: id}},
		}
	}
	return hits
}

func idList(docs []scoredDoc) []string {
	ids := make([]string, len(docs))
	for i, d := range docs {
		ids[i] = d.id
	}
	return ids
}

func TestReciprocalRankFusion_BothResults(t *testing.T) {
	s := newTestService(60)

	bm25Hits := makeHits("a", "b", "c")
	knnHits := makeHits("b", "d", "e")

	result := s.reciprocalRankFusion(bm25Hits, knnHits, 60)
	require.Len(t, result, 5)
	assert.Equal(t, "b", result[0].id, "overlapping doc should be ranked first")
}

func TestReciprocalRankFusion_BM25Only(t *testing.T) {
	s := newTestService(60)
	bm25Hits := makeHits("a", "b", "c")

	result := s.reciprocalRankFusion(bm25Hits, nil, 60)
	require.Len(t, result, 3)
	assert.Equal(t, []string{"a", "b", "c"}, idList(result))
}

func TestReciprocalRankFusion_KNNOnly(t *testing.T) {
	s := newTestService(60)
	knnHits := makeHits("x", "y", "z")

	result := s.reciprocalRankFusion(nil, knnHits, 60)
	require.Len(t, result, 3)
	assert.Equal(t, []string{"x", "y", "z"}, idList(result))
}

func TestReciprocalRankFusion_BothEmpty(t *testing.T) {
	s := newTestService(60)

	result := s.reciprocalRankFusion(nil, nil, 60)
	assert.Empty(t, result)
}

func TestReciprocalRankFusion_Deduplication(t *testing.T) {
	s := newTestService(60)

	bm25Hits := makeHits("dup")
	knnHits := makeHits("dup")

	result := s.reciprocalRankFusion(bm25Hits, knnHits, 60)
	require.Len(t, result, 1)

	expectedScore := 1.0/61.0 + 1.0/61.0
	assert.InDelta(t, expectedScore, result[0].score, 1e-9)
}

func TestReciprocalRankFusion_SortStability(t *testing.T) {
	s := newTestService(60)

	bm25Hits := makeHits("z", "a", "m")
	knnHits := []msearchHit{}

	result := s.reciprocalRankFusion(bm25Hits, knnHits, 60)
	require.Len(t, result, 3)
	assert.Equal(t, []string{"z", "a", "m"}, idList(result), "original rank order preserved when scores differ")

	bm25Same := []msearchHit{
		{ID: "beta", Source: types.PhotoDocument{Photo: types.Photo{ID: "beta"}}},
		{ID: "alpha", Source: types.PhotoDocument{Photo: types.Photo{ID: "alpha"}}},
	}
	knnSame := []msearchHit{
		{ID: "alpha", Source: types.PhotoDocument{Photo: types.Photo{ID: "alpha"}}},
		{ID: "beta", Source: types.PhotoDocument{Photo: types.Photo{ID: "beta"}}},
	}
	sameRank := s.reciprocalRankFusion(bm25Same, knnSame, 60)
	require.Len(t, sameRank, 2)
	assert.Equal(t, "alpha", sameRank[0].id, "same-score docs sorted by ID ascending")
	assert.Equal(t, "beta", sameRank[1].id)
	assert.InDelta(t, sameRank[0].score, sameRank[1].score, 1e-9)
}

func TestPaginateResults_Normal(t *testing.T) {
	docs := makeScoredDocs(10)

	page := paginateResults(docs, 0, 3)
	require.Len(t, page, 3)
	assert.Equal(t, "0", page[0].id)
	assert.Equal(t, "2", page[2].id)
}

func TestPaginateResults_LastPage(t *testing.T) {
	docs := makeScoredDocs(7)

	page := paginateResults(docs, 5, 3)
	require.Len(t, page, 2)
	assert.Equal(t, "5", page[0].id)
	assert.Equal(t, "6", page[1].id)
}

func TestPaginateResults_BeyondEnd(t *testing.T) {
	docs := makeScoredDocs(5)

	page := paginateResults(docs, 10, 3)
	assert.Empty(t, page)
}

func TestCalculateRankWindowSize(t *testing.T) {
	tests := []struct {
		name     string
		from     int
		pageSize int
		expected int
	}{
		{"small values clamped to 50", 0, 20, 50},
		{"exactly at minimum", 30, 20, 50},
		{"above minimum", 100, 20, 120},
		{"large values clamped to 1000", 900, 200, 1000},
		{"exactly at 1000", 800, 200, 1000},
		{"zero zero clamped to 50", 0, 0, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, calculateRankWindowSize(tt.from, tt.pageSize))
		})
	}
}

func makeScoredDocs(n int) []scoredDoc {
	docs := make([]scoredDoc, n)
	for i := range docs {
		docs[i] = scoredDoc{
			id:     string(rune('0' + i)),
			score:  float64(n - i),
			source: types.PhotoDocument{Photo: types.Photo{ID: string(rune('0' + i))}},
		}
	}
	return docs
}
