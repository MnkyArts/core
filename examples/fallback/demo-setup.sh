#!/bin/bash

# Fallback Streaming Demo Script
# This script demonstrates how to use the new fallback streaming system

echo "=== Datarhei Core Fallback Streaming System Demo ==="
echo ""

# Create demo assets directory
mkdir -p /tmp/demo/fallback
cd /tmp/demo

# Create a simple fallback image using ImageMagick (if available) or download one
if command -v convert >/dev/null 2>&1; then
    echo "Creating demo fallback image..."
    convert -size 1280x720 xc:darkblue \
        -gravity center -pointsize 48 -fill white \
        -annotate +0+0 "Stream Temporarily Unavailable\nPlease Stand By" \
        fallback/offline.png
else
    echo "ImageMagick not available, you'll need to provide your own fallback image at:"
    echo "/tmp/demo/fallback/offline.png"
fi

# Create demo process configuration
cat > demo-process.json << 'EOF'
{
  "id": "demo-fallback-stream",
  "reference": "fallback-demo",
  "input": [
    {
      "id": "primary_input",
      "address": "rtmp://localhost:1935/live/demo",
      "options": ["-f", "flv"],
      "fallback": {
        "enabled": true,
        "failure_threshold_ms": 3000,
        "silence_threshold_ms": 5000,
        "recovery_enabled": true,
        "recovery_threshold_ms": 5000,
        "check_interval_ms": 1000,
        "sources": [
          {
            "type": "image",
            "address": "/tmp/demo/fallback/offline.png",
            "options": ["-f", "lavfi"],
            "loop": true
          },
          {
            "type": "video",
            "address": "testsrc=size=1280x720:rate=25:duration=10",
            "options": ["-f", "lavfi", "-re"],
            "loop": true
          },
          {
            "type": "rtmp",
            "address": "rtmp://backup.example.com/live/backup-stream",
            "options": ["-f", "flv"]
          }
        ]
      }
    }
  ],
  "output": [
    {
      "id": "hls_output",
      "address": "/tmp/demo/hls/stream.m3u8",
      "options": [
        "-c:v", "libx264",
        "-preset", "veryfast",
        "-b:v", "2000k",
        "-c:a", "aac",
        "-b:a", "128k",
        "-f", "hls",
        "-hls_time", "6",
        "-hls_list_size", "10",
        "-hls_flags", "delete_segments"
      ]
    },
    {
      "id": "youtube_output",
      "address": "rtmp://a.rtmp.youtube.com/live2/YOUR_STREAM_KEY_HERE",
      "options": [
        "-c:v", "libx264",
        "-preset", "veryfast",
        "-b:v", "2500k",
        "-maxrate", "2500k",
        "-bufsize", "5000k",
        "-c:a", "aac",
        "-b:a", "128k",
        "-f", "flv"
      ]
    }
  ],
  "options": [
    "-loglevel", "info"
  ],
  "reconnect": true,
  "reconnect_delay_seconds": 10,
  "autostart": true
}
EOF

echo "Created demo process configuration: demo-process.json"
echo ""

# Create a test script for the API
cat > test-fallback-api.sh << 'EOF'
#!/bin/bash

CORE_URL="http://localhost:8080"
PROCESS_ID="demo-fallback-stream"

echo "=== Testing Fallback API ==="
echo ""

# Function to make API calls
api_call() {
    curl -s -H "Content-Type: application/json" \
         -H "Authorization: Bearer YOUR_API_TOKEN" \
         "$@"
}

echo "1. Adding the fallback-enabled process..."
response=$(api_call -X POST "$CORE_URL/api/v3/process" -d @demo-process.json)
echo "Response: $response"
echo ""

echo "2. Getting fallback status..."
api_call -X GET "$CORE_URL/api/v3/process/$PROCESS_ID/fallback" | jq '.'
echo ""

echo "3. Getting process state..."
api_call -X GET "$CORE_URL/api/v3/process/$PROCESS_ID/state" | jq '.order, .state'
echo ""

echo "4. Monitoring fallback status (check this periodically)..."
echo "curl -H 'Authorization: Bearer YOUR_TOKEN' $CORE_URL/api/v3/process/$PROCESS_ID/fallback"
echo ""

echo "5. To simulate stream failure, stop your RTMP input and watch the fallback activate"
echo "6. To test recovery, restart your RTMP input and observe automatic return to primary"
EOF

chmod +x test-fallback-api.sh

echo "Created API test script: test-fallback-api.sh"
echo ""

# Create directories for output
mkdir -p hls

# Create a simple HTML player for testing
cat > player.html << 'EOF'
<!DOCTYPE html>
<html>
<head>
    <title>Fallback Stream Demo Player</title>
    <script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        video { width: 100%; max-width: 800px; }
        .status { margin: 10px 0; padding: 10px; background: #f0f0f0; }
        .controls { margin: 10px 0; }
        button { margin: 5px; padding: 10px; }
    </style>
</head>
<body>
    <h1>Fallback Streaming Demo</h1>
    
    <video id="video" controls autoplay muted></video>
    
    <div class="status">
        <strong>Stream Status:</strong> <span id="status">Disconnected</span><br>
        <strong>Fallback Status:</strong> <span id="fallback-status">Unknown</span>
    </div>
    
    <div class="controls">
        <button onclick="loadStream()">Load Stream</button>
        <button onclick="checkFallbackStatus()">Check Fallback Status</button>
        <button onclick="refreshStatus()">Refresh Status</button>
    </div>

    <div id="info">
        <h3>Demo Instructions:</h3>
        <ol>
            <li>Start the Core server with the demo configuration</li>
            <li>Start streaming to <code>rtmp://localhost:1935/live/demo</code></li>
            <li>Click "Load Stream" to start playback</li>
            <li>Stop your RTMP stream to see fallback activation</li>
            <li>Restart your RTMP stream to see automatic recovery</li>
        </ol>
        
        <h3>Fallback Sources (in priority order):</h3>
        <ul>
            <li><strong>Static Image:</strong> "Stream Temporarily Unavailable" message</li>
            <li><strong>Test Video:</strong> Generated test pattern with audio</li>
            <li><strong>Backup RTMP:</strong> Alternative stream source</li>
        </ul>
    </div>

    <script>
        const video = document.getElementById('video');
        const statusEl = document.getElementById('status');
        const fallbackStatusEl = document.getElementById('fallback-status');
        
        function loadStream() {
            if (Hls.isSupported()) {
                const hls = new Hls();
                hls.loadSource('/tmp/demo/hls/stream.m3u8');
                hls.attachMedia(video);
                
                hls.on(Hls.Events.MANIFEST_PARSED, function() {
                    statusEl.textContent = 'Connected';
                    video.play();
                });
                
                hls.on(Hls.Events.ERROR, function(event, data) {
                    statusEl.textContent = 'Error: ' + data.type;
                });
            } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
                video.src = '/tmp/demo/hls/stream.m3u8';
                video.addEventListener('loadedmetadata', function() {
                    statusEl.textContent = 'Connected';
                });
            }
        }
        
        async function checkFallbackStatus() {
            try {
                const response = await fetch('http://localhost:8080/api/v3/process/demo-fallback-stream/fallback');
                const data = await response.json();
                
                if (data.primary_input) {
                    const status = data.primary_input;
                    fallbackStatusEl.innerHTML = `
                        State: ${status.state}<br>
                        In Fallback: ${status.in_fallback}<br>
                        Failures: ${status.consecutive_failures}
                    `;
                } else {
                    fallbackStatusEl.textContent = 'No fallback data';
                }
            } catch (error) {
                fallbackStatusEl.textContent = 'Error: ' + error.message;
            }
        }
        
        function refreshStatus() {
            checkFallbackStatus();
        }
        
        // Auto-refresh fallback status every 5 seconds
        setInterval(checkFallbackStatus, 5000);
    </script>
</body>
</html>
EOF

echo "Created demo player: player.html"
echo ""

echo "=== Demo Setup Complete ==="
echo ""
echo "Files created:"
echo "  - demo-process.json       : Process configuration with fallback"
echo "  - test-fallback-api.sh    : API testing script"
echo "  - player.html             : HTML5 player for testing"
echo "  - fallback/offline.png    : Fallback image (if ImageMagick available)"
echo ""
echo "To run the demo:"
echo "1. Start datarhei Core:"
echo "   ./core"
echo ""
echo "2. Add the demo process (replace YOUR_API_TOKEN):"
echo "   curl -H 'Content-Type: application/json' \\"
echo "        -H 'Authorization: Bearer YOUR_API_TOKEN' \\"
echo "        -X POST http://localhost:8080/api/v3/process \\"
echo "        -d @demo-process.json"
echo ""
echo "3. Start streaming to: rtmp://localhost:1935/live/demo"
echo ""
echo "4. Open player.html in a browser to monitor the stream"
echo ""
echo "5. Test fallback by stopping/starting your RTMP stream"
echo ""
echo "6. Monitor fallback status via API:"
echo "   curl -H 'Authorization: Bearer YOUR_TOKEN' \\"
echo "        http://localhost:8080/api/v3/process/demo-fallback-stream/fallback"
echo ""
echo "=== End of Demo Setup ==="