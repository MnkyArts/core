package fallback

import (
	"strings"
	"testing"
	"time"

	"github.com/datarhei/core/v16/ffmpeg"
	"github.com/datarhei/core/v16/internal/testhelper"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"

	"github.com/stretchr/testify/require"
)

func getDummyFFmpeg() (ffmpeg.FFmpeg, error) {
	binary, err := testhelper.BuildBinary("ffmpeg", "../../internal/testhelper")
	if err != nil {
		return nil, err
	}

	return ffmpeg.New(ffmpeg.Config{
		Binary:           binary,
		LogHistoryLength: 3,
	})
}

func getDummyFallbackConfig() *app.ConfigFallback {
	return &app.ConfigFallback{
		Enabled:              true,
		FailureThreshold:     1000, // 1 second
		SilenceThreshold:     2000, // 2 seconds
		RecoveryEnabled:      true,
		RecoveryThreshold:    1000, // 1 second
		CheckInterval:        100,  // 100ms
		Sources: []app.ConfigFallbackSource{
			{
				Type:    "image",
				Address: "/tmp/fallback.png",
				Options: []string{"-f", "lavfi"},
				Loop:    true,
			},
			{
				Type:    "video",
				Address: "/tmp/fallback.mp4",
				Options: []string{"-re"},
				Loop:    true,
			},
		},
	}
}

func TestNewMonitor(t *testing.T) {
	ffmpeg, err := getDummyFFmpeg()
	require.NoError(t, err)

	config := getDummyFallbackConfig()
	
	monitor, err := New(Config{
		InputID:        "test_input",
		FallbackConfig: config,
		FFmpeg:         ffmpeg,
		Logger:         log.New("test"),
	})
	
	require.NoError(t, err)
	require.NotNil(t, monitor)
	require.Equal(t, "test_input", monitor.inputID)
	require.Equal(t, config, monitor.config)
}

func TestNewMonitorWithoutConfig(t *testing.T) {
	ffmpeg, err := getDummyFFmpeg()
	require.NoError(t, err)

	_, err = New(Config{
		InputID:        "test_input",
		FallbackConfig: nil,
		FFmpeg:         ffmpeg,
		Logger:         log.New("test"),
	})
	
	require.Error(t, err)
}

func TestNewMonitorWithoutFFmpeg(t *testing.T) {
	config := getDummyFallbackConfig()
	
	_, err := New(Config{
		InputID:        "test_input",
		FallbackConfig: config,
		FFmpeg:         nil,
		Logger:         log.New("test"),
	})
	
	require.Error(t, err)
}

func TestMonitorDefaults(t *testing.T) {
	ffmpeg, err := getDummyFFmpeg()
	require.NoError(t, err)

	config := &app.ConfigFallback{
		Enabled: true,
		Sources: []app.ConfigFallbackSource{
			{
				Type:    "image",
				Address: "/tmp/test.png",
			},
		},
	}
	
	monitor, err := New(Config{
		InputID:        "test_input",
		FallbackConfig: config,
		FFmpeg:         ffmpeg,
		Logger:         log.New("test"),
	})
	
	require.NoError(t, err)
	require.Equal(t, uint64(5000), monitor.config.FailureThreshold)
	require.Equal(t, uint64(10000), monitor.config.SilenceThreshold)
	require.Equal(t, uint64(10000), monitor.config.RecoveryThreshold)
	require.Equal(t, uint64(1000), monitor.config.CheckInterval)
}

func TestUpdateHealth(t *testing.T) {
	ffmpeg, err := getDummyFFmpeg()
	require.NoError(t, err)

	config := getDummyFallbackConfig()
	
	monitor, err := New(Config{
		InputID:        "test_input",
		FallbackConfig: config,
		FFmpeg:         ffmpeg,
		Logger:         log.New("test"),
	})
	
	require.NoError(t, err)

	// Initial health should be unknown
	health := monitor.GetHealth()
	require.Equal(t, StreamStateUnknown, health.State)
	require.Equal(t, uint64(0), health.FrameCount)

	// Update with progress containing frames
	progress := app.Progress{
		Input: []app.ProgressIO{
			{
				Frame: 100,
			},
		},
	}
	
	monitor.UpdateHealth(progress)
	
	health = monitor.GetHealth()
	require.Equal(t, uint64(100), health.FrameCount)
	require.Equal(t, uint64(100), health.LastFrameCount)
	require.False(t, health.LastSeen.IsZero())
}

func TestStreamStateString(t *testing.T) {
	require.Equal(t, "unknown", StreamStateUnknown.String())
	require.Equal(t, "healthy", StreamStateHealthy.String())
	require.Equal(t, "failing", StreamStateFailing.String())
	require.Equal(t, "failed", StreamStateFailed.String())
	require.Equal(t, "fallback", StreamStateFallback.String())
}

func TestIsInFallback(t *testing.T) {
	ffmpeg, err := getDummyFFmpeg()
	require.NoError(t, err)

	config := getDummyFallbackConfig()
	
	monitor, err := New(Config{
		InputID:        "test_input",
		FallbackConfig: config,
		FFmpeg:         ffmpeg,
		Logger:         log.New("test"),
	})
	
	require.NoError(t, err)
	require.False(t, monitor.IsInFallback())

	// Simulate fallback state
	monitor.healthLock.Lock()
	monitor.health.InFallback = true
	monitor.healthLock.Unlock()

	require.True(t, monitor.IsInFallback())
}

func TestBuildFallbackCommand(t *testing.T) {
	ffmpeg, err := getDummyFFmpeg()
	require.NoError(t, err)

	config := getDummyFallbackConfig()
	
	monitor, err := New(Config{
		InputID:        "test_input",
		FallbackConfig: config,
		FFmpeg:         ffmpeg,
		Logger:         log.New("test"),
	})
	
	require.NoError(t, err)

	// Test image source
	imageSource := &app.ConfigFallbackSource{
		Type:    "image",
		Address: "/tmp/test.png",
		Options: []string{"-f", "lavfi"},
		Loop:    true,
	}
	
	command := monitor.buildFallbackCommand(imageSource)
	require.Contains(t, command, "-f")
	require.Contains(t, command, "lavfi")
	require.Contains(t, command, "-loop")
	require.Contains(t, command, "1")
	require.Contains(t, command, "-i")
	require.Contains(t, command, "/tmp/test.png")
	
	// Check that the command contains anullsrc (it will be in one of the parameters)
	found := false
	for _, arg := range command {
		if strings.Contains(arg, "anullsrc") {
			found = true
			break
		}
	}
	require.True(t, found, "Command should contain anullsrc for audio generation")

	// Test video source with loop
	videoSource := &app.ConfigFallbackSource{
		Type:    "video",
		Address: "/tmp/test.mp4",
		Options: []string{"-re"},
		Loop:    true,
	}
	
	command = monitor.buildFallbackCommand(videoSource)
	require.Contains(t, command, "-re")
	require.Contains(t, command, "-stream_loop")
	require.Contains(t, command, "-1")
	require.Contains(t, command, "-i")
	require.Contains(t, command, "/tmp/test.mp4")

	// Test RTMP source
	rtmpSource := &app.ConfigFallbackSource{
		Type:    "rtmp",
		Address: "rtmp://example.com/live/stream",
		Options: []string{"-f", "flv"},
	}
	
	command = monitor.buildFallbackCommand(rtmpSource)
	require.Contains(t, command, "-f")
	require.Contains(t, command, "flv")
	require.Contains(t, command, "-i")
	require.Contains(t, command, "rtmp://example.com/live/stream")
}

func TestStartStop(t *testing.T) {
	ffmpeg, err := getDummyFFmpeg()
	require.NoError(t, err)

	// Test with disabled config
	disabledConfig := &app.ConfigFallback{
		Enabled: false,
	}
	
	monitor, err := New(Config{
		InputID:        "test_input",
		FallbackConfig: disabledConfig,
		FFmpeg:         ffmpeg,
		Logger:         log.New("test"),
	})
	
	require.NoError(t, err)
	
	// Start should do nothing for disabled monitor
	monitor.Start()
	monitor.Stop()

	// Test with enabled config
	config := getDummyFallbackConfig()
	
	monitor, err = New(Config{
		InputID:        "test_input",
		FallbackConfig: config,
		FFmpeg:         ffmpeg,
		Logger:         log.New("test"),
	})
	
	require.NoError(t, err)

	// Start monitoring
	monitor.Start()
	
	// Give it a moment to start
	time.Sleep(200 * time.Millisecond)
	
	// Stop monitoring
	monitor.Stop()
}

func TestCallbacks(t *testing.T) {
	ffmpeg, err := getDummyFFmpeg()
	require.NoError(t, err)

	config := getDummyFallbackConfig()
	
	var (
		fallbackCalled bool
		recoveryCalled bool
		failureCalled  bool
	)
	
	monitor, err := New(Config{
		InputID:        "test_input",
		FallbackConfig: config,
		FFmpeg:         ffmpeg,
		Logger:         log.New("test"),
		OnFallbackSwitch: func(inputID string, fallbackIndex int, source *app.ConfigFallbackSource) {
			fallbackCalled = true
			require.Equal(t, "test_input", inputID)
			require.Equal(t, 0, fallbackIndex)
			require.NotNil(t, source)
		},
		OnRecovery: func(inputID string) {
			recoveryCalled = true
			require.Equal(t, "test_input", inputID)
		},
		OnFailure: func(inputID string, reason string) {
			failureCalled = true
			require.Equal(t, "test_input", inputID)
			require.NotEmpty(t, reason)
		},
	})
	
	require.NoError(t, err)

	// Simulate failure triggering callback
	if monitor.onFailure != nil {
		monitor.onFailure("test_input", "test failure")
	}
	require.True(t, failureCalled)

	// Simulate fallback triggering callback
	if monitor.onFallbackSwitch != nil {
		monitor.onFallbackSwitch("test_input", 0, &config.Sources[0])
	}
	require.True(t, fallbackCalled)

	// Simulate recovery triggering callback
	if monitor.onRecovery != nil {
		monitor.onRecovery("test_input")
	}
	require.True(t, recoveryCalled)
}