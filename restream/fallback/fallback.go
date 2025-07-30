package fallback

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/datarhei/core/v16/ffmpeg"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/process"
	"github.com/datarhei/core/v16/restream/app"
)

// StreamState represents the current state of a monitored stream
type StreamState int

const (
	StreamStateUnknown StreamState = iota
	StreamStateHealthy
	StreamStateFailing
	StreamStateFailed
	StreamStateFallback
)

func (s StreamState) String() string {
	switch s {
	case StreamStateHealthy:
		return "healthy"
	case StreamStateFailing:
		return "failing"
	case StreamStateFailed:
		return "failed"
	case StreamStateFallback:
		return "fallback"
	default:
		return "unknown"
	}
}

// StreamHealth contains health metrics for a stream
type StreamHealth struct {
	State              StreamState
	LastSeen           time.Time
	LastFailure        time.Time
	FailureDuration    time.Duration
	SilenceDuration    time.Duration
	FrameCount         uint64
	LastFrameCount     uint64
	ConsecutiveFailures uint64
	InFallback         bool
	CurrentFallbackIndex int
}

// Monitor monitors stream health and manages fallback switching
type Monitor struct {
	inputID    string
	config     *app.ConfigFallback
	ffmpeg     ffmpeg.FFmpeg
	logger     log.Logger
	
	// Stream monitoring
	health     StreamHealth
	healthLock sync.RWMutex
	
	// Fallback management
	fallbackProcess process.Process
	fallbackLock    sync.RWMutex
	
	// Control
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	
	// Callbacks
	onFallbackSwitch func(inputID string, fallbackIndex int, source *app.ConfigFallbackSource)
	onRecovery       func(inputID string)
	onFailure        func(inputID string, reason string)
}

// Config contains configuration for creating a new fallback monitor
type Config struct {
	InputID          string
	FallbackConfig   *app.ConfigFallback
	FFmpeg           ffmpeg.FFmpeg
	Logger           log.Logger
	OnFallbackSwitch func(inputID string, fallbackIndex int, source *app.ConfigFallbackSource)
	OnRecovery       func(inputID string)
	OnFailure        func(inputID string, reason string)
}

// New creates a new fallback monitor
func New(config Config) (*Monitor, error) {
	if config.FallbackConfig == nil {
		return nil, fmt.Errorf("fallback config is required")
	}
	
	if config.FFmpeg == nil {
		return nil, fmt.Errorf("ffmpeg instance is required")
	}
	
	if config.Logger == nil {
		config.Logger = log.New("")
	}
	
	// Set default values for fallback config
	fallbackConfig := *config.FallbackConfig
	if fallbackConfig.FailureThreshold == 0 {
		fallbackConfig.FailureThreshold = 5000 // 5 seconds
	}
	if fallbackConfig.SilenceThreshold == 0 {
		fallbackConfig.SilenceThreshold = 10000 // 10 seconds
	}
	if fallbackConfig.RecoveryThreshold == 0 {
		fallbackConfig.RecoveryThreshold = 10000 // 10 seconds
	}
	if fallbackConfig.CheckInterval == 0 {
		fallbackConfig.CheckInterval = 1000 // 1 second
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	monitor := &Monitor{
		inputID:          config.InputID,
		config:           &fallbackConfig,
		ffmpeg:           config.FFmpeg,
		logger:           config.Logger.WithField("fallback_input", config.InputID),
		ctx:              ctx,
		cancel:           cancel,
		onFallbackSwitch: config.OnFallbackSwitch,
		onRecovery:       config.OnRecovery,
		onFailure:        config.OnFailure,
		health: StreamHealth{
			State:                 StreamStateUnknown,
			CurrentFallbackIndex: -1,
		},
	}
	
	return monitor, nil
}

// Start begins monitoring the stream health
func (m *Monitor) Start() {
	if !m.config.Enabled {
		m.logger.Debug().Log("Fallback monitoring disabled")
		return
	}
	
	m.logger.Info().Log("Starting fallback monitoring")
	
	m.wg.Add(1)
	go m.monitorLoop()
}

// Stop stops the fallback monitor
func (m *Monitor) Stop() {
	m.logger.Info().Log("Stopping fallback monitoring")
	
	m.cancel()
	m.wg.Wait()
	
	// Stop any active fallback process
	m.stopFallback()
}

// UpdateHealth updates the health metrics for the primary stream
func (m *Monitor) UpdateHealth(progress app.Progress) {
	m.healthLock.Lock()
	defer m.healthLock.Unlock()
	
	now := time.Now()
	m.health.LastSeen = now
	
	// Check if we have frame progression
	if len(progress.Input) > 0 {
		currentFrames := progress.Input[0].Frame
		if currentFrames > m.health.LastFrameCount {
			m.health.LastFrameCount = currentFrames
			m.health.FrameCount = currentFrames
			
			// Reset failure counters if we're receiving frames
			if m.health.State == StreamStateFailing {
				m.health.State = StreamStateHealthy
				m.health.ConsecutiveFailures = 0
				m.logger.Debug().Log("Stream recovered, resetting failure counters")
			}
		}
	}
}

// GetHealth returns the current stream health
func (m *Monitor) GetHealth() StreamHealth {
	m.healthLock.RLock()
	defer m.healthLock.RUnlock()
	return m.health
}

// IsInFallback returns true if currently using fallback source
func (m *Monitor) IsInFallback() bool {
	m.healthLock.RLock()
	defer m.healthLock.RUnlock()
	return m.health.InFallback
}

// monitorLoop is the main monitoring loop
func (m *Monitor) monitorLoop() {
	defer m.wg.Done()
	
	ticker := time.NewTicker(time.Duration(m.config.CheckInterval) * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkStreamHealth()
		}
	}
}

// checkStreamHealth evaluates the current stream health and takes action if needed
func (m *Monitor) checkStreamHealth() {
	m.healthLock.Lock()
	
	now := time.Now()
	timeSinceLastSeen := now.Sub(m.health.LastSeen)
	
	// Determine current state
	oldState := m.health.State
	
	if timeSinceLastSeen > time.Duration(m.config.FailureThreshold)*time.Millisecond {
		if m.health.State == StreamStateHealthy {
			m.health.State = StreamStateFailing
			m.health.LastFailure = now
			m.health.ConsecutiveFailures++
			
			m.logger.Warn().WithFields(log.Fields{
				"time_since_last_seen": timeSinceLastSeen,
				"failure_threshold":    m.config.FailureThreshold,
			}).Log("Stream appears to be failing")
			
			if m.onFailure != nil {
				go m.onFailure(m.inputID, fmt.Sprintf("No frames received for %v", timeSinceLastSeen))
			}
		} else if m.health.State == StreamStateFailing {
			// If we've been failing for too long, mark as failed
			failureDuration := now.Sub(m.health.LastFailure)
			if failureDuration > time.Duration(m.config.FailureThreshold)*time.Millisecond {
				m.health.State = StreamStateFailed
				m.health.FailureDuration = failureDuration
				
				m.logger.Error().WithField("failure_duration", failureDuration).Log("Stream has failed")
			}
		}
	}
	
	// Handle state transitions
	newState := m.health.State
	m.healthLock.Unlock()
	
	if oldState != newState {
		m.handleStateChange(oldState, newState)
	}
	
	// If we're in fallback and recovery is enabled, check if primary is back
	if m.health.InFallback && m.config.RecoveryEnabled {
		m.checkPrimaryRecovery()
	}
}

// handleStateChange handles transitions between stream states
func (m *Monitor) handleStateChange(oldState, newState StreamState) {
	m.logger.Info().WithFields(log.Fields{
		"old_state": oldState.String(),
		"new_state": newState.String(),
	}).Log("Stream state changed")
	
	switch newState {
	case StreamStateFailed:
		if !m.health.InFallback {
			m.startFallback()
		}
	case StreamStateHealthy:
		if m.health.InFallback {
			m.startRecovery()
		}
	}
}

// startFallback initiates fallback to the next available source
func (m *Monitor) startFallback() {
	if len(m.config.Sources) == 0 {
		m.logger.Error().Log("No fallback sources configured")
		return
	}
	
	m.healthLock.Lock()
	m.health.InFallback = true
	m.health.CurrentFallbackIndex = 0
	m.health.State = StreamStateFallback
	m.healthLock.Unlock()
	
	source := &m.config.Sources[0]
	
	m.logger.Info().WithFields(log.Fields{
		"fallback_type":    source.Type,
		"fallback_address": source.Address,
	}).Log("Starting fallback")
	
	if err := m.createFallbackProcess(source); err != nil {
		m.logger.Error().WithError(err).Log("Failed to create fallback process")
		return
	}
	
	if m.onFallbackSwitch != nil {
		go m.onFallbackSwitch(m.inputID, 0, source)
	}
}

// createFallbackProcess creates and starts a fallback FFmpeg process
func (m *Monitor) createFallbackProcess(source *app.ConfigFallbackSource) error {
	m.fallbackLock.Lock()
	defer m.fallbackLock.Unlock()
	
	// Stop any existing fallback process
	if m.fallbackProcess != nil {
		m.fallbackProcess.Stop(true)
		m.fallbackProcess = nil
	}
	
	command := m.buildFallbackCommand(source)
	parser := m.ffmpeg.NewProcessParser(m.logger, fmt.Sprintf("%s_fallback", m.inputID), "")
	
	fallbackProcess, err := m.ffmpeg.New(ffmpeg.ProcessConfig{
		Reconnect:      false, // Fallback sources should not reconnect
		ReconnectDelay: 0,
		StaleTimeout:   30 * time.Second,
		Command:        command,
		Parser:         parser,
		Logger:         m.logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create fallback process: %w", err)
	}
	
	m.fallbackProcess = fallbackProcess
	fallbackProcess.Start()
	
	m.logger.Info().WithField("command", command).Log("Fallback process started")
	
	return nil
}

// buildFallbackCommand builds the FFmpeg command for a fallback source
func (m *Monitor) buildFallbackCommand(source *app.ConfigFallbackSource) []string {
	var command []string
	
	// Add source-specific options
	command = append(command, source.Options...)
	
	switch source.Type {
	case "image":
		command = append(command, "-loop", "1", "-i", source.Address)
		command = append(command, "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000")
		if source.Loop {
			command = append(command, "-shortest")
		}
	case "video":
		if source.Loop {
			command = append(command, "-stream_loop", "-1")
		}
		command = append(command, "-i", source.Address)
	case "rtmp":
		command = append(command, "-i", source.Address)
	default:
		m.logger.Warn().WithField("type", source.Type).Log("Unknown fallback source type, treating as generic input")
		command = append(command, "-i", source.Address)
	}
	
	// Add output configuration to match the original stream format
	command = append(command, "-c", "copy", "-f", "null", "-")
	
	return command
}

// checkPrimaryRecovery checks if the primary stream has recovered
func (m *Monitor) checkPrimaryRecovery() {
	m.healthLock.RLock()
	timeSinceLastSeen := time.Since(m.health.LastSeen)
	m.healthLock.RUnlock()
	
	// If we've been receiving frames consistently, consider recovery
	if timeSinceLastSeen < time.Duration(m.config.RecoveryThreshold)*time.Millisecond {
		m.startRecovery()
	}
}

// startRecovery initiates recovery back to the primary stream
func (m *Monitor) startRecovery() {
	if !m.health.InFallback {
		return
	}
	
	m.logger.Info().Log("Primary stream appears to have recovered, switching back")
	
	m.healthLock.Lock()
	m.health.InFallback = false
	m.health.CurrentFallbackIndex = -1
	m.health.State = StreamStateHealthy
	m.health.ConsecutiveFailures = 0
	m.healthLock.Unlock()
	
	m.stopFallback()
	
	if m.onRecovery != nil {
		go m.onRecovery(m.inputID)
	}
}

// stopFallback stops the current fallback process
func (m *Monitor) stopFallback() {
	m.fallbackLock.Lock()
	defer m.fallbackLock.Unlock()
	
	if m.fallbackProcess != nil {
		m.logger.Info().Log("Stopping fallback process")
		m.fallbackProcess.Stop(true)
		m.fallbackProcess = nil
	}
}