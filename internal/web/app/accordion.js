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
            switch(section.dataset.section) {
                case 'base-image':
                    const baseImageLabel = document.querySelector(`label[for="${value}-option"]`);
                    header.textContent = baseImageLabel ? baseImageLabel.querySelector('.text-m').textContent : value;
                    break;
                case 'model':
                    const modelLabel = document.querySelector(`label[for="${value}-option"]`);
                    header.textContent = modelLabel ? modelLabel.querySelector('.model-title').textContent : value;
                    break;
                case 'kubernetes-release':
                    header.textContent = value || 'Latest';
                    break;
                default:
                    header.textContent = value;
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
                    updateSelectedOption(section, e.target.value);
                    section.classList.remove('active');
                }
            }
        });
    });

    // Add change event listeners to text inputs
    document.querySelectorAll('input[type="text"]').forEach(input => {
        input.addEventListener('input', (e) => {
            const section = e.target.closest('.accordion-section');
            if (section) {
                updateSelectedOption(section, e.target.value);
            }
        });
    });
} 