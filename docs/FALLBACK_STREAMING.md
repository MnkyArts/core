# Fallback Streaming System

The datarhei Core now includes a robust fallback streaming system that automatically switches to alternative input sources when the primary stream fails. This ensures uninterrupted streaming experience for viewers.

## Features

- **Real-time Stream Monitoring**: Continuously monitors primary input stream health
- **Automatic Fallback Switching**: Seamlessly switches to predefined fallback sources when the main stream fails
- **Multiple Fallback Types**: Supports static images, video files, and secondary RTMP streams
- **Automatic Recovery**: Detects when the primary stream is restored and switches back automatically
- **Configurable Thresholds**: All detection and recovery parameters are user-configurable
- **Logging & Observability**: Comprehensive logging of all fallback events

## Configuration

The fallback system is configured per input in the process configuration. Here's the structure:

```json
{
  "input": [
    {
      "id": "primary_input",
      "address": "rtmp://your-stream-source/live/stream",
      "fallback": {
        "enabled": true,
        "failure_threshold_ms": 5000,
        "silence_threshold_ms": 10000,
        "recovery_enabled": true,
        "recovery_threshold_ms": 10000,
        "check_interval_ms": 1000,
        "sources": [
          {
            "type": "image",
            "address": "/core/data/fallback_image.png",
            "options": ["-f", "lavfi"],
            "loop": true
          },
          {
            "type": "video", 
            "address": "/core/data/fallback_video.mp4",
            "options": ["-re"],
            "loop": true
          },
          {
            "type": "rtmp",
            "address": "rtmp://backup-server/live/backup-stream",
            "options": ["-f", "flv"]
          }
        ]
      }
    }
  ]
}
```

## Configuration Parameters

### Main Fallback Settings

- `enabled`: Boolean to enable/disable fallback functionality
- `failure_threshold_ms`: Time in milliseconds before considering stream failed (default: 5000)
- `silence_threshold_ms`: Time in milliseconds of silence before triggering fallback (default: 10000)
- `recovery_enabled`: Whether to automatically return to primary stream when recovered (default: true)
- `recovery_threshold_ms`: Time in milliseconds of stable primary stream before recovery (default: 10000)
- `check_interval_ms`: How often to check stream health in milliseconds (default: 1000)

### Fallback Sources

Sources are tried in order until one works. Each source supports:

- `type`: Type of fallback source ("image", "video", "rtmp")
- `address`: Path to file or RTMP URL
- `options`: FFmpeg options specific to this source
- `loop`: Whether to loop the content (for image/video types)

## Fallback Source Types

### 1. Static Image Fallback

Used to display a static "Stream will return soon" message:

```json
{
  "type": "image",
  "address": "/core/data/stream_offline.png",
  "options": ["-f", "lavfi"],
  "loop": true
}
```

### 2. Video File Fallback

Play a pre-recorded video (looped or single-play):

```json
{
  "type": "video",
  "address": "/core/data/standby_video.mp4", 
  "options": ["-re"],
  "loop": true
}
```

### 3. Secondary RTMP Stream

Switch to a backup RTMP stream:

```json
{
  "type": "rtmp",
  "address": "rtmp://backup.example.com/live/backup-stream",
  "options": ["-f", "flv"]
}
```

## Environment Variables

You can also configure fallback defaults via environment variables:

```bash
# Default failure threshold (milliseconds)
CORE_FALLBACK_FAILURE_THRESHOLD=5000

# Default silence threshold (milliseconds) 
CORE_FALLBACK_SILENCE_THRESHOLD=10000

# Default recovery threshold (milliseconds)
CORE_FALLBACK_RECOVERY_THRESHOLD=10000

# Default check interval (milliseconds)
CORE_FALLBACK_CHECK_INTERVAL=1000

# Enable fallback by default for all inputs
CORE_FALLBACK_ENABLED=true
```

## API Endpoints

### Get Fallback Status

```
GET /api/v3/process/{id}/fallback
```

Returns the current fallback status for all monitored inputs:

```json
{
  "primary_input": {
    "enabled": true,
    "state": "healthy",
    "last_seen": 1642781234,
    "last_failure": 1642781200,
    "failure_duration_ms": 0,
    "consecutive_failures": 0,
    "in_fallback": false,
    "current_fallback_index": -1
  }
}
```

## Stream States

- `unknown`: Initial state before monitoring begins
- `healthy`: Stream is operating normally
- `failing`: Stream issues detected but not yet failed
- `failed`: Stream has failed and fallback triggered
- `fallback`: Currently using fallback source

## Failure Detection

The system detects failures through multiple mechanisms:

1. **Frame Analysis**: Monitors frame count progression
2. **Connection Monitoring**: Tracks connection state
3. **Buffer Analysis**: Detects buffer underruns
4. **Silence Detection**: Identifies prolonged silence periods

## Best Practices

### 1. Fallback Source Preparation

- Prepare high-quality fallback images and videos
- Test fallback sources regularly
- Keep fallback content up-to-date
- Use appropriate bitrates and formats

### 2. Threshold Configuration

- Set `failure_threshold_ms` based on your network conditions
- Configure `recovery_threshold_ms` to avoid rapid switching
- Adjust `check_interval_ms` based on monitoring needs

### 3. Monitoring Setup

- Monitor fallback events through logs
- Set up alerts for repeated failures
- Track fallback usage analytics
- Test recovery procedures regularly

### 4. Multiple Fallback Sources

- Configure multiple fallback sources for redundancy
- Order sources by preference (primary fallback first)
- Include both local and remote sources
- Test all configured sources

## Logging

Fallback events are logged with different levels:

```
INFO: Switched to fallback source (type=image, address=/fallback.png)
WARN: Stream failure detected (reason=No frames received for 5.2s)
INFO: Primary stream appears to have recovered, switching back
ERROR: Failed to create fallback process (error details)
```

## Example Complete Configuration

```json
{
  "id": "live-stream",
  "input": [
    {
      "id": "primary",
      "address": "rtmp://live.example.com/stream/abc123",
      "options": ["-f", "flv"],
      "fallback": {
        "enabled": true,
        "failure_threshold_ms": 3000,
        "recovery_threshold_ms": 5000,
        "check_interval_ms": 500,
        "sources": [
          {
            "type": "image",
            "address": "/core/data/technical_difficulties.png",
            "loop": true
          },
          {
            "type": "video",
            "address": "/core/data/please_stand_by.mp4",
            "loop": true
          },
          {
            "type": "rtmp", 
            "address": "rtmp://backup.example.com/stream/backup123"
          }
        ]
      }
    }
  ],
  "output": [
    {
      "id": "youtube",
      "address": "rtmp://a.rtmp.youtube.com/live2/YOUR_STREAM_KEY",
      "options": ["-c:v", "libx264", "-c:a", "aac", "-f", "flv"]
    },
    {
      "id": "twitch", 
      "address": "rtmp://live.twitch.tv/live/YOUR_STREAM_KEY",
      "options": ["-c:v", "libx264", "-c:a", "aac", "-f", "flv"]
    }
  ],
  "options": ["-loglevel", "info"]
}
```

This configuration will:
1. Monitor the primary RTMP stream
2. Switch to a static image if stream fails for 3+ seconds
3. If image fails, try the standby video
4. If video fails, try the backup RTMP stream
5. Automatically return to primary when it recovers for 5+ seconds
6. Continue streaming to YouTube and Twitch throughout any failures

## Troubleshooting

### Common Issues

1. **Fallback not triggering**: Check failure thresholds and source configuration
2. **Rapid switching**: Adjust recovery threshold to prevent oscillation
3. **Fallback source errors**: Verify file paths and network connectivity
4. **High CPU usage**: Increase check interval or optimize fallback sources

### Debug Information

Enable debug logging to see detailed fallback operation:

```bash
CORE_LOG_LEVEL=debug ./core
```

This will show frame count updates, health checks, and state transitions in detail.