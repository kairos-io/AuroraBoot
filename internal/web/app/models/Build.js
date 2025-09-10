// Build Model - Represents a build object with its properties and methods
// This provides a consistent interface for working with build data across the application

export class Build {
    constructor(data = {}) {
        // Core build properties
        this.uuid = data.uuid || '';
        this.image = data.image || '';
        this.architecture = data.architecture || '';
        this.model = data.model || '';
        this.variant = data.variant || '';
        this.version = data.version || '';
        this.status = data.status || 'queued';
        this.created_at = data.created_at || null;
        this.updated_at = data.updated_at || null;
        this.started_at = data.started_at || null;
        this.completed_at = data.completed_at || null;
        
        // Additional properties that might come from API
        this.config = data.config || {};
        this.error_message = data.error_message || '';
        
        // Cached data (loaded separately)
        this._artifacts = null;
        this._logs = null;
    }

    // Static factory method to create Build from API response
    static fromApiResponse(data) {
        return new Build(data);
    }

    // Static factory method to create multiple builds from API response
    static fromApiResponseArray(dataArray) {
        return (dataArray || []).map(data => Build.fromApiResponse(data));
    }

    // Getters for computed properties
    get title() {
        return `${this.variant} ${this.model} (${this.architecture})`;
    }

    get isActive() {
        return ['queued', 'assigned', 'running'].includes(this.status);
    }

    get isCompleted() {
        return ['complete', 'failed'].includes(this.status);
    }

    get isSuccess() {
        return this.status === 'complete';
    }

    get isFailure() {
        return this.status === 'failed';
    }

    get statusIndicatorClass() {
        const classes = {
            'queued': 'bg-gray-400',
            'assigned': 'bg-yellow-400',
            'running': 'bg-blue-500 animate-pulse',
            'complete': 'bg-green-500',
            'failed': 'bg-red-500'
        };
        return classes[this.status] || 'bg-gray-400';
    }

    get statusBadgeClass() {
        const classes = {
            'queued': 'inline-flex items-center px-3 py-1 rounded-lg text-xs font-medium uppercase tracking-wide bg-slate-700 text-slate-300 border border-slate-600',
            'assigned': 'inline-flex items-center px-3 py-1 rounded-lg text-xs font-medium uppercase tracking-wide bg-amber-800 text-amber-200 border border-amber-700',
            'running': 'inline-flex items-center px-3 py-1 rounded-lg text-xs font-medium uppercase tracking-wide bg-sky-800 text-sky-200 border border-sky-700',
            'complete': 'inline-flex items-center px-3 py-1 rounded-lg text-xs font-medium uppercase tracking-wide bg-emerald-800 text-emerald-200 border border-emerald-700',
            'failed': 'inline-flex items-center px-3 py-1 rounded-lg text-xs font-medium uppercase tracking-wide bg-rose-800 text-rose-200 border border-rose-700'
        };
        return classes[this.status] || 'inline-flex items-center px-3 py-1 rounded-lg text-xs font-medium uppercase tracking-wide bg-slate-700 text-slate-300 border border-slate-600';
    }

    // Time formatting
    getFormattedTime(timestamp) {
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
    }

    get formattedCreatedAt() {
        return this.getFormattedTime(this.created_at);
    }

    get formattedUpdatedAt() {
        return this.getFormattedTime(this.updated_at);
    }

    get formattedStartedAt() {
        return this.getFormattedTime(this.started_at);
    }

    get formattedCompletedAt() {
        return this.getFormattedTime(this.completed_at);
    }

    // Get build configuration summary
    get summary() {
        return {
            'Base Image': this.image || 'N/A',
            'Architecture': this.architecture || 'N/A',
            'Model': this.model || 'N/A',
            'Variant': this.variant || 'N/A',
            'Version': this.version || 'N/A',
            'Status': this.status || 'N/A',
            'Created': this.formattedCreatedAt
        };
    }

    // API endpoints for this build
    get artifactsUrl() {
        return `/api/v1/builds/${this.uuid}/artifacts`;
    }

    get logsUrl() {
        return `/api/v1/builds/${this.uuid}/logs`;
    }

    get logsWebSocketUrl() {
        const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        return `${wsProtocol}//${window.location.host}/api/v1/builds/${this.uuid}/logs`;
    }

    // Load artifacts for this build
    async loadArtifacts() {
        if (this._artifacts !== null) {
            return this._artifacts;
        }

        try {
            const response = await fetch(this.artifactsUrl);
            if (response.ok) {
                this._artifacts = await response.json();
            } else {
                this._artifacts = [];
            }
        } catch (error) {
            console.error('Error loading artifacts:', error);
            this._artifacts = [];
        }
        
        return this._artifacts;
    }

    // Note: Log loading is now handled via WebSocket streaming in BuildModal

    // Get cached artifacts (returns null if not loaded)
    get artifacts() {
        return this._artifacts;
    }

    // Get cached logs (returns null if not loaded)
    get logs() {
        return this._logs;
    }

    // Clear cached data
    clearCache() {
        this._artifacts = null;
        this._logs = null;
    }

    // Update build data from new API response
    update(data) {
        // Only update core properties, avoid getter-only properties like 'artifacts' and 'logs'
        const coreProperties = [
            'uuid', 'image', 'architecture', 'model', 'variant', 'version', 
            'status', 'created_at', 'updated_at', 'started_at', 'completed_at',
            'config', 'error_message', 'worker_id'
        ];
        
        const oldStatus = this.status;
        
        // Update only the core properties
        coreProperties.forEach(prop => {
            if (data.hasOwnProperty(prop)) {
                this[prop] = data[prop];
            }
        });
        
        // Clear cache if status changed to avoid stale data
        if (data.status && data.status !== oldStatus) {
            this.clearCache();
        }
    }

    // Convert to plain object (for API calls)
    toJSON() {
        const obj = {
            uuid: this.uuid,
            image: this.image,
            architecture: this.architecture,
            model: this.model,
            variant: this.variant,
            version: this.version,
            status: this.status,
            created_at: this.created_at,
            updated_at: this.updated_at,
            started_at: this.started_at,
            completed_at: this.completed_at,
            config: this.config,
            error_message: this.error_message
        };

        // Remove null/undefined values
        return Object.fromEntries(
            Object.entries(obj).filter(([_, value]) => value != null)
        );
    }
}
