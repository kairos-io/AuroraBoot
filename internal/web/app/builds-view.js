// Builds View Controller - Manages the builds list and detail view
// This handles listing builds, real-time updates, log streaming, and artifact management

import Alpine from 'alpinejs';

export function createBuildsView() {
    return {
        // State
        builds: [],
        selectedBuild: null,
        loading: false,
        statusFilter: '',
        activeTab: 'builds', // Start with builds list tab
        logs: '',
        artifacts: [],
        isStreamingLogs: false,
        autoScroll: true,
        
        // WebSocket connections
        logsSocket: null,
        refreshInterval: null,

        // Initialize the view
        init() {
            // Only load builds when the builds tab is active, not on page load
            
            // Watch for build selection changes
            this.$watch('selectedBuild', (newBuild, oldBuild) => {
                if (newBuild) {
                    this.loadBuildDetails();
                }
            });
        },

        // Load builds list from API
        async loadBuilds() {
            this.loading = true;
            try {
                const params = new URLSearchParams();
                if (this.statusFilter) {
                    params.append('status', this.statusFilter);
                }
                
                const response = await fetch(`/api/v1/builds?${params}`);
                if (response.ok) {
                    const data = await response.json();
                    this.builds = data.builds || [];
                    
                    // If we have a selected build, update it with fresh data
                    if (this.selectedBuild) {
                        const updatedBuild = this.builds.find(b => b.uuid === this.selectedBuild.uuid);
                        if (updatedBuild) {
                            this.selectedBuild = updatedBuild;
                        }
                    }
                } else {
                    console.error('Failed to load builds:', response.statusText);
                }
            } catch (error) {
                console.error('Error loading builds:', error);
            } finally {
                this.loading = false;
            }
        },

        // Refresh builds (called by user action)
        async refreshBuilds() {
            await this.loadBuilds();
        },



        // Select a build and load its details
        selectBuild(build, switchToOverview = false) {
            this.selectedBuild = build;
            
            // Switch to overview tab only if explicitly requested
            if (switchToOverview) {
                this.activeTab = 'overview';
            }
            
            // Load build details immediately
            this.loadBuildDetails();
        },



        // Load additional build details (artifacts, etc.)
        async loadBuildDetails() {
            if (!this.selectedBuild) return;
            
            // Load artifacts
            try {
                const response = await fetch(`/api/v1/builds/${this.selectedBuild.uuid}/artifacts`);
                if (response.ok) {
                    this.artifacts = await response.json();
                } else {
                    this.artifacts = [];
                }
            } catch (error) {
                console.error('Error loading artifacts:', error);
                this.artifacts = [];
            }
        },

        // Start streaming logs for the selected build
        startLogStreaming() {
            if (!this.selectedBuild || this.isStreamingLogs) return;
            
            try {
                const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
                const wsUrl = `${wsProtocol}//${window.location.host}/api/v1/builds/${this.selectedBuild.uuid}/logs`;
                
                this.logsSocket = new WebSocket(wsUrl);
                this.isStreamingLogs = true;
                this.logs = '';
                
                this.logsSocket.onmessage = (event) => {
                    const message = event.data;
                    if (message.trim()) {
                        // Strip ANSI escape codes for display
                        const strippedMessage = message.replace(/\x1b\[[0-9;]*m/g, '');
                        this.logs += strippedMessage;
                        
                        // Auto-scroll to bottom if enabled
                        if (this.autoScroll) {
                            this.$nextTick(() => {
                                const container = this.$refs.logsContainer;
                                if (container) {
                                    container.scrollTop = container.scrollHeight;
                                }
                            });
                        }
                    }
                };
                
                this.logsSocket.onclose = () => {
                    this.isStreamingLogs = false;
                    this.logs += '\n--- Connection closed ---\n';
                };
                
                this.logsSocket.onerror = (error) => {
                    console.error('WebSocket error:', error);
                    this.isStreamingLogs = false;
                    this.logs += '\n--- Connection error ---\n';
                };
                
            } catch (error) {
                console.error('Failed to start log streaming:', error);
                this.isStreamingLogs = false;
            }
        },

        // Stop streaming logs
        stopLogStreaming() {
            if (this.logsSocket) {
                this.logsSocket.close();
                this.logsSocket = null;
            }
            this.isStreamingLogs = false;
        },

        // Clear logs display
        clearLogs() {
            this.logs = '';
        },

        // Utility functions for UI
        getBuildTitle(build) {
            return `${build.variant} ${build.model} (${build.architecture})`;
        },

        formatTime(timestamp) {
            if (!timestamp) return '';
            const date = new Date(timestamp);
            const now = new Date();
            const diffMs = now - date;
            const diffMins = Math.floor(diffMs / 60000);
            const diffHours = Math.floor(diffMins / 60);
            const diffDays = Math.floor(diffHours / 24);
            
            if (diffMins < 1) return 'just now';
            if (diffMins < 60) return `${diffMins}m ago`;
            if (diffHours < 24) return `${diffHours}h ago`;
            if (diffDays < 7) return `${diffDays}d ago`;
            
            return date.toLocaleDateString();
        },

        getStatusIndicatorClass(status) {
            const classes = {
                'queued': 'bg-gray-400',
                'assigned': 'bg-yellow-400',
                'running': 'bg-blue-500 animate-pulse',
                'complete': 'bg-green-500',
                'failed': 'bg-red-500'
            };
            return classes[status] || 'bg-gray-400';
        },

        getStatusBadgeClass(status) {
            const classes = {
                'queued': 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300',
                'assigned': 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-300',
                'running': 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-300',
                'complete': 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300',
                'failed': 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-300'
            };
            return classes[status] || 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300';
        },

        // Cleanup when component is destroyed
        destroy() {
            this.stopLogStreaming();
            if (this.refreshInterval) {
                clearInterval(this.refreshInterval);
            }
        }
    };
}

// Register with Alpine.js
Alpine.data('buildsView', createBuildsView);
