// Accordion functionality
export function initializeAccordion() {
    const accordionSections = document.querySelectorAll('.accordion-section');
    // Remove active class from all sections on initialization
    accordionSections.forEach(section => {
        section.classList.remove('active');
    });
    accordionSections.forEach(section => {
        const header = section.querySelector('.accordion-header');
        header.addEventListener('click', () => {
            // Close all other sections
            accordionSections.forEach(otherSection => {
                if (otherSection !== section) {
                    otherSection.classList.remove('active');
                }
            });
            // Toggle current section
            section.classList.toggle('active');
        });
    });
    // Update selected options in accordion headers
    function updateSelectedOption(section, value) {
        const header = section.querySelector('[data-js="selected-option"]');
        if (header) {
            // Extract base name before colon and before first slash
            // e.g., "ubuntu:24.04" -> "ubuntu", "opensuse/leap:15.6" -> "opensuse"
            const baseValue = value.split(':')[0].split('/')[0];
            const baseImageLabel = document.querySelector(`label[for="${baseValue}-option"]`);
            // Get the SVG icon from the selected option
            let selectedIcon = null;
            if (baseImageLabel) {
                // For kubernetes section, look for the icon inside the icon container
                if (section.dataset.section === 'kubernetes') {
                    const iconContainer = baseImageLabel.querySelector('[data-js="option-container"]');
                    if (iconContainer) {
                        selectedIcon = iconContainer.querySelector('img')?.cloneNode(true);
                    }
                } else {
                    selectedIcon = baseImageLabel.querySelector('img')?.cloneNode(true);
                }
            }
            // Clear existing content and add icon if available
            header.innerHTML = '';
            header.style.display = 'flex';
            header.style.alignItems = 'center';
            header.style.gap = '0.5rem';
            if (selectedIcon) {
                selectedIcon.classList.add('w-4', 'h-4');
                header.appendChild(selectedIcon);
            }
            // Add text content
            const textSpan = document.createElement('span');
            switch(section.dataset.section) {
                case 'base-image':
                    if (value === 'byoi') {
                        // For BYOI, show the text input value or a default message
                        const byoiInput = document.getElementById('byoi');
                        textSpan.textContent = byoiInput.value || 'Not set';
                    } else {
                        textSpan.textContent = baseImageLabel ? baseImageLabel.querySelector('[data-js="option-label"]').textContent : value;
                    }
                    break;
                case 'model':
                    const modelLabel = document.querySelector(`label[for="${value}-option"]`);
                    textSpan.textContent = modelLabel ? modelLabel.querySelector('[data-js="model-title"]').textContent : value;
                    break;
                case 'kubernetes':
                    const k8sLabel = document.querySelector(`label[for="${value}-option"]`);
                    textSpan.textContent = k8sLabel ? k8sLabel.querySelector('[data-js="option-label"]').textContent : value;
                    break;
                case 'kubernetes-release':
                    textSpan.textContent = value || 'Latest';
                    break;
                default:
                    textSpan.textContent = baseImageLabel ? baseImageLabel.querySelector('[data-js="option-label"]').textContent : value;
                    break;
            }
            header.appendChild(textSpan);
        }
    }
    // Initialize icons for all sections, including hidden ones
    accordionSections.forEach(section => {
        const selectedRadio = section.querySelector('input[type="radio"]:checked');
        if (selectedRadio) {
            const value = selectedRadio.id === 'byoi-option' ? 'byoi' : selectedRadio.value;
            updateSelectedOption(section, value);

            // Initialize kubernetes distro name for the default selection
            if (section.dataset.section === 'kubernetes') {
                const distroName = value.toUpperCase();
                document.querySelectorAll('#kubernetes_distro_name').forEach(span => {
                    span.textContent = distroName;
                });
            }
        }
    });
    // Add change event listeners to all radio inputs
    document.querySelectorAll('input[type="radio"]').forEach(radio => {
        radio.addEventListener('change', (e) => {
            if (e.target.checked) {
                const section = e.target.closest('.accordion-section');
                if (section) {
                    // For BYOI radio, use 'byoi' as value to trigger special handling
                    const value = e.target.id === 'byoi-option' ? 'byoi' : e.target.value;
                    updateSelectedOption(section, value);
                    // Only collapse if the change wasn't triggered by clicking the text input
                    if (!e.byoiTextInput) {
                        section.classList.remove('active');
                    }

                    // Update kubernetes distro name when distribution changes
                    if (section.dataset.section === 'kubernetes') {
                        const distroName = value.toUpperCase();
                        document.querySelectorAll('#kubernetes_distro_name').forEach(span => {
                            span.textContent = distroName;
                        });
                    }

                    // Special handling for variant selection
                    if (section.dataset.section === 'variant' && value === 'standard') {
                        // Show kubernetes fields
                        document.querySelectorAll('.kubernetes_fields').forEach(field => {
                            field.classList.remove('hidden');
                            // Initialize kubernetes distribution selection if it exists
                            const k8sRadio = field.querySelector('input[type="radio"]:checked');
                            if (k8sRadio) {
                                updateSelectedOption(field, k8sRadio.value);
                            }
                        });
                    } else if (section.dataset.section === 'variant' && value === 'core') {
                        // Hide kubernetes fields
                        document.querySelectorAll('.kubernetes_fields').forEach(field => {
                            field.classList.add('hidden');
                        });
                    }

                    // Special handling for architecture selection
                    if (section.dataset.section === 'architecture') {
                        const modelSection = document.querySelector('.accordion-section[data-section="model"]');
                        if (modelSection) {
                            const modelOptions = modelSection.querySelectorAll('.model-option');
                            modelOptions.forEach(option => {
                                if (value === 'amd64') {
                                    // For AMD64, hide ARM-specific models
                                    if (option.classList.contains('arm-only')) {
                                        option.classList.add('hidden');
                                        // Uncheck the radio if it was selected
                                        const radio = option.querySelector('input[type="radio"]');
                                        if (radio && radio.checked) {
                                            radio.checked = false;
                                            // Select the first visible model
                                            const firstVisibleModel = modelSection.querySelector('.model-option:not(.hidden) input[type="radio"]');
                                            if (firstVisibleModel) {
                                                firstVisibleModel.checked = true;
                                                updateSelectedOption(modelSection, firstVisibleModel.value);
                                            }
                                        }
                                    } else {
                                        option.classList.remove('hidden');
                                    }
                                } else if (value === 'arm64') {
                                    // For ARM64, show all models
                                    option.classList.remove('hidden');
                                }
                            });
                        }
                    }
                }
            }
        });
    });
    // Add change event listeners to text inputs
    document.querySelectorAll('input[type="text"]').forEach(input => {
        input.addEventListener('input', (e) => {
            const section = e.target.closest('.accordion-section');
            if (section) {
                // For BYOI text input, update if it's currently selected
                if (e.target.id === 'byoi') {
                    const byoiRadio = document.getElementById('byoi-option');
                    if (byoiRadio.checked) {
                        updateSelectedOption(section, 'byoi');
                    }
                } else if (e.target.id === 'version') {
                    // For version input, update the selected option directly
                    const header = section.querySelector('[data-js="selected-option"]');
                    if (header) {
                        header.textContent = e.target.value || 'Not set';
                    }
                } else {
                    updateSelectedOption(section, e.target.value);
                }
            }
        });
    });
    // Add special handling for BYOI text input
    const byoiInput = document.getElementById('byoi');
    if (byoiInput) {
        // When clicking or focusing on the BYOI input, select its radio button
        ['click', 'focus'].forEach(eventType => {
            byoiInput.addEventListener(eventType, (e) => {
                const byoiRadio = document.getElementById('byoi-option');
                if (!byoiRadio.checked) {
                    byoiRadio.checked = true;
                    // Create a custom event with a flag to indicate it was triggered by text input
                    const event = new Event('change');
                    event.byoiTextInput = true;
                    byoiRadio.dispatchEvent(event);
                }
            });
        });
    }
}