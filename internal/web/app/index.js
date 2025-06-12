import 'flowbite';
import { initializeAccordion } from './accordion.js';
document.addEventListener('DOMContentLoaded', () => {
  // Initialize accordion functionality
  initializeAccordion();
  const byoi = document.getElementById('byoi');
  // when byoi is clicked on, select the base_image radio button that has the value byoi
  byoi.addEventListener('click', function() {
    document.querySelector('input[value="byoi"]').checked = true;
  });
  const arm64 = document.getElementById('arm64-option');
  const amd64 = document.getElementById('amd64-option');
  const armOnlyElements = document.querySelectorAll('.arm-only');
  const modelOptions = document.querySelectorAll('.model-option input');
  arm64.addEventListener('change', function() {
    if (this.checked) {
      // uncheck amd64
      amd64.checked = false;
      // make all .arm-only elements visible
      armOnlyElements.forEach(function(element) {
        element.style.display = 'block';
      });
    }
  });
  amd64.addEventListener('change', function() {
    if (this.checked) {
      // uncheck arm64
      arm64.checked = false;
      // make all .arm-only elements invisible
      armOnlyElements.forEach(function(element) {
        element.style.display = 'none';
      });
      // uncheck all model options except for the generic one
      modelOptions.forEach(function(element) {
        // find the checkbox
        element.checked = false;
      });
      document.getElementById('generic-option').click();
    }
  });
  // whenever a model options changes, uncheck all other model options
  modelOptions.forEach(function(element) {
    element.addEventListener('change', function() {
      if (this.checked) {
        modelOptions.forEach(function(element) {
          if (element !== this) {
            element.checked = false;
          }
        }, this);
      }
    });
  });
  const coreOption = document.getElementById('core-option');
  const standardOption = document.getElementById('standard-option');
  // Get Kubernetes Distribution and Release accordion sections
  const k8sHeading = document.getElementById('accordion-heading-kubernetes');
  const k8sBody = document.getElementById('accordion-body-kubernetes');
  const k8sReleaseHeading = document.getElementById('accordion-heading-kubernetes-release');
  const k8sReleaseBody = document.getElementById('accordion-body-kubernetes-release');
  const k3sOption = document.getElementById('k3s-option');
  const k0sOption = document.getElementById('k0s-option');
  const kubernetesDistroName = document.getElementById('kubernetes_distro_name');

  // Helper to show/hide k8s fields
  function showK8sFields(show) {
    if (show) {
      k8sHeading.classList.remove('hidden');
      k8sBody.classList.remove('hidden');
      k8sReleaseHeading.classList.remove('hidden');
      k8sReleaseBody.classList.remove('hidden');
    } else {
      k8sHeading.classList.add('hidden');
      k8sBody.classList.add('hidden');
      k8sReleaseHeading.classList.add('hidden');
      k8sReleaseBody.classList.add('hidden');
    }
  }

  // On load, set correct visibility
  showK8sFields(standardOption.checked);

  standardOption.addEventListener('change', function() {
    if (this.checked) {
      coreOption.checked = false;
      k3sOption.checked = true;
      kubernetesDistroName.innerText = 'K3s';
      showK8sFields(true);
    }
  });
  coreOption.addEventListener('change', function() {
    if (this.checked) {
      standardOption.checked = false;
      showK8sFields(false);
    }
  });
  k3sOption.addEventListener('change', function() {
    if (this.checked) {
      k0sOption.checked = false;
      kubernetesDistroName.innerText = 'K3s';
    }
  });
  k0sOption.addEventListener('change', function() {
    if (this.checked) {
      k3sOption.checked = false;
      kubernetesDistroName.innerText = 'K0s';
    }
  });
  const logsToggle = document.getElementById('logs-toggle');
  const logs = document.getElementById('output');
  logsToggle.addEventListener('change', function() {
    if (this.checked) {
      logs.style.display = 'block';
    } else {
      logs.style.display = 'none';
    }
  });
  const restartButton = document.getElementById('restart-button');
  const staticModal = document.getElementById('static-modal');
  const modalBackdrop = document.getElementById('modal-backdrop');
  restartButton.addEventListener('click', function() {
    logs.innerHTML = "";
    const downloads = document.getElementById('downloads');
    downloads.style.display = 'none';
    const linkElement = document.getElementById('links');
    linkElement.innerHTML = "";
    modalBackdrop.classList.add("hidden");
    staticModal.classList.add("hidden");
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
  document.getElementById('process-form').addEventListener('submit', function(event) {
    event.preventDefault(); // Always prevent default submission

    // Check if version is missing
    const versionInput = document.getElementById('version');
    if (versionInput && versionInput.value.trim() === '') {
        // Open the Version accordion
        const versionHeader = document.getElementById('accordion-heading-version');
        const versionButton = versionHeader && versionHeader.querySelector('button');
        const versionBody = document.getElementById('accordion-body-version');
        if (versionButton && versionBody) {
            // Use Flowbite's collapse API if available
            if (window.collapse && typeof window.collapse.open === 'function') {
                window.collapse.open('accordion-body-version');
            } else {
                // Fallback: set aria-expanded and show the body
                versionButton.setAttribute('aria-expanded', 'true');
                versionBody.classList.remove('hidden');
            }
        }
        // Mark the field as required/invalid (Flowbite/Tailwind style)
        versionInput.classList.add('border-red-500', 'focus:border-red-500', 'focus:ring-red-500');
        // Optionally show a message
        versionInput.reportValidity();
        versionInput.focus();
        return;
    } else if (versionInput) {
        // Remove error style if present
        versionInput.classList.remove('border-red-500', 'focus:border-red-500', 'focus:ring-red-500');
    }

    // First check if the form is valid
    if (!event.target.checkValidity()) {
        // Find all invalid fields
        const invalidFields = event.target.querySelectorAll(':invalid');
        // Open accordion sections containing invalid fields
        invalidFields.forEach(field => {
            // Find the closest accordion body
            const accordionBody = field.closest('[id^="accordion-body-"]');
            if (accordionBody) {
                const accordionId = accordionBody.id;
                const accordionHeading = document.getElementById(accordionId.replace('body', 'heading'));
                const accordionButton = accordionHeading?.querySelector('button');
                if (accordionButton && accordionBody.classList.contains('hidden')) {
                    // Use Flowbite's collapse API if available
                    if (window.collapse && typeof window.collapse.open === 'function') {
                        window.collapse.open(accordionId);
                    } else {
                        // Fallback: set aria-expanded and show the body
                        accordionButton.setAttribute('aria-expanded', 'true');
                        accordionBody.classList.remove('hidden');
                    }
                }
            }
        });
        // Show validation message and focus on first invalid field
        const firstInvalid = invalidFields[0];
        if (firstInvalid) {
            firstInvalid.focus();
            firstInvalid.reportValidity();
        }
        return;
    }

    modalBackdrop.classList.remove("hidden");
    staticModal.classList.remove("hidden");
    const formData = new FormData(event.target);
    // The artifact checkboxes (artifact_raw, artifact_iso, artifact_tar) are included in FormData by default.
    // No extra JS is needed unless we want to enforce logic, but Raw is always checked+disabled in HTML.
    fetch('/start', {
      method: 'POST',
      body: formData
    }).then(response => response.json())
      .then(result => {
        const socket = new WebSocket("ws://" + window.location.host + "/ws/" + result.uuid);
        const outputElement = document.getElementById('output');
        const linkElement = document.getElementById('links');
        outputElement.innerHTML = "";
        socket.onmessage = function(event) {
          const message = event.data;
          if (!message.trim()) return; // Skip empty messages

          updateStatus(message);
          // Create a pre element to preserve whitespace and prevent wrapping
          const pre = document.createElement('pre');
          // Strip ANSI color codes using regex
          const strippedMessage = message.replace(/\x1b\[[0-9;]*m/g, '');
          pre.textContent = strippedMessage;
          outputElement.appendChild(pre);
          outputElement.scrollTop = outputElement.scrollHeight;

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
          const steps = Array.from(statusList.querySelectorAll('li'));
          const currentStep = document.getElementById(stepId);
          const currentStepIndex = steps.indexOf(currentStep);
          const previousStep = steps[currentStepIndex - 1];
          if (previousStep) {
            previousStep.querySelector('.spinner').classList.add('hidden');
            previousStep.querySelector('.done').classList.remove('hidden');
          }
        }

        socket.onclose = function() {
          restartButton.classList.remove("hidden");
          generatingDownloadLinks.querySelector('.spinner').classList.add("hidden");
          generatingDownloadLinks.querySelector('.done').classList.remove("hidden");
          outputElement.innerHTML += "Process complete. Check the links above for downloads.\n";
          outputElement.scrollTop = outputElement.scrollHeight;
          const downloads = document.getElementById('downloads');
          downloads.style.display = 'block';

          // Fetch artifacts from the server
          fetch(`/api/v1/builds/${result.uuid}/artifacts`)
            .then(response => response.json())
            .then(artifacts => {
              const linkElement = document.getElementById('links');
              linkElement.innerHTML = ""; // Clear existing links
              for (const artifact of artifacts) {
                const fullUrl = `/builds/${result.uuid}/artifacts/${artifact.url}`;
                linkElement.innerHTML += `
                  <a href="${fullUrl}" target="_blank" class="block text-white bg-blue-700 hover:bg-blue-800 focus:ring-4 focus:ring-blue-300 font-medium rounded-lg px-5 py-2.5 min-h-[8rem] h-full flex flex-col justify-between dark:bg-blue-600 dark:hover:bg-blue-700 focus:outline-none dark:focus:ring-blue-800">
                    <div class="text-lg font-semibold">${artifact.name}</div>
                    <div class="text-sm opacity-80">${artifact.description}</div>
                  </a>`;
              }
            })
            .catch(error => {
              console.error('Error fetching artifacts:', error);
              outputElement.innerHTML += "Error fetching download links. Please try refreshing the page.\n";
              outputElement.scrollTop = outputElement.scrollHeight;
            });
        };
      });
  });
  // --- Artifacts summary update logic ---
  function toggleArtifactIcon(checkboxId, iconId) {
    const icon = document.getElementById(iconId);
    if (document.getElementById(checkboxId).checked) {
      icon.classList.remove('hidden');
    } else {
      icon.classList.add('hidden');
    }
  }

  function updateArtifactsSummary() {
    toggleArtifactIcon('artifact-iso', 'iso-selected');
    toggleArtifactIcon('artifact-tar', 'tar-selected');
    toggleArtifactIcon('artifact-gcp', 'gcp-selected');
    toggleArtifactIcon('artifact-azure', 'azure-selected');
  }
  // Initial update
  updateArtifactsSummary();
  // Add listeners
  ['artifact-iso','artifact-tar','artifact-gcp','artifact-azure'].forEach(id => {
    document.getElementById(id).addEventListener('change', updateArtifactsSummary);
  });
});
