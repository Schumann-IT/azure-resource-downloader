package pipeline

import (
	"fmt"
	"sync"
	"time"

	"azure-resource-downloader/internal/logger"
)

// StageMetrics tracks performance metrics for a pipeline stage
type StageMetrics struct {
	StageName      string
	StartTime      time.Time
	EndTime        time.Time
	ItemsProcessed int
	mu             sync.Mutex
}

// NewStageMetrics creates a new metrics tracker
func NewStageMetrics(stageName string) *StageMetrics {
	return &StageMetrics{
		StageName: stageName,
		StartTime: time.Now(),
	}
}

// IncrementProcessed increments the processed counter
func (m *StageMetrics) IncrementProcessed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ItemsProcessed++
}

// Complete marks the stage as complete and logs metrics
func (m *StageMetrics) Complete() {
	m.EndTime = time.Now()
	duration := m.EndTime.Sub(m.StartTime)

	log := logger.Default
	log.Info("Stage completed",
		"stage", m.StageName,
		"duration", duration.Round(time.Millisecond),
		"items", m.ItemsProcessed,
		"items_per_sec", float64(m.ItemsProcessed)/duration.Seconds())
}

// PipelineMetrics tracks overall pipeline performance
type PipelineMetrics struct {
	StartTime       time.Time
	FirstResultTime time.Time
	LastResultTime  time.Time
	TotalItems      int
	WorkerCount     int
	mu              sync.Mutex
}

// NewPipelineMetrics creates a new pipeline metrics tracker
func NewPipelineMetrics(workerCount, totalItems int) *PipelineMetrics {
	return &PipelineMetrics{
		StartTime:   time.Now(),
		WorkerCount: workerCount,
		TotalItems:  totalItems,
	}
}

// RecordResult records when a result is received
func (m *PipelineMetrics) RecordResult() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if m.FirstResultTime.IsZero() {
		m.FirstResultTime = now
	}
	m.LastResultTime = now
}

// LogSummary logs the complete pipeline metrics
func (m *PipelineMetrics) LogSummary() {
	totalDuration := time.Since(m.StartTime)

	log := logger.Default
	log.Info("═══════════════════════════════════════════════════════════")
	log.Info("Pipeline Performance Summary",
		"total_duration", totalDuration.Round(time.Millisecond),
		"workers", m.WorkerCount,
		"total_items", m.TotalItems,
		"throughput_items_per_sec", fmt.Sprintf("%.2f", float64(m.TotalItems)/totalDuration.Seconds()))

	if !m.FirstResultTime.IsZero() {
		timeToFirst := m.FirstResultTime.Sub(m.StartTime)
		log.Info("Pipeline latency",
			"time_to_first_result", timeToFirst.Round(time.Millisecond),
			"note", "Time from start until first resource completed all stages")
	}
}
