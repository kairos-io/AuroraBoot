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
        pendingBuildId: null, // For URL restoration
        
        // Modal state
        isModalVisible: false,
        modalSelectedBuild: null,
        modalLogs: '',
        modalArtifacts: [],
        modalIsStreamingLogs: false,
        modalIsBuilding: false,
        modalShowLogs: false,
        
        // WebSocket connections
        logsSocket: null,
        modalLogsSocket: null,
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

            // Watch for builds list changes to handle pending build ID
            this.$watch('builds', (newBuilds) => {
                if (newBuilds.length > 0 && this.pendingBuildId) {
                    const build = newBuilds.find(b => b.uuid === this.pendingBuildId);
                    if (build) {
                        this.openBuildModal(build);
                        this.pendingBuildId = null; // Clear after processing
                    } else {
                        // If not found in list, try to fetch it directly
                        this.selectBuildById(this.pendingBuildId);
                    }
                }
            });

            // Handle escape key to close modal
            const handleEscape = (event) => {
                if (event.key === 'Escape' && this.isModalVisible) {
                    this.closeModal();
                }
            };
            
            this.$watch('isModalVisible', (value) => {
                if (value) {
                    document.addEventListener('keydown', handleEscape);
                } else {
                    document.removeEventListener('keydown', handleEscape);
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

        // Modal methods
        openBuildModal(build) {
            this.modalSelectedBuild = build;
            this.isModalVisible = true;
            this.modalLogs = '';
            this.modalArtifacts = [];
            this.modalIsBuilding = ['queued', 'assigned', 'running'].includes(build.status);
            this.modalShowLogs = false;
            
            // Update URL with build ID (only if not from URL restoration)
            if (!window.location.search.includes(`build=${build.uuid}`)) {
                const url = new URL(window.location);
                url.hash = 'builds';
                url.searchParams.set('build', build.uuid);
                window.history.pushState({}, '', url);
            }
            
            // Load modal data
            this.loadModalBuildDetails();
            
            // Start log streaming if build is active
            if (this.modalIsBuilding) {
                this.startModalLogStreaming();
            }
        },

        closeModal() {
            this.isModalVisible = false;
            this.modalSelectedBuild = null;
            this.stopModalLogStreaming();
            this.modalLogs = '';
            this.modalArtifacts = [];
            this.modalIsBuilding = false;
            this.modalShowLogs = false;
            
            // Remove build ID from URL
            const url = new URL(window.location);
            url.searchParams.delete('build');
            window.history.pushState({}, '', url);
        },

        // Load additional build details for modal
        async loadModalBuildDetails() {
            if (!this.modalSelectedBuild) return;
            
            // Load artifacts
            try {
                const response = await fetch(`/api/v1/builds/${this.modalSelectedBuild.uuid}/artifacts`);
                if (response.ok) {
                    this.modalArtifacts = await response.json();
                } else {
                    this.modalArtifacts = [];
                }
            } catch (error) {
                console.error('Error loading modal artifacts:', error);
                this.modalArtifacts = [];
            }

            // Load logs for completed builds
            if (this.modalSelectedBuild.status === 'complete' || this.modalSelectedBuild.status === 'failed') {
                this.loadModalBuildLogs();
            }
        },

        // Load logs for completed builds
        async loadModalBuildLogs() {
            if (!this.modalSelectedBuild) return;
            
            try {
                const response = await fetch(`/api/v1/builds/${this.modalSelectedBuild.uuid}/logs`);
                if (response.ok) {
                    const logs = await response.text();
                    // Strip ANSI escape codes for display
                    this.modalLogs = logs.replace(/\x1b\[[0-9;]*m/g, '');
                }
            } catch (error) {
                console.error('Error loading build logs:', error);
                this.modalLogs = 'Error loading logs.';
            }
        },

        // Start streaming logs for active builds in modal
        startModalLogStreaming() {
            if (!this.modalSelectedBuild || this.modalIsStreamingLogs) return;
            
            try {
                const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
                const wsUrl = `${wsProtocol}//${window.location.host}/api/v1/builds/${this.modalSelectedBuild.uuid}/logs`;
                
                this.modalLogsSocket = new WebSocket(wsUrl);
                this.modalIsStreamingLogs = true;
                this.modalLogs = '';
                
                this.modalLogsSocket.onmessage = (event) => {
                    const message = event.data;
                    if (message.trim()) {
                        // Strip ANSI escape codes for display
                        const strippedMessage = message.replace(/\x1b\[[0-9;]*m/g, '');
                        this.modalLogs += strippedMessage;
                        
                        // Auto-scroll to bottom if logs are visible
                        if (this.modalShowLogs) {
                            this.$nextTick(() => {
                                const container = this.$refs.modalLogsContainer;
                                if (container) {
                                    container.scrollTop = container.scrollHeight;
                                }
                            });
                        }
                    }
                };
                
                this.modalLogsSocket.onclose = () => {
                    this.modalIsStreamingLogs = false;
                    this.modalIsBuilding = false;
                    this.modalLogs += '\n--- Build completed ---\n';
                    
                    // Refresh build status and artifacts
                    this.refreshBuilds();
                    this.loadModalBuildDetails();
                };
                
                this.modalLogsSocket.onerror = (error) => {
                    console.error('Modal WebSocket error:', error);
                    this.modalIsStreamingLogs = false;
                    this.modalLogs += '\n--- Connection error ---\n';
                };
                
            } catch (error) {
                console.error('Failed to start modal log streaming:', error);
                this.modalIsStreamingLogs = false;
            }
        },

        // Stop streaming logs for modal
        stopModalLogStreaming() {
            if (this.modalLogsSocket) {
                this.modalLogsSocket.close();
                this.modalLogsSocket = null;
            }
            this.modalIsStreamingLogs = false;
        },

        // Handle logs display toggle in modal
        onModalLogsToggle() {
            // Auto-scroll to bottom when showing logs
            if (this.modalShowLogs) {
                this.$nextTick(() => {
                    const container = this.$refs.modalLogsContainer;
                    if (container) {
                        container.scrollTop = container.scrollHeight;
                    }
                });
            }
        },

        // Get build configuration summary for modal
        getBuildSummary(build) {
            if (!build) return {};
            
            return {
                'Base Image': build.image || 'N/A',
                'Architecture': build.architecture || 'N/A',
                'Model': build.model || 'N/A',
                'Variant': build.variant || 'N/A',
                'Version': build.version || 'N/A',
                'Status': build.status || 'N/A',
                'Created': this.formatTime(build.created_at)
            };
        },

        // Select build by ID (for URL restoration)
        async selectBuildById(buildId) {
            try {
                const response = await fetch(`/api/v1/builds/${buildId}`);
                if (response.ok) {
                    const build = await response.json();
                    this.openBuildModal(build);
                    this.pendingBuildId = null; // Clear after processing
                } else {
                    console.error('Build not found:', buildId);
                    // Remove invalid build ID from URL
                    const url = new URL(window.location);
                    url.searchParams.delete('build');
                    window.history.replaceState({}, '', url);
                    this.pendingBuildId = null;
                }
            } catch (error) {
                console.error('Error loading build:', error);
                this.pendingBuildId = null;
            }
        },

        // Cleanup when component is destroyed
        destroy() {
            this.stopLogStreaming();
            this.stopModalLogStreaming();
            if (this.refreshInterval) {
                clearInterval(this.refreshInterval);
            }
        }
    };
}

// Register with Alpine.js
Alpine.data('buildsView', createBuildsView);
