// BuildModal - Simple class that works with Alpine.js reactivity
// Alpine.js handles all DOM updates automatically when these properties change

export class BuildModal {
    constructor() {
        // Reactive properties - Alpine.js watches these automatically
        this.isVisible = false;
        this.build = null;
        this.logs = '';
        this.artifacts = [];
        this.isStreamingLogs = false;
        this.showLogs = false;
        
        // WebSocket connection
        this.logsSocket = null;
    }

    // Open modal with a build
    async open(build) {
        // Stop any existing log streaming first
        this.stopLogStreaming();
        
        this.build = build;
        this.isVisible = true;
        this.showLogs = build.isCompleted; // Auto-show logs for completed builds
        this.logs = '';
        this.artifacts = [];
        
        // Load artifacts
        try {
            this.artifacts = await build.loadArtifacts();
        } catch (error) {
            console.error('Error loading artifacts:', error);
            this.artifacts = [];
        }
        
        // Handle logs based on build status
        if (build.status === 'running') {
            this.startLogStreaming();
        } else if (build.isCompleted) {
            try {
                this.logs = await build.loadLogs();
            } catch (error) {
                console.error('Error loading logs:', error);
                this.logs = 'Error loading logs.';
            }
        } else if (build.status === 'queued') {
            this.logs = 'Build is queued. Waiting for an available worker...\n';
        } else if (build.status === 'assigned') {
            this.logs = 'Build assigned to worker. Starting soon...\n';
        }
    }

    // Close modal
    close() {
        this.isVisible = false;
        this.stopLogStreaming();
        this.build = null;
        this.logs = '';
        this.artifacts = [];
    }


    // Start WebSocket log streaming for active builds
    startLogStreaming() {
        // Only start WebSocket for builds that are actually running (not just queued/assigned)
        if (!this.build?.isActive || this.isStreamingLogs || this.build.status !== 'running') {
            return;
        }
        
        // Stop any existing connection first
        this.stopLogStreaming();
        
        // Double-check we don't have an existing socket
        if (this.logsSocket) {
            return;
        }
        
        try {
            this.logsSocket = new WebSocket(this.build.logsWebSocketUrl);
            this.isStreamingLogs = true;
            this.logs = '';
            this.connectionClosed = false;
            this.lastMessageTime = Date.now();
            
            this.logsSocket.onopen = () => {
                this.connectionClosed = false;
            };
            
            this.logsSocket.onmessage = (event) => {
                const message = event.data;
                if (message.trim()) {
                    this.lastMessageTime = Date.now();
                    
                    // Strip ANSI escape codes for display
                    const strippedMessage = message.replace(/\x1b\[[0-9;]*m/g, '');
                    
                    // If we got a message after thinking the connection was closed,
                    // it means the close was a false alarm (network hiccup)
                    if (this.connectionClosed) {
                        this.logs += '\n--- Connection recovered ---\n';
                        this.connectionClosed = false;
                        this.isStreamingLogs = true;
                        // Clear any pending close timeout since connection is alive
                        if (this.closeTimeout) {
                            clearTimeout(this.closeTimeout);
                            this.closeTimeout = null;
                        }
                    }
                    
                    this.logs += strippedMessage;
                }
            };
            
            this.logsSocket.onclose = (event) => {
                
                // For abnormal closures (like 1006), wait a moment to see if more messages come
                // This handles Firefox's tendency to fire close events during network hiccups
                if (event.code === 1006) {
                    this.connectionClosed = true;
                    // Set a timeout to check if connection is truly dead
                    if (this.closeTimeout) {
                        clearTimeout(this.closeTimeout);
                    }
                    this.closeTimeout = setTimeout(() => {
                        // If we haven't received messages for 2 seconds after close, truly close
                        if (this.connectionClosed && Date.now() - this.lastMessageTime > 2000) {
                            this.isStreamingLogs = false;
                            this.logs += `\n--- Connection lost (network interruption) ---\n`;
                        }
                    }, 2000);
                } else {
                    // Clean close or other error codes - immediately stop streaming
                    this.isStreamingLogs = false;
                    this.connectionClosed = true;
                    if (event.wasClean && event.code === 1000) {
                        this.logs += '\n--- Build completed ---\n';
                    } else {
                        this.logs += `\n--- Connection closed (code: ${event.code}) ---\n`;
                    }
                }
            };
            
            this.logsSocket.onerror = (error) => {
                // Don't immediately show error - let onclose handle it
            };
            
        } catch (error) {
            this.isStreamingLogs = false;
            this.logs += '\n--- Failed to connect to log stream ---\n';
        }
    }

    // Stop log streaming
    stopLogStreaming() {
        if (this.logsSocket) {
            this.logsSocket.close();
            this.logsSocket = null;
        }
        if (this.closeTimeout) {
            clearTimeout(this.closeTimeout);
            this.closeTimeout = null;
        }
        this.isStreamingLogs = false;
        this.connectionClosed = false;
    }

    // Refresh build data from API
    async refreshBuild() {
        if (!this.build) return;
        
        try {
            const response = await fetch(`/api/v1/builds/${this.build.uuid}`);
            if (response.ok) {
                const buildData = await response.json();
                this.build.update(buildData);
                
                // Reload artifacts if build completed
                if (this.build.isCompleted) {
                    this.artifacts = await this.build.loadArtifacts();
                }
                
                return true; // Signal successful refresh
            }
        } catch (error) {
            console.error('Error refreshing build data:', error);
        }
        return false;
    }
}