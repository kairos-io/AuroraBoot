import { createAccordionView } from './accordion-view.js';
import { createBuildsView } from './builds-view.js';
import Alpine from 'alpinejs';



// URL Navigation component for tab management and persistence
const urlNavigation = () => {
  return {
    mainActiveTab: 'newbuild',
    
    init() {
      // Watch for tab changes
      this.$watch('mainActiveTab', (newTab) => {
        // Tab changed
      });
    
    // Parse URL on page load to restore state
    this.parseUrlAndSetState();
    
    // Also try again after a short delay to handle initialization timing
    setTimeout(() => {
      this.parseUrlAndSetState();
    }, 100);
    
    // Listen for browser back/forward navigation
    window.addEventListener('popstate', () => {
      this.parseUrlAndSetState();
    });
  },
  
  // Parse URL hash and query parameters to restore state
  parseUrlAndSetState() {
    const urlParams = new URLSearchParams(window.location.search);
    const hash = window.location.hash.substring(1); // Remove the # character
    
    // Set active tab from hash
    if (hash === 'builds') {
      this.mainActiveTab = 'builds';
      // Auto-load builds when switching to builds tab
      if (this.builds.length === 0) {
        this.refreshBuilds();
      }
    } else {
      this.mainActiveTab = 'newbuild';
    }
    
    // If there's a build ID in query params and we're on builds tab, select it
    const buildId = urlParams.get('build');
    if (buildId && this.mainActiveTab === 'builds') {
      // Store the build ID to be processed by the builds view
      this.pendingBuildId = buildId;
      
      // Auto-load builds immediately when there's a pending build ID
      if (this.builds.length === 0) {
        this.refreshBuilds();
      }
    }
  },
  
  // Switch to a specific tab and update URL
  switchToTab(tabName) {
    this.mainActiveTab = tabName;
    
    // Update URL hash
    const url = new URL(window.location);
    if (tabName === 'builds') {
      url.hash = 'builds';
      // Auto-load builds when switching to builds tab
      if (this.builds.length === 0) {
        this.refreshBuilds();
      }
    } else {
      url.hash = '';
      // Clear build selection when going back to new build tab
      url.searchParams.delete('build');
      this.selectedBuild = null;
    }
    
    // Update URL without page reload
    window.history.pushState({}, '', url);
  },
  
  // Select a build and update URL - now opens modal instead
  selectBuildWithUrl(build) {
    this.openBuildModal(build);
  },
  
  // Select build by ID (for URL restoration) - now opens modal
  async selectBuildById(buildId) {
    try {
      // Load builds list first if not loaded
      if (this.builds.length === 0) {
        await this.refreshBuilds();
      }
      
      // Try to find the build in the loaded list first
      let build = this.builds.find(b => b.uuid === buildId);
      
      // If not found in list, fetch it directly
      if (!build) {
        const response = await fetch(`/api/v1/builds/${buildId}`);
        if (response.ok) {
          build = await response.json();
        } else {
          console.error('Build not found:', buildId);
          // Remove invalid build ID from URL
          const url = new URL(window.location);
          url.searchParams.delete('build');
          window.history.replaceState({}, '', url);
          return;
        }
      }
      
      // Open the modal for the found build
      if (build) {
        this.openBuildModal(build);
      }
    } catch (error) {
      console.error('Error loading build:', error);
    }
  }
  };
};

// Form submission handler
const formSubmissionHandler = () => {
  return {
    // Initialize form submission communication
    init() {
    // Listen for form submission events from accordion component
    this.$watch('$store.formSubmission', (submission) => {
      if (submission && submission.shouldSubmit) {
        this.handleFormSubmission(submission.formData);
        // Reset the store flag
        Alpine.store('formSubmission', { shouldSubmit: false, formData: null });
      }
    });
  },

  // Handle form submission through Alpine.js communication
  handleFormSubmission(formData) {
    fetch('/start', {
      method: 'POST',
      body: formData
    }).then(response => response.json())
      .then(result => {
        // Switch to builds tab
        this.switchToTab('builds');
        
        // Wait for builds to load, then find and open the new build
        setTimeout(() => {
          this.refreshBuilds().then(() => {
            const newBuild = this.builds.find(b => b.uuid === result.uuid);
            if (newBuild) {
              this.openBuildModal(newBuild);
            }
          });
        }, 500);
      });
  }
  };
};

// Merged components function to avoid init() conflicts
const mergedComponents = () => {
  const accordion = createAccordionView();
  const builds = createBuildsView();
  const urlNav = urlNavigation();
  const formHandler = formSubmissionHandler();
  
  return {
    ...accordion,
    ...builds,
    ...urlNav,
    ...formHandler,
    
    // Combined init method that calls all individual init methods
    init() {
      // Call individual init methods in the right context
      if (accordion.init) accordion.init.call(this);
      if (builds.init) builds.init.call(this);
      if (urlNav.init) urlNav.init.call(this);
      if (formHandler.init) formHandler.init.call(this);
    }
  };
};

// Alpine.js component registration
Alpine.data('createAccordionView', createAccordionView);
Alpine.data('buildsView', createBuildsView);
Alpine.data('urlNavigation', urlNavigation);
Alpine.data('formSubmissionHandler', formSubmissionHandler);
Alpine.data('mergedComponents', mergedComponents);

// Initialize Alpine store for component communication
Alpine.store('formSubmission', { shouldSubmit: false, formData: null });

Alpine.start(); 