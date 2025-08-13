import { createAccordionView } from './accordion-view.js';
import './alpine-3.14.8.js';
import Alpine from 'alpinejs';

// Alpine.js component registration
Alpine.data('createAccordionView', createAccordionView);

// Modal and build process state management
Alpine.data('buildModal', () => ({
  // Modal state
  isModalVisible: false,
  
  // Initialize keyboard listeners
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
  }
}));

Alpine.start();

document.addEventListener('DOMContentLoaded', () => {

  // Get the buildModal component instance once and reuse it
  let modalData = null;
  
  // Function to get modal data with retry
  function getModalData() {
    if (!modalData) {
      const modalElement = document.querySelector('[x-data*="buildModal"]');
      if (modalElement) {
        modalData = Alpine.$data(modalElement);
      }
    }
    return modalData;
  }
  
  // Wait for Alpine.js to initialize
  Alpine.nextTick(() => {
    modalData = getModalData();
  });

  const form = document.getElementById('process-form');
  if (form) {
    form.addEventListener('submit', function(event) {
      event.preventDefault(); // Always prevent default submission

      // Get the Alpine.js component instance to access validation and form data
      const alpineData = Alpine.$data(form);

      // Run validation before proceeding
      if (alpineData && typeof alpineData.validateForm === 'function') {
        const validation = alpineData.validateForm();

        if (!validation.isValid) {
          // Validation failed - accordion sections should already be opened by validateForm()
          // Visual feedback is handled by the accordion component
          return; // Don't proceed with form submission
        }
      }

      // Ensure we have the modalData reference
      modalData = getModalData();

      // Update Alpine.js state
      if (modalData) {
        modalData.isModalVisible = true;
        modalData.isBuilding = true;
        modalData.currentStep = -1; // Start with no step active (waiting for worker)
        modalData.isComplete = false;
        modalData.logs = '';
        modalData.downloads = [];
      }

      const formData = new FormData(event.target);
      
      fetch('/start', {
        method: 'POST',
        body: formData
      }).then(response => response.json())
        .then(result => {
          const socket = new WebSocket("ws://" + window.location.host + "/ws/" + result.uuid);
          
          socket.onmessage = function(event) {
            const message = event.data;
            if (!message.trim()) return; // Skip empty messages

            updateStatus(message);
            
            // Update Alpine.js logs state only
            modalData = getModalData();
            if (modalData) {
              const strippedMessage = message.replace(/\x1b\[[0-9;]*m/g, '');
              modalData.logs += strippedMessage + '\n';
            }
          };
          
          function updateStatus(message) {
            modalData = getModalData();
            if (message.includes("Waiting for worker to pick up the job")) {
              if (modalData) modalData.currentStep = -1;
              return;
            }
            if (message.includes("Building container image")) {
              if (modalData) modalData.currentStep = 0;
              return;
            }
            if (message.includes("Generating tarball")) {
              if (modalData) modalData.currentStep = 1;
              return;
            }
            if (message.includes("Generating raw image")) {
              if (modalData) modalData.currentStep = 2;
              return;
            }
            if (message.includes("Generating ISO")) {
              if (modalData) modalData.currentStep = 3;
              return;
            }
            if (message.includes("Uploading artifacts to server")) {
              if (modalData) modalData.currentStep = 4;
              return;
            }
          }



          socket.onclose = function() {
            // Update Alpine.js state
            modalData = getModalData();
            if (modalData) {
              modalData.isBuilding = false;
              modalData.isComplete = true;
              modalData.logs += "Process complete. Check the links above for downloads.\n";
            }

            // Fetch artifacts from the server
            fetch(`/api/v1/builds/${result.uuid}/artifacts`)
              .then(response => response.json())
              .then(artifacts => {
                // Update Alpine.js state
                modalData = getModalData();
                if (modalData) {
                  modalData.downloads = artifacts.map(artifact => ({
                    name: artifact.name,
                    fullUrl: `/builds/${result.uuid}/artifacts/${artifact.url}`,
                    description: artifact.description
                  }));
                }
              })
              .catch(error => {
                console.error('Error fetching artifacts:', error);
                
                // Update Alpine.js state
                modalData = getModalData();
                if (modalData) {
                  modalData.logs += "Error fetching download links. Please try refreshing the page.\n";
                }
              });
          };
        });
    });
  }
}); 