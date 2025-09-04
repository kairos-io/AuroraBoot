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
            this.refreshBuilds();
            this.refreshInterval = setInterval(() => this.refreshBuilds(), 5000);
            
            // Watch for logs toggle changes to handle loading
            this.$watch('modal.showLogs', async (showLogs) => {
                if (showLogs && this.modal.build?.isCompleted && !this.modal.logs) {
                    try {
                        this.modal.logs = await this.modal.build.loadLogs();
                    } catch (error) {
                        console.error('Error loading completed build logs:', error);
                        this.modal.logs = 'Error loading logs for completed build.';
                    }
                }
            });
        },

        // Load builds from API
        async refreshBuilds() {
            try {
                this.loading = true;
                const response = await fetch('/api/v1/builds?');
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

        // Cleanup
        destroy() {
            this.modal.close(); // This handles stopping log streaming
            if (this.refreshInterval) {
                clearInterval(this.refreshInterval);
            }
        }
    };
}