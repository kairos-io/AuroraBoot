import 'flowbite';
import { createAccordionView } from './accordion-view.js';
import './alpine-3.14.8.js';
import Alpine from 'alpinejs';

// Make createAccordionView available globally for Alpine.js
window.createAccordionView = createAccordionView;

// Alpine.js component registration
Alpine.data('createAccordionView', createAccordionView);

window.Alpine = Alpine;
Alpine.start();

// Initialize Flowbite components after Alpine.js starts
Alpine.nextTick(() => {
  if (typeof window.initFlowbite === 'function') {
    window.initFlowbite();
  }
});

document.addEventListener('DOMContentLoaded', () => {
  // Alpine.js now handles all the form logic, so we only need to keep the modal and submission logic
  
  // Initialize Flowbite modal instance
  let flowbiteModal = null;
  if (window.Modal) {
    const modalElement = document.getElementById('static-modal');
    if (modalElement) {
      flowbiteModal = new window.Modal(modalElement, {
        backdrop: 'static',
        keyboard: false
      });
    }
  }
  
  const logsToggle = document.getElementById('logs-toggle');
  const logs = document.getElementById('output');
  
  if (logsToggle && logs) {
    logsToggle.addEventListener('change', function() {
      if (this.checked) {
        logs.style.display = 'block';
      } else {
        logs.style.display = 'none';
      }
    });
  }

  const restartButton = document.getElementById('restart-button');
  const staticModal = document.getElementById('static-modal');
  const modalBackdrop = document.getElementById('modal-backdrop');
  
  if (restartButton) {
    restartButton.addEventListener('click', function() {
      if (logs) logs.innerHTML = "";
      const downloads = document.getElementById('downloads');
      if (downloads) downloads.style.display = 'none';
      const linkElement = document.getElementById('links');
      if (linkElement) linkElement.innerHTML = "";
      // Hide the Flowbite modal
      if (flowbiteModal) {
        flowbiteModal.hide();
      } else {
        // Fallback to manual show/hide
        if (modalBackdrop) modalBackdrop.classList.add("hidden");
        if (staticModal) staticModal.classList.add("hidden");
      }
      restartButton.classList.add("hidden");
      
      document.querySelectorAll('.spinner').forEach(function(element) {
        element.classList.remove("hidden");
      });
      document.querySelectorAll('.done').forEach(function(element) {
        element.classList.add("hidden");
      });
      
      // Hide all status steps except the first one
      const statusSteps = [
        document.getElementById('building-container-image'),
        document.getElementById('generating-tarball'),
        document.getElementById('generating-raw-image'),
        document.getElementById('generating-iso'),
        document.getElementById('generating-download-links')
      ];
      
      statusSteps.forEach((el, idx) => {
        if (el) {
          if (idx === 0) {
            el.classList.remove('hidden'); // Only show the first step at the start
          } else {
            el.classList.add('hidden');
          }
          // Also reset spinners and checkmarks for all
          const spinner = el.querySelector('.spinner');
          const done = el.querySelector('.done');
          if (spinner) spinner.classList.remove('hidden');
          if (done) done.classList.add('hidden');
        }
      });
    });
  }

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

      // Show the Flowbite modal
      if (flowbiteModal) {
        flowbiteModal.show();
      } else {
        // Fallback to manual show/hide
        if (modalBackdrop) modalBackdrop.classList.remove("hidden");
        if (staticModal) staticModal.classList.remove("hidden");
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
            // Create a pre element to preserve whitespace and prevent wrapping
            const pre = document.createElement('pre');
            // Strip ANSI color codes using regex
            const strippedMessage = message.replace(/\x1b\[[0-9;]*m/g, '');
            pre.textContent = strippedMessage;
            if (outputElement) {
              outputElement.appendChild(pre);
              outputElement.scrollTop = outputElement.scrollHeight;
            }
          };

          const generatingDownloadLinks = document.getElementById('generating-download-links');
          
          function updateStatus(message) {
            if (message.includes("Waiting for worker to pick up the job")) {
              showStep("waiting-for-worker");
              return;
            }
            if (message.includes("Building container image")) {
              showStep("building-container-image");
              return;
            }
            if (message.includes("Generating tarball")) {
              showStep("generating-tarball");
            }
            if (message.includes("Generating raw image")) {
              showStep("generating-raw-image");
              return;
            }
            if (message.includes("Generating ISO")) {
              showStep("generating-iso");
              return;
            }
            if (message.includes("Generating AWS image")) {
              showStep("generating-aws-image");
              return;
            }
            if (message.includes("Generating GCP image")) {
              showStep("generating-gcp-image");
              return;
            }
            if (message.includes("Generating Azure image")) {
              showStep("generating-azure-image");
              return;
            }
            if (message.includes("Uploading artifacts to server")) {
              showStep("generating-download-links");
              return;
            }
          }

          function showStep(stepId) {
            const step = document.getElementById(stepId);
            if (step) {
              step.classList.remove('hidden');
            }
            // find the previous li in the status-list
            const statusList = document.querySelector('.status-list');
            if (statusList) {
              const steps = Array.from(statusList.querySelectorAll('li'));
              const currentStep = document.getElementById(stepId);
              const currentStepIndex = steps.indexOf(currentStep);
              // find all previous steps and hide their spinners and show their done icons
              for (let i = 0; i < currentStepIndex; i++) {
                const previousStep = steps[i];
                const spinner = previousStep.querySelector('.spinner');
                const done = previousStep.querySelector('.done');
                if (spinner) spinner.classList.add('hidden');
                if (done) done.classList.remove('hidden');
              }
            }
          }

          socket.onclose = function() {
            if (restartButton) restartButton.classList.remove("hidden");
            if (generatingDownloadLinks) {
              const spinner = generatingDownloadLinks.querySelector('.spinner');
              const done = generatingDownloadLinks.querySelector('.done');
              if (spinner) spinner.classList.add("hidden");
              if (done) done.classList.remove("hidden");
            }
            if (outputElement) {
              outputElement.innerHTML += "Process complete. Check the links above for downloads.\n";
              outputElement.scrollTop = outputElement.scrollHeight;
            }
            const downloads = document.getElementById('downloads');
            if (downloads) downloads.style.display = 'block';

            // Fetch artifacts from the server
            fetch(`/api/v1/builds/${result.uuid}/artifacts`)
              .then(response => response.json())
              .then(artifacts => {
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