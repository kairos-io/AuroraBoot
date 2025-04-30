// Accordion functionality
export function initializeAccordion() {
    const accordionSections = document.querySelectorAll('.accordion-section');
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
        const header = section.querySelector('.selected-option');
        if (header) {
            // Extract base name before colon and before first slash
            // e.g., "ubuntu:24.04" -> "ubuntu", "opensuse/leap:15.6" -> "opensuse"
            const baseValue = value.split(':')[0].split('/')[0];
            const baseImageLabel = document.querySelector(`label[for="${baseValue}-option"]`);
            switch(section.dataset.section) {
                case 'base-image':
                    if (value === 'byoi') {
                        // For BYOI, show the text input value or a default message
                        const byoiInput = document.getElementById('byoi');
                        header.textContent = byoiInput.value || 'Not set';
                    } else {
                        header.textContent = baseImageLabel ? baseImageLabel.querySelector('.option-label').textContent : value;
                    }
                    break;
                case 'model':
                    const modelLabel = document.querySelector(`label[for="${value}-option"]`);
                    header.textContent = modelLabel ? modelLabel.querySelector('.model-title').textContent : value;
                    break;
                case 'kubernetes-release':
                    header.textContent = value || 'Latest';
                    break;
                default:
                    header.textContent = baseImageLabel ? baseImageLabel.querySelector('.option-label').textContent : value;
                    break;
            }
        }
    }
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