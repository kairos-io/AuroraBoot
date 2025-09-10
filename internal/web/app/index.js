import { createAccordionView } from './accordion-view.js';
import { createBuildsView } from './builds-view.js';
import { Build } from './models/index.js';
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
    
    // Parse hash to extract tab name (hash might contain query params like "builds?build=123")
    const hashParts = hash.split('?');
    const tabName = hashParts[0];
    
    // Set active tab from hash
    if (tabName === 'builds') {
      this.mainActiveTab = 'builds';
      // Auto-load builds when switching to builds tab
      if (this.builds.length === 0) {
        this.refreshBuilds();
      }
    } else {
      this.mainActiveTab = 'newbuild';
    }
    
    // Check for build ID in both search params and hash params
    let buildId = urlParams.get('build');
    if (!buildId && hashParts.length > 1) {
      // Build ID might be in hash query params (e.g., "builds?build=123")
      const hashParams = new URLSearchParams(hashParts[1]);
      buildId = hashParams.get('build');
    }
    
    // If there's a build ID and we're on builds tab, select it
    if (buildId && this.mainActiveTab === 'builds') {
      // Skip URL restoration if we just created this build to avoid race condition
      if (this.newBuildCreated === buildId) {
        return; // Let the form submission handler manage this build
      }
      
      // Store the build ID to be processed by the builds view
      this.pendingBuildId = buildId;
      
      // Auto-load builds immediately when there's a pending build ID
      if (this.builds.length === 0) {
        this.refreshBuilds();
      }
      
      // Process the pending build ID to open the modal
      this.processPendingBuildId();
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
      
      // Update URL without page reload first
      window.history.pushState({}, '', url);
      
      // Then sync filter state to URL if there's an active filter
      // Use setTimeout to ensure the hash change has been processed
      setTimeout(() => {
        if (this.syncFilterToURL) {
          this.syncFilterToURL();
        }
        // Also process any pending build ID when switching to builds tab
        this.processPendingBuildId();
      }, 0);
      
      return; // Early return to avoid the second pushState call below
    } else {
      url.hash = '';
      // Clear build-related parameters when going back to new build tab
      url.searchParams.delete('build');
      url.searchParams.delete('status');
      this.selectedBuild = null;
    }
    
    // Update URL without page reload
    window.history.pushState({}, '', url);
  },
  
  // Select a build and update URL - now opens modal instead
  async selectBuildWithUrl(build) {
    await this.openBuildModal(build);
    
    // Update URL to include build ID
    const url = new URL(window.location);
    url.searchParams.set('build', build.uuid);
    window.history.pushState({}, '', url);
  },
  
  // Process pending build ID to open modal
  async processPendingBuildId() {
    if (this.pendingBuildId) {
      const buildId = this.pendingBuildId;
      this.pendingBuildId = null; // Clear it to avoid reprocessing
      
      // Use a small delay to ensure builds are loaded
      setTimeout(() => {
        this.selectBuildById(buildId);
      }, 100);
    }
  },
  
  // Select build by ID (for URL restoration) - now opens modal
  async selectBuildById(buildId, retryCount = 0) {
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
          const buildData = await response.json();
          build = Build.fromApiResponse(buildData);
        } else if (response.status === 404 && retryCount < 3) {
          // Build might not be available yet (just created), retry after a short delay
          console.log(`Build ${buildId} not found, retrying... (${retryCount + 1}/3)`);
          setTimeout(() => {
            this.selectBuildById(buildId, retryCount + 1);
          }, 1000);
          return;
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
        
        // Set a flag to prevent URL restoration from interfering
        this.newBuildCreated = result.uuid;
        
        // Wait for builds to load, then find and open the new build
        setTimeout(() => {
          this.refreshBuilds().then(() => {
            const newBuild = this.builds.find(b => b.uuid === result.uuid);
            if (newBuild) {
              // Clear the flag before opening modal
              this.newBuildCreated = null;
              
              // Open modal without URL update to avoid race condition
              this.openBuildModal(newBuild, false);
              
              // Manually update URL after modal is open
              const url = new URL(window.location);
              url.searchParams.set('build', result.uuid);
              window.history.pushState({}, '', url);
            } else {
              // If build not found in list, try to fetch it directly with retry
              this.selectBuildById(result.uuid);
            }
          });
        }, 500);
      })
      .catch(error => {
        console.error('Error submitting form:', error);
        this.newBuildCreated = null;
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
    
    // Additional state for handling new build creation
    newBuildCreated: null,
    
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