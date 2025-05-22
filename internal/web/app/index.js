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
      document.getElementById('generic-option').checked = true;
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
  const kubernetesFields = document.querySelectorAll('.kubernetes_fields');
  const k3sOption = document.getElementById('k3s-option');
  const k0sOption = document.getElementById('k0s-option');
  const kubernetesDistroName = document.getElementById('kubernetes_distro_name');
  standardOption.addEventListener('change', function() {
    if (this.checked) {
      coreOption.checked = false;
      k3sOption.checked = true;
      kubernetesDistroName.innerText = 'K3s';
      kubernetesFields.forEach(function(element) {
        element.style.display = 'block';
      });
    }
  });
  coreOption.addEventListener('change', function() {
    if (this.checked) {
      standardOption.checked = false;
      kubernetesFields.forEach(function(element) {
        element.style.display = 'none';
      });
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
  });
  document.getElementById('process-form').addEventListener('submit', function(event) {
    event.preventDefault(); // Always prevent default submission
    // First check if the form is valid
    if (!event.target.checkValidity()) {
      // Find all invalid fields
      const invalidFields = event.target.querySelectorAll(':invalid');
      // Open accordion sections containing invalid fields
      invalidFields.forEach(field => {
        const section = field.closest('.accordion-section');
        if (section && !section.classList.contains('active')) {
          section.classList.add('active');
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
        const buildingContainerImage = document.getElementById('building-container-image');
        const generatingTarball = document.getElementById('generating-tarball');
        const generatingRawImage = document.getElementById('generating-raw-image');
        const generatingISO = document.getElementById('generating-iso');
        const generatingDownloadLinks = document.getElementById('generating-download-links');
        function updateStatus(message) {
          if (message.includes("Building container image")) {
            buildingContainerImage.classList.remove("hidden");
          } else if (message.includes("Generating tarball")) {
            generatingTarball.classList.remove("hidden");
            buildingContainerImage.querySelector('.spinner').classList.add("hidden");
            buildingContainerImage.querySelector('.done').classList.remove("hidden");
          } else if (message.includes("Generating raw image")) {
            generatingRawImage.classList.remove("hidden");
            generatingTarball.querySelector('.spinner').classList.add("hidden");
            generatingTarball.querySelector('.done').classList.remove("hidden");
          } else if (message.includes("Generating ISO")) {
            generatingISO.classList.remove("hidden");
            generatingRawImage.querySelector('.spinner').classList.add("hidden");
            generatingRawImage.querySelector('.done').classList.remove("hidden");
          } else if (message.includes("Uploading artifacts to server")) {
            generatingDownloadLinks.classList.remove("hidden");
            generatingISO.querySelector('.spinner').classList.add("hidden");
            generatingISO.querySelector('.done').classList.remove("hidden");
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
                linkElement.innerHTML += `<a href="${fullUrl}" target="_blank" class="text-white bg-blue-700 hover:bg-blue-800 focus:ring-4 focus:ring-blue-300 font-medium rounded-lg text-sm px-5 py-2.5 dark:bg-blue-600 dark:hover:bg-blue-700 focus:outline-none dark:focus:ring-blue-800">${artifact.name}</a>`;
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
});
