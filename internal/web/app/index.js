import { createAccordionView } from './accordion-view.js';
import Alpine from 'alpinejs';

// Alpine.js component registration
Alpine.data('createAccordionView', createAccordionView);

// Modal and build process state management with form communication
Alpine.data('buildModal', () => ({
  // Modal state
  isModalVisible: false,
  
  // Initialize keyboard listeners and form communication
  init() {
    // Handle escape key to close modal
    const handleEscape = (event) => {
      if (event.key === 'Escape' && this.isModalVisible) {
        // Only allow closing if build is not active
        if (!this.isBuilding) {
          this.resetBuildState();
        }
      }
    };
    
    this.$watch('isModalVisible', (value) => {
      if (value) {
        document.addEventListener('keydown', handleEscape);
      } else {
        document.removeEventListener('keydown', handleEscape);
      }
    });

    // Listen for form submission events from accordion component
    this.$watch('$store.formSubmission', (submission) => {
      if (submission && submission.shouldSubmit) {
        this.handleFormSubmission(submission.formData);
        // Reset the store flag
        Alpine.store('formSubmission', { shouldSubmit: false, formData: null });
      }
    });
  },
  
  // Build state
  currentStep: -1, // -1 = no step active, 0-4 = active steps
  isBuilding: false,
  isComplete: false,
  showLogs: false,
  logs: '',
  downloads: [],
  
  // Build steps
  statusSteps: [
    { id: 'building-container-image', name: 'Building container image' },
    { id: 'generating-tarball', name: 'Generating tarball' },
    { id: 'generating-raw-image', name: 'Generating raw image' },
    { id: 'generating-iso', name: 'Generating ISO' },
    { id: 'generating-download-links', name: 'Generating download links' }
  ],
  
  // Methods
  resetBuildState() {
    this.logs = '';
    this.downloads = [];
    this.currentStep = -1; // Reset to initial state (no step active)
    this.isBuilding = false;
    this.isComplete = false;
    this.isModalVisible = false;
    this.showLogs = false; // Also reset logs visibility
  },
  
  toggleLogs() {
    this.showLogs = !this.showLogs;
  },
  
  isStepActive(stepIndex) {
    return stepIndex === this.currentStep && this.isBuilding;
  },
  
  isStepComplete(stepIndex) {
    return stepIndex < this.currentStep || this.isComplete;
  },
  
  isStepVisible(stepIndex) {
    return (this.isBuilding && stepIndex <= this.currentStep) || 
           (this.isComplete && stepIndex < this.statusSteps.length);
  },

  // Handle form submission through Alpine.js communication
  handleFormSubmission(formData) {
    // Update Alpine.js state
    this.isModalVisible = true;
    this.isBuilding = true;
    this.currentStep = -1; // Start with no step active (waiting for worker)
    this.isComplete = false;
    this.logs = '';
    this.downloads = [];

    fetch('/start', {
      method: 'POST',
      body: formData
    }).then(response => response.json())
      .then(result => {
        const socket = new WebSocket("ws://" + window.location.host + "/ws/" + result.uuid);
        
        socket.onmessage = (event) => {
          const message = event.data;
          if (!message.trim()) return; // Skip empty messages

          this.updateStatus(message);
          
          // Update Alpine.js logs state
          const strippedMessage = message.replace(/\x1b\[[0-9;]*m/g, '');
          this.logs += strippedMessage + '\n';
        };
        
        socket.onclose = () => {
          // Update Alpine.js state
          this.isBuilding = false;
          this.isComplete = true;
          this.logs += "Process complete. Check the links above for downloads.\n";

          // Fetch artifacts from the server
          fetch(`/api/v1/builds/${result.uuid}/artifacts`)
            .then(response => response.json())
            .then(artifacts => {
              this.downloads = artifacts.map(artifact => ({
                name: artifact.name,
                fullUrl: `/builds/${result.uuid}/artifacts/${artifact.url}`,
                description: artifact.description
              }));
            })
            .catch(error => {
              console.error('Error fetching artifacts:', error);
              this.logs += "Error fetching download links. Please try refreshing the page.\n";
            });
        };
      });
  },

  updateStatus(message) {
    if (message.includes("Waiting for worker to pick up the job")) {
      this.currentStep = -1;
      return;
    }
    if (message.includes("Building container image")) {
      this.currentStep = 0;
      return;
    }
    if (message.includes("Generating tarball")) {
      this.currentStep = 1;
      return;
    }
    if (message.includes("Generating raw image")) {
      this.currentStep = 2;
      return;
    }
    if (message.includes("Generating ISO")) {
      this.currentStep = 3;
      return;
    }
    if (message.includes("Uploading artifacts to server")) {
      this.currentStep = 4;
      return;
    }
  }
}));

// Initialize Alpine store for component communication
Alpine.store('formSubmission', { shouldSubmit: false, formData: null });

Alpine.start(); 