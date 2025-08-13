import 'flowbite';
import { createAccordionView } from './accordion-view.js';
import './alpine-3.14.8.js';
import Alpine from 'alpinejs';

// Make createAccordionView available globally for Alpine.js
window.createAccordionView = createAccordionView;

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

window.Alpine = Alpine;
Alpine.start();

// Initialize Flowbite components after Alpine.js starts
Alpine.nextTick(() => {
  if (typeof window.initFlowbite === 'function') {
    window.initFlowbite();
  }
});

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

      // Get the Alpine.js component instance to access validation
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
          const outputElement = document.getElementById('output');
          const linkElement = document.getElementById('links');
          
          if (outputElement) outputElement.innerHTML = "";
          
          socket.onmessage = function(event) {
            const message = event.data;
            if (!message.trim()) return; // Skip empty messages

            updateStatus(message);
            
            // Update Alpine.js logs state
            modalData = getModalData();
            if (modalData) {
              const strippedMessage = message.replace(/\x1b\[[0-9;]*m/g, '');
              modalData.logs += strippedMessage + '\n';
            }
            
            // Legacy fallback for output element
            const pre = document.createElement('pre');
            const strippedMessage = message.replace(/\x1b\[[0-9;]*m/g, '');
            pre.textContent = strippedMessage;
            if (outputElement) {
              outputElement.appendChild(pre);
              outputElement.scrollTop = outputElement.scrollHeight;
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
            
            // Legacy fallback
            if (outputElement) {
              outputElement.innerHTML += "Process complete. Check the links above for downloads.\n";
              outputElement.scrollTop = outputElement.scrollHeight;
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
                
                // Legacy fallback
                if (linkElement) {
                  linkElement.innerHTML = ""; // Clear existing links
                  for (const [i, artifact] of artifacts.entries()) {
                    const fullUrl = `/builds/${result.uuid}/artifacts/${artifact.url}`;
                    const popoverId = `artifact-popover-${i}`;
                    linkElement.innerHTML += `
                      <a href="${fullUrl}" target="_blank" class="block text-white bg-blue-700 hover:bg-blue-800 focus:ring-4 focus:ring-blue-300 font-small rounded-lg px-5 py-2.5 h-full flex flex-col justify-between dark:bg-blue-600 dark:hover:bg-blue-700 focus:outline-none dark:focus:ring-blue-800">
                        <div class="text-sm font-semibold flex items-center gap-2">
                          ${artifact.name}
                          <button type="button" data-popover-target="${popoverId}" data-popover-placement="bottom-end" tabindex="0" class="ml-2 align-middle">
                            <svg class="w-4 h-4 text-gray-200 hover:text-gray-100 dark:text-gray-400 dark:hover:text-gray-200" aria-hidden="true" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg"><path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-8-3a1 1 0 00-.867.5 1 1 0 11-1.731-1A3 3 0 0113 8a3.001 3.001 0 01-2 2.83V11a1 1 0 11-2 0v-1a1 1 0 011-1 1 1 0 100-2zm0 8a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd"></path></svg>
                            <span class="sr-only">Show information</span>
                          </button>
                        </div>
                      </a>
                      <div data-popover id="${popoverId}" role="tooltip" class="absolute z-10 invisible inline-block text-sm text-gray-500 transition-opacity duration-300 bg-white border border-gray-200 rounded-lg shadow-xs opacity-0 w-72 dark:bg-gray-800 dark:border-gray-600 dark:text-gray-400">
                        <div class="p-3 space-y-2">
                          <p>${artifact.description}</p>
                        </div>
                        <div data-popper-arrow></div>
                      </div>
                    `;
                  }
                  if (window.initPopovers) window.initPopovers();
                }
              })
              .catch(error => {
                console.error('Error fetching artifacts:', error);
                
                // Update Alpine.js state
                modalData = getModalData();
                if (modalData) {
                  modalData.logs += "Error fetching download links. Please try refreshing the page.\n";
                }
                
                // Legacy fallback
                if (outputElement) {
                  outputElement.innerHTML += "Error fetching download links. Please try refreshing the page.\n";
                  outputElement.scrollTop = outputElement.scrollHeight;
                }
              });
          };
        });
    });
  }
}); 