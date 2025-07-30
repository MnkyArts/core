package restream

import (
	"testing"
	"time"

	"github.com/datarhei/core/v16/restream/app"

	"github.com/stretchr/testify/require"
)

func TestFallbackIntegration(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil)
	require.NoError(t, err)

	// Create a process with fallback configuration
	process := &app.Config{
		ID: "integration_test",
		Input: []app.ConfigIO{
			{
				ID:      "primary",
				Address: "testsrc=size=640x480:rate=1", // Very low rate for testing
				Options: []string{
					"-f",
					"lavfi",
					"-t", "5", // Only run for 5 seconds
				},
				Fallback: app.ConfigFallback{
					Enabled:              true,
					FailureThreshold:     1000, // 1 second
					SilenceThreshold:     2000, // 2 seconds
					RecoveryEnabled:      true,
					RecoveryThreshold:    1000, // 1 second
					CheckInterval:        100,  // 100ms for fast testing
					Sources: []app.ConfigFallbackSource{
						{
							Type:    "image",
							Address: "color=red:size=640x480:rate=1",
							Options: []string{"-f", "lavfi"},
							Loop:    true,
						},
					},
				},
			},
		},
		Output: []app.ConfigIO{
			{
				ID:      "test_output",
				Address: "-",
				Options: []string{
					"-codec",
					"copy",
					"-f",
					"null",
				},
			},
		},
		Options: []string{
			"-loglevel",
			"error", // Reduce noise in tests
		},
		Reconnect:      true,
		ReconnectDelay: 1,
		Autostart:      false,
		StaleTimeout:   0,
	}

	// Add the process
	err = rs.AddProcess(process)
	require.NoError(t, err)

	// Verify fallback status is available
	status, err := rs.GetFallbackStatus(process.ID)
	require.NoError(t, err)
	require.Contains(t, status, "primary")

	inputStatus, ok := status["primary"].(map[string]interface{})
	require.True(t, ok)
	require.True(t, inputStatus["enabled"].(bool))
	require.False(t, inputStatus["in_fallback"].(bool))

	// Start the process
	err = rs.StartProcess(process.ID)
	require.NoError(t, err)

	// Give it a moment to start and establish monitoring
	time.Sleep(2 * time.Second)

	// Check process state
	state, err := rs.GetProcessState(process.ID)
	require.NoError(t, err)
	require.Equal(t, "start", state.Order)

	// Verify fallback monitoring is working
	status, err = rs.GetFallbackStatus(process.ID)
	require.NoError(t, err)
	
	inputStatus, ok = status["primary"].(map[string]interface{})
	require.True(t, ok)
	require.True(t, inputStatus["enabled"].(bool))

	// Stop the process
	err = rs.StopProcess(process.ID)
	require.NoError(t, err)

	// Clean up
	err = rs.DeleteProcess(process.ID)
	require.NoError(t, err)
}

func TestFallbackProcessLifecycle(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil)
	require.NoError(t, err)

	process := getDummyProcessWithFallback()
	process.ID = "lifecycle_test"

	// Test adding process with fallback
	err = rs.AddProcess(process)
	require.NoError(t, err)

	// Check that the task has fallback monitors
	restreamer := rs.(*restream)
	restreamer.lock.RLock()
	task, exists := restreamer.tasks[process.ID]
	restreamer.lock.RUnlock()
	
	require.True(t, exists)
	require.NotNil(t, task.fallbackMonitors)
	require.Len(t, task.fallbackMonitors, 1) // Should have one monitor for the input

	// Test starting process starts fallback monitoring
	err = rs.StartProcess(process.ID)
	require.NoError(t, err)

	// Test stopping process stops fallback monitoring
	err = rs.StopProcess(process.ID)
	require.NoError(t, err)

	// Test deleting process cleans up fallback monitors
	err = rs.DeleteProcess(process.ID)
	require.NoError(t, err)
}

func TestFallbackConfigValidation(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil)
	require.NoError(t, err)

	// Test process with enabled fallback but no sources
	process := &app.Config{
		ID: "validation_test",
		Input: []app.ConfigIO{
			{
				ID:      "test_input",
				Address: "testsrc=size=320x240:rate=1",
				Options: []string{"-f", "lavfi"},
				Fallback: app.ConfigFallback{
					Enabled: true,
					Sources: []app.ConfigFallbackSource{}, // No sources
				},
			},
		},
		Output: []app.ConfigIO{
			{
				ID:      "test_output",
				Address: "-",
				Options: []string{"-codec", "copy", "-f", "null"},
			},
		},
		Options:   []string{"-loglevel", "error"},
		Autostart: false,
	}

	// Should still add the process (fallback monitor will warn about no sources)
	err = rs.AddProcess(process)
	require.NoError(t, err)

	// Check fallback status returns empty for input with no sources (monitor not created)
	status, err := rs.GetFallbackStatus(process.ID)
	require.NoError(t, err)
	// Should be empty since no fallback monitor was created due to empty sources
	require.Empty(t, status)

	// Clean up
	err = rs.DeleteProcess(process.ID)
	require.NoError(t, err)
}

func TestFallbackAPIIntegration(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil)
	require.NoError(t, err)

	// Test process without fallback
	normalProcess := getDummyProcess()
	normalProcess.ID = "no_fallback"
	
	err = rs.AddProcess(normalProcess)
	require.NoError(t, err)

	// Should return empty status for process without fallback
	status, err := rs.GetFallbackStatus(normalProcess.ID)
	require.NoError(t, err)
	require.Empty(t, status)

	// Test process with fallback
	fallbackProcess := getDummyProcessWithFallback()
	fallbackProcess.ID = "with_fallback"
	
	err = rs.AddProcess(fallbackProcess)
	require.NoError(t, err)

	// Should return status for process with fallback
	status, err = rs.GetFallbackStatus(fallbackProcess.ID)
	require.NoError(t, err)
	require.NotEmpty(t, status)
	require.Contains(t, status, "in")

	// Test unknown process ID
	_, err = rs.GetFallbackStatus("unknown_process")
	require.Error(t, err)
	require.Equal(t, ErrUnknownProcess, err)

	// Clean up
	err = rs.DeleteProcess(normalProcess.ID)
	require.NoError(t, err)
	err = rs.DeleteProcess(fallbackProcess.ID)
	require.NoError(t, err)
}