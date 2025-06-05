// Accordion functionality for Flowbite
export function initializeAccordion() {
    // Map section names to their input selectors and header IDs
    const sections = [
        {
            name: 'base-image',
            headerId: 'accordion-heading-base-image',
            inputSelector: 'input[name="base_image"]',
        },
        {
            name: 'architecture',
            headerId: 'accordion-heading-architecture',
            inputSelector: 'input[name="architecture"]',
        },
        {
            name: 'model',
            headerId: 'accordion-heading-model',
            inputSelector: 'input[name="model"]',
        },
        {
            name: 'variant',
            headerId: 'accordion-heading-variant',
            inputSelector: 'input[name="variant"]',
        },
        {
            name: 'kubernetes',
            headerId: 'accordion-heading-kubernetes',
            inputSelector: 'input[name="kubernetes_distribution"]',
        },
        {
            name: 'kubernetes-release',
            headerId: 'accordion-heading-kubernetes-release',
            inputSelector: 'input[name="kubernetes_release"]',
            isText: true,
        },
        {
            name: 'version',
            headerId: 'accordion-heading-version',
            inputSelector: 'input[name="version"]',
            isText: true,
        },
    ];

    // Helper to update a header with the selected value and logo
    function updateHeader(section) {
        const headerH2 = document.getElementById(section.headerId);
        if (!headerH2) return;
        const button = headerH2.querySelector('button');
        if (!button) return;
        // Find the span with the section name
        const nameSpan = button.querySelector('span');
        if (!nameSpan) return;
        // Remove any previous selected-value container
        let old = button.querySelector('.selected-value');
        if (old) old.remove();
        // Find the selected input
        let selectedValue = '';
        let selectedLabel = '';
        let selectedIcon = null;
        if (section.isText) {
            // For text input (kubernetes-release, version)
            const input = document.querySelector(section.inputSelector);
            if (input) {
                if (section.name === 'kubernetes-release') {
                    selectedLabel = input.value.trim() === '' ? 'Latest' : input.value;
                } else if (section.name === 'version') {
                    selectedLabel = input.value.trim() === '' ? 'Missing' : input.value;
                } else {
                    selectedLabel = input.value || input.placeholder || 'Not set';
                }
            }
        } else {
            const checked = document.querySelector(section.inputSelector + ':checked');
            if (checked) {
                selectedValue = checked.value;
                // Find the label for this input
                const label = button.closest('form')
                    ? button.closest('form').querySelector(`label[for="${checked.id}"]`)
                    : document.querySelector(`label[for="${checked.id}"]`);
                if (label) {
                    // Try to find the icon (img or svg) inside the label
                    const icon = label.querySelector('img,svg');
                    if (icon) {
                        selectedIcon = icon.cloneNode(true);
                        selectedIcon.classList.add('w-4', 'h-4');
                        selectedIcon.style.marginRight = '0.5rem';
                    }
                    // Try to find the label text
                    const labelText = label.querySelector('[data-js="option-label"], [data-js="model-title"]');
                    selectedLabel = labelText ? labelText.textContent : label.textContent.trim();
                } else {
                    selectedLabel = selectedValue;
                }
                // Special case for BYOI
                if (checked.id === 'byoi-option') {
                    const byoiInput = document.getElementById('byoi');
                    selectedLabel = byoiInput && byoiInput.value ? byoiInput.value : 'Not set';
                }
            }
        }
        // Create a container for the selected value
        const container = document.createElement('span');
        // Use ml-auto to push the value toward the center/right, and min-w-0 to allow text truncation if needed
        container.className = 'selected-value flex items-center gap-2 ml-auto min-w-0 text-gray-900 dark:text-white';
        if (selectedIcon) container.appendChild(selectedIcon);
        const textSpan = document.createElement('span');
        textSpan.textContent = selectedLabel;
        textSpan.className = 'truncate';
        container.appendChild(textSpan);
        // Insert before the SVG (accordion icon)
        const svg = button.querySelector('svg[data-accordion-icon]');
        if (svg) {
            button.insertBefore(container, svg);
        } else {
            button.appendChild(container);
        }
    }

    // Initialize all headers on page load
    sections.forEach(section => updateHeader(section));

    // Listen for changes on all relevant inputs
    sections.forEach(section => {
        if (section.isText) {
            // For text inputs, listen to input event
            const input = document.querySelector(section.inputSelector);
            if (input) {
                input.addEventListener('input', () => updateHeader(section));
            }
        } else {
            // For radio inputs, listen to change event on all radios in the group
            document.querySelectorAll(section.inputSelector).forEach(radio => {
                radio.addEventListener('change', () => updateHeader(section));
            });
            // Special case: BYOI text input
            if (section.name === 'base-image') {
                const byoiInput = document.getElementById('byoi');
                if (byoiInput) {
                    byoiInput.addEventListener('input', () => updateHeader(section));
                }
            }
        }
    });
}