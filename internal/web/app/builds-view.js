// Simple Alpine.js Builds View - Native Alpine.js patterns with class organization
import { Build } from './models/index.js';
import { BuildModal } from './components/BuildModal.js';

export function createBuildsView() {
    return {
        // State - just plain reactive properties
        builds: [],
        loading: false,
        statusFilter: '',
        
        // Modal instance - Alpine.js watches its properties automatically
        modal: new BuildModal(),
        
        // Auto-refresh
        refreshInterval: null,

        init() {
            // Initialize filter from URL parameters only if we're on the builds tab
            if (this.isBuildsTabActive()) {
                const urlParams = new URLSearchParams(window.location.search);
                const statusFromUrl = urlParams.get('status');
                if (statusFromUrl && ['queued', 'assigned', 'running', 'complete', 'failed'].includes(statusFromUrl)) {
                    this.statusFilter = statusFromUrl;
                }
            }
            
            this.refreshBuilds();
            this.refreshInterval = setInterval(() => this.refreshBuilds(), 5000);
            
            // Watch for logs toggle changes - WebSocket streaming handles all cases now
            this.$watch('modal.showLogs', (showLogs) => {
                if (showLogs && this.modal.build && !this.modal.isStreamingLogs) {
                    // Start streaming if not already active
                    this.modal.startLogStreaming();
                }
            });


            // Handle browser back/forward navigation
            window.addEventListener('popstate', () => {
                // Only update filter from URL if we're on the builds tab
                if (this.isBuildsTabActive()) {
                    const urlParams = new URLSearchParams(window.location.search);
                    const statusFromUrl = urlParams.get('status');
                    this.statusFilter = statusFromUrl && ['queued', 'assigned', 'running', 'complete', 'failed'].includes(statusFromUrl) ? statusFromUrl : '';
                } else {
                    // If we navigated away from builds tab, keep the current filter but don't read from URL
                    // This preserves the user's filter choice within the tab
                }
                // Refresh builds if we're on the builds tab
                if (this.isBuildsTabActive()) {
                    this.refreshBuilds();
                }
            });
        },

        // Load builds from API
        async refreshBuilds() {
            try {
                this.loading = true;
                // Build query string with status filter if set
                const params = new URLSearchParams();
                if (this.statusFilter) {
                    params.append('status', this.statusFilter);
                }
                const response = await fetch(`/api/v1/builds?${params.toString()}`);
                if (response.ok) {
                    const data = await response.json();
                    
                    // API returns: { builds: [...], total: number }
                    const buildsArray = data.builds || [];
                    this.builds = Build.fromApiResponseArray(buildsArray);
                }
            } catch (error) {
                console.error('Error loading builds:', error);
                this.builds = []; // Reset on error
            } finally {
                this.loading = false;
            }
        },

        // Open modal - delegate to modal class, Alpine.js handles reactivity
        async openBuildModal(build) {
            await this.modal.open(build);
        },

        // Close modal
        closeModal() {
            this.modal.close();
            
            // Remove build ID from URL
            const url = new URL(window.location);
            url.searchParams.delete('build');
            window.history.pushState({}, '', url);
        },


        // Handle WebSocket close in modal - refresh builds list
        async onModalWebSocketClose() {
            // Refresh build status via modal
            const refreshed = await this.modal.refreshBuild();
            if (refreshed) {
                // Refresh the builds list to show updated status
                this.refreshBuilds();
            }
        },

        // Scroll logs to bottom (called from Alpine.js template)
        scrollLogsToBottom() {
                            this.$nextTick(() => {
                                const container = this.$refs.modalLogsContainer;
                                if (container) {
                                    container.scrollTop = container.scrollHeight;
                                }
                            });
        },

        // Utility methods
        getBuildTitle(build) {
            return `${build.image} ${build.variant} (${build.architecture})`;
        },

        formatTime(timestamp) {
            return new Date(timestamp).toLocaleString();
        },

        getBuildSummary(build) {
            return {
                'Image': build.image,
                'Variant': build.variant,
                'Architecture': build.architecture,
                'Model': build.model,
                'Version': build.version,
                'Status': build.status,
                'Created': this.formatTime(build.created_at)
            };
        },

        getStatusIndicatorClass(build) {
            return build.statusIndicatorClass;
        },

        getStatusBadgeClass(build) {
            return build.statusBadgeClass;
        },

        // URL synchronization - tab-specific approach
        updateURLAndRefresh() {
            // Only update URL parameters when we're on the builds tab
            if (this.isBuildsTabActive()) {
                const url = new URL(window.location);
                
                if (this.statusFilter) {
                    url.searchParams.set('status', this.statusFilter);
                } else {
                    url.searchParams.delete('status');
                }
                
                // Update URL without triggering a page reload
                window.history.pushState({}, '', url);
            }
            
            // Always refresh builds with new filter regardless of URL update
            this.refreshBuilds();
        },

        // Check if we're currently on the builds tab
        isBuildsTabActive() {
            // Access the main app's activeTab state
            // This assumes the builds view is used within the main app context
            return window.location.hash === '#builds' || 
                   (window.location.hash === '' && window.location.pathname.includes('builds'));
        },

        // Sync filter state to URL - called when tab becomes active
        syncFilterToURL() {
            if (this.isBuildsTabActive() && this.statusFilter) {
                const url = new URL(window.location);
                url.searchParams.set('status', this.statusFilter);
                window.history.replaceState({}, '', url);
            }
        },

        // Cleanup
        destroy() {
            this.modal.close(); // This handles stopping log streaming
            if (this.refreshInterval) {
                clearInterval(this.refreshInterval);
            }
        }
    };
}