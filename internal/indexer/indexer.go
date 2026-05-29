package indexer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/elastic/go-elasticsearch/v8/esutil"
	apperrors "github.com/zwh8800/phosche/internal/errors"
	"github.com/zwh8800/phosche/internal/types"
)

// queueItemKind distinguishes the type of operation queued for retry.
type queueItemKind int

const (
	queueItemIndex queueItemKind = iota
	queueItemUpdateStatus
)

// queueItem represents a deferred indexer operation.
type queueItem struct {
	kind      queueItemKind
	doc       *types.PhotoDocument
	path      string
	status    types.JobStatus
	indexName string
}

// IndexerService provides CRUD operations against Elasticsearch with
// circuit-breaker protection and a bounded retry queue.
type IndexerService struct {
	client              *ESClient
	queue               chan queueItem
	done                chan struct{}
	mu                  sync.RWMutex
	circuitOpen         bool
	failureCount        int
	maxFailures         int
	healthCheckInterval time.Duration
	logger              *slog.Logger
}

// NewIndexerService creates an IndexerService backed by the given ES client.
// queueSize determines the capacity of the in-memory retry queue.
func NewIndexerService(client *ESClient, queueSize int) *IndexerService {
	svc := &IndexerService{
		client:              client,
		queue:               make(chan queueItem, queueSize),
		done:                make(chan struct{}),
		maxFailures:         3,
		healthCheckInterval: 5 * time.Second,
		logger:              slog.Default(),
	}
	go svc.runCircuitBreaker()
	return svc
}

// Stop shuts down the background circuit-breaker goroutine and drains
// any remaining queued items before returning.
func (s *IndexerService) Stop() {
	close(s.done)
}

// --------------------------------------------------------------------------
// Public write operations (return nil on ES failure; queue for retry)
// --------------------------------------------------------------------------

// IndexPhoto indexes a photo document. The document _id is sha256(path).
// On ES failure the document is placed in the retry queue and nil is returned.
func (s *IndexerService) IndexPhoto(ctx context.Context, doc *types.PhotoDocument, indexName string) error {
	s.logger.Debug("IndexPhoto", "path", doc.Path, "status", doc.Status, "index", indexName)

	if s.isCircuitOpen() {
		s.queueDocument(doc, indexName)
		return nil
	}

	if err := s.indexDocDirect(ctx, doc, indexName); err != nil {
		s.logger.Warn("index failed, queuing", "path", doc.Path, "error", err)
		s.queueDocument(doc, indexName)
		s.recordWriteResult(false)
		return nil
	}

	s.recordWriteResult(true)
	return nil
}

// UpdateStatus updates only the status field of a photo document using
// the ES Update API. Other fields are not overwritten.
func (s *IndexerService) UpdateStatus(ctx context.Context, path string, status types.JobStatus, indexName string) error {
	s.logger.Debug("UpdateStatus", "path", path, "status", status, "index", indexName)

	if s.isCircuitOpen() {
		s.queueStatusUpdate(path, status, indexName)
		return nil
	}

	if err := s.updateStatusDirect(ctx, path, status, indexName); err != nil {
		s.logger.Warn("update status failed, queuing", "path", path, "error", err)
		s.queueStatusUpdate(path, status, indexName)
		s.recordWriteResult(false)
		return nil
	}

	s.recordWriteResult(true)
	return nil
}

// BulkIndex indexes multiple documents in a single ES bulk request.
// Returns an error containing the failure count if any individual document
// fails. On connection-level failure, all documents are queued for retry.
func (s *IndexerService) BulkIndex(ctx context.Context, docs []*types.PhotoDocument, indexName string) error {
	s.logger.Debug("BulkIndex", "count", len(docs), "index", indexName)

	if s.isCircuitOpen() {
		for _, doc := range docs {
			s.queueDocument(doc, indexName)
		}
		return nil
	}

	failed, err := s.bulkIndexDirect(ctx, docs, indexName)
	if err != nil {
		s.logger.Warn("bulk index connection failed, queuing", "count", len(docs), "error", err)
		for _, doc := range docs {
			s.queueDocument(doc, indexName)
		}
		s.recordWriteResult(false)
		return nil
	}

	if failed > 0 {
		s.recordWriteResult(true)
		return fmt.Errorf("bulk index: %d of %d documents failed", failed, len(docs))
	}

	s.recordWriteResult(true)
	return nil
}

// --------------------------------------------------------------------------
// Public read operation
// --------------------------------------------------------------------------

// DeletePhoto removes a photo document from the index by path.
func (s *IndexerService) DeletePhoto(ctx context.Context, path string, indexName string) error {
	docID := sha256hex(path)
	s.logger.Debug("DeletePhoto", "path", path, "doc_id", docID, "index", indexName)

	req := esapi.DeleteRequest{
		Index:      indexName,
		DocumentID: docID,
	}

	resp, err := req.Do(ctx, s.client.Client())
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() && resp.StatusCode != 404 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete returned %s: %s", resp.Status(), string(b))
	}

	return nil
}

// GetPhoto retrieves a photo document by path. Returns an *AppError with
// code NOT_FOUND if the document does not exist.
func (s *IndexerService) GetPhoto(ctx context.Context, path string, indexName string) (*types.PhotoDocument, error) {
	docID := sha256hex(path)
	s.logger.Debug("GetPhoto", "path", path, "doc_id", docID, "index", indexName)

	req := esapi.GetRequest{
		Index:      indexName,
		DocumentID: docID,
	}

	resp, err := req.Do(ctx, s.client.Client())
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, apperrors.NewNotFoundError("photo not found: " + path)
	}

	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get document returned %s: %s", resp.Status(), string(b))
	}

	var result struct {
		Source types.PhotoDocument `json:"_source"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode document: %w", err)
	}

	return &result.Source, nil
}

// --------------------------------------------------------------------------
// Internal: direct ES operations (no circuit breaker, used for retries)
// --------------------------------------------------------------------------

func (s *IndexerService) indexDocDirect(ctx context.Context, doc *types.PhotoDocument, indexName string) error {
	docID := sha256hex(doc.Path)
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal document: %w", err)
	}

	req := esapi.IndexRequest{
		Index:      indexName,
		DocumentID: docID,
		Body:       bytes.NewReader(body),
		Refresh:    "true",
	}

	resp, err := req.Do(ctx, s.client.Client())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("index returned status %s: %s", resp.Status(), string(b))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read index response: %w", err)
	}

	var ir indexResponse
	if err := json.Unmarshal(bodyBytes, &ir); err == nil {
		s.logger.Debug("indexed document",
			"path", doc.Path,
			"result", ir.Result,
			"seq_no", ir.SeqNo,
			"primary_term", ir.PrimaryTerm,
		)
	}

	return nil
}

type indexResponse struct {
	Result      string `json:"result"`
	SeqNo       int64  `json:"_seq_no"`
	PrimaryTerm int64  `json:"_primary_term"`
}

func (s *IndexerService) updateStatusDirect(ctx context.Context, path string, status types.JobStatus, indexName string) error {
	docID := sha256hex(path)

	updateBody := map[string]any{
		"doc": map[string]any{
			"status": status,
		},
	}
	bodyBytes, err := json.Marshal(updateBody)
	if err != nil {
		return fmt.Errorf("marshal update body: %w", err)
	}

	req := esapi.UpdateRequest{
		Index:      indexName,
		DocumentID: docID,
		Body:       bytes.NewReader(bodyBytes),
		Refresh:    "true",
	}

	resp, err := req.Do(ctx, s.client.Client())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.IsError() {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update returned status %s: %s", resp.Status(), string(b))
	}

	return nil
}

func (s *IndexerService) bulkIndexDirect(ctx context.Context, docs []*types.PhotoDocument, indexName string) (int, error) {
	bi, err := esutil.NewBulkIndexer(esutil.BulkIndexerConfig{
		Index:         indexName,
		Client:        s.client.Client(),
		NumWorkers:    4,
		FlushBytes:    5 * 1024 * 1024,
		FlushInterval: 30 * time.Second,
	})
	if err != nil {
		return 0, fmt.Errorf("create bulk indexer: %w", err)
	}

	var (
		mu     sync.Mutex
		failed int
	)

	for _, doc := range docs {
		docID := sha256hex(doc.Path)
		body, err := json.Marshal(doc)
		if err != nil {
			mu.Lock()
			failed++
			mu.Unlock()
			continue
		}

		err = bi.Add(ctx, esutil.BulkIndexerItem{
			Action:     "index",
			DocumentID: docID,
			Body:       bytes.NewReader(body),
			OnFailure: func(_ context.Context, _ esutil.BulkIndexerItem, _ esutil.BulkIndexerResponseItem, _ error) {
				mu.Lock()
				failed++
				mu.Unlock()
			},
		})
		if err != nil {
			mu.Lock()
			failed++
			mu.Unlock()
		}
	}

	if closeErr := bi.Close(ctx); closeErr != nil {
		return 0, fmt.Errorf("close bulk indexer: %w", closeErr)
	}

	return failed, nil
}

// --------------------------------------------------------------------------
// Circuit breaker & queue
// --------------------------------------------------------------------------

func (s *IndexerService) isCircuitOpen() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.circuitOpen
}

func (s *IndexerService) recordWriteResult(success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if success {
		s.failureCount = 0
		if s.circuitOpen {
			s.circuitOpen = false
			s.logger.Info("circuit breaker closed")
		}
	} else {
		s.failureCount++
		if s.failureCount >= s.maxFailures && !s.circuitOpen {
			s.circuitOpen = true
			s.logger.Warn("circuit breaker opened", "failure_count", s.failureCount)
		}
	}
}

func (s *IndexerService) queueDocument(doc *types.PhotoDocument, indexName string) {
	select {
	case s.queue <- queueItem{kind: queueItemIndex, doc: doc, indexName: indexName}:
		s.logger.Debug("queued document for retry", "path", doc.Path)
	default:
		s.logger.Warn("queue full, dropping document", "path", doc.Path)
	}
}

func (s *IndexerService) queueStatusUpdate(path string, status types.JobStatus, indexName string) {
	select {
	case s.queue <- queueItem{kind: queueItemUpdateStatus, path: path, status: status, indexName: indexName}:
		s.logger.Debug("queued status update for retry", "path", path)
	default:
		s.logger.Warn("queue full, dropping status update", "path", path)
	}
}

func (s *IndexerService) runCircuitBreaker() {
	ticker := time.NewTicker(s.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			s.drainQueue()
			return
		case <-ticker.C:
			s.mu.RLock()
			open := s.circuitOpen
			s.mu.RUnlock()

			if !open {
				continue
			}

			if s.pingES() {
				s.logger.Info("ES health check passed, draining queue")
				s.mu.Lock()
				s.circuitOpen = false
				s.failureCount = 0
				s.mu.Unlock()
				s.drainQueue()
			}
		}
	}
}

func (s *IndexerService) pingES() bool {
	resp, err := s.client.Client().Info()
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return !resp.IsError()
}

func (s *IndexerService) drainQueue() {
	count := 0
	for {
		select {
		case item := <-s.queue:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			var retryErr error
			switch item.kind {
			case queueItemIndex:
				retryErr = s.indexDocDirect(ctx, item.doc, item.indexName)
			case queueItemUpdateStatus:
				retryErr = s.updateStatusDirect(ctx, item.path, item.status, item.indexName)
			}
			cancel()

			if retryErr != nil {
				s.logger.Warn("retry failed, dropping queue item",
					"kind", item.kind,
					"index", item.indexName,
					"error", retryErr,
				)
			} else {
				count++
			}
		default:
			if count > 0 {
				s.logger.Info("queue drained", "count", count)
			}
			return
		}
	}
}

// sha256hex returns the hex-encoded SHA-256 hash of s.
func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
