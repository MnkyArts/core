package app

import (
	"github.com/datarhei/core/v16/process"
)

type ConfigIOCleanup struct {
	Pattern       string `json:"pattern"`
	MaxFiles      uint   `json:"max_files"`
	MaxFileAge    uint   `json:"max_file_age_seconds"`
	PurgeOnDelete bool   `json:"purge_on_delete"`
}

// ConfigFallbackSource represents a fallback source configuration
type ConfigFallbackSource struct {
	Type     string   `json:"type"`     // "image", "video", "rtmp"
	Address  string   `json:"address"`  // Path to file or RTMP URL
	Options  []string `json:"options"`  // FFmpeg options for this fallback source
	Loop     bool     `json:"loop"`     // Whether to loop video files
}

// ConfigFallback represents fallback configuration for an input
type ConfigFallback struct {
	Enabled              bool                    `json:"enabled"`                 // Enable fallback functionality
	Sources              []ConfigFallbackSource `json:"sources"`                 // List of fallback sources in priority order
	FailureThreshold     uint64                  `json:"failure_threshold_ms"`   // Milliseconds before considering stream failed
	SilenceThreshold     uint64                  `json:"silence_threshold_ms"`   // Milliseconds of silence before fallback
	RecoveryEnabled      bool                    `json:"recovery_enabled"`       // Whether to automatically return to primary
	RecoveryThreshold    uint64                  `json:"recovery_threshold_ms"`  // Milliseconds of stable primary before recovery
	CheckInterval        uint64                  `json:"check_interval_ms"`      // How often to check stream health
}

type ConfigIO struct {
	ID       string            `json:"id"`
	Address  string            `json:"address"`
	Options  []string          `json:"options"`
	Cleanup  []ConfigIOCleanup `json:"cleanup"`
	Fallback ConfigFallback    `json:"fallback"` // Fallback configuration for inputs
}

func (io ConfigIO) Clone() ConfigIO {
	clone := ConfigIO{
		ID:      io.ID,
		Address: io.Address,
	}

	clone.Options = make([]string, len(io.Options))
	copy(clone.Options, io.Options)

	clone.Cleanup = make([]ConfigIOCleanup, len(io.Cleanup))
	copy(clone.Cleanup, io.Cleanup)

	// Clone fallback configuration
	clone.Fallback = ConfigFallback{
		Enabled:              io.Fallback.Enabled,
		FailureThreshold:     io.Fallback.FailureThreshold,
		SilenceThreshold:     io.Fallback.SilenceThreshold,
		RecoveryEnabled:      io.Fallback.RecoveryEnabled,
		RecoveryThreshold:    io.Fallback.RecoveryThreshold,
		CheckInterval:        io.Fallback.CheckInterval,
	}

	clone.Fallback.Sources = make([]ConfigFallbackSource, len(io.Fallback.Sources))
	for i, source := range io.Fallback.Sources {
		clone.Fallback.Sources[i] = ConfigFallbackSource{
			Type:    source.Type,
			Address: source.Address,
			Loop:    source.Loop,
		}
		clone.Fallback.Sources[i].Options = make([]string, len(source.Options))
		copy(clone.Fallback.Sources[i].Options, source.Options)
	}

	return clone
}

type Config struct {
	ID             string     `json:"id"`
	Reference      string     `json:"reference"`
	FFVersion      string     `json:"ffversion"`
	Input          []ConfigIO `json:"input"`
	Output         []ConfigIO `json:"output"`
	Options        []string   `json:"options"`
	Reconnect      bool       `json:"reconnect"`
	ReconnectDelay uint64     `json:"reconnect_delay_seconds"` // seconds
	Autostart      bool       `json:"autostart"`
	StaleTimeout   uint64     `json:"stale_timeout_seconds"` // seconds
	LimitCPU       float64    `json:"limit_cpu_usage"`       // percent
	LimitMemory    uint64     `json:"limit_memory_bytes"`    // bytes
	LimitWaitFor   uint64     `json:"limit_waitfor_seconds"` // seconds
}

func (config *Config) Clone() *Config {
	clone := &Config{
		ID:             config.ID,
		Reference:      config.Reference,
		FFVersion:      config.FFVersion,
		Reconnect:      config.Reconnect,
		ReconnectDelay: config.ReconnectDelay,
		Autostart:      config.Autostart,
		StaleTimeout:   config.StaleTimeout,
		LimitCPU:       config.LimitCPU,
		LimitMemory:    config.LimitMemory,
		LimitWaitFor:   config.LimitWaitFor,
	}

	clone.Input = make([]ConfigIO, len(config.Input))
	for i, io := range config.Input {
		clone.Input[i] = io.Clone()
	}

	clone.Output = make([]ConfigIO, len(config.Output))
	for i, io := range config.Output {
		clone.Output[i] = io.Clone()
	}

	clone.Options = make([]string, len(config.Options))
	copy(clone.Options, config.Options)

	return clone
}

// CreateCommand created the FFmpeg command from this config.
func (config *Config) CreateCommand() []string {
	var command []string

	// Copy global options
	command = append(command, config.Options...)

	for _, input := range config.Input {
		// Add the resolved input to the process command
		command = append(command, input.Options...)
		command = append(command, "-i", input.Address)
	}

	for _, output := range config.Output {
		// Add the resolved output to the process command
		command = append(command, output.Options...)
		command = append(command, output.Address)
	}

	return command
}

type Process struct {
	ID        string  `json:"id"`
	Reference string  `json:"reference"`
	Config    *Config `json:"config"`
	CreatedAt int64   `json:"created_at"`
	UpdatedAt int64   `json:"updated_at"`
	Order     string  `json:"order"`
}

func (process *Process) Clone() *Process {
	clone := &Process{
		ID:        process.ID,
		Reference: process.Reference,
		Config:    process.Config.Clone(),
		CreatedAt: process.CreatedAt,
		UpdatedAt: process.UpdatedAt,
		Order:     process.Order,
	}

	return clone
}

type ProcessStates struct {
	Finished  uint64
	Starting  uint64
	Running   uint64
	Finishing uint64
	Failed    uint64
	Killed    uint64
}

func (p *ProcessStates) Marshal(s process.States) {
	p.Finished = s.Finished
	p.Starting = s.Starting
	p.Running = s.Running
	p.Finishing = s.Finishing
	p.Failed = s.Failed
	p.Killed = s.Killed
}

type State struct {
	Order     string        // Current order, e.g. "start", "stop"
	State     string        // Current state, e.g. "running"
	States    ProcessStates // Cumulated process states
	Time      int64         // Unix timestamp of last status change
	Duration  float64       // Runtime in seconds since last status change
	Reconnect float64       // Seconds until next reconnect, negative if not reconnecting
	LastLog   string        // Last recorded line from the process
	Progress  Progress      // Progress data of the process
	Memory    uint64        // Current memory consumption in bytes
	CPU       float64       // Current CPU consumption in percent
	Command   []string      // ffmpeg command line parameters
}
