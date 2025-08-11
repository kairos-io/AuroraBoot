// Accordion View Component - UI implementation using the BuildForm model
// This is the "view" layer that renders the form as an accordion
import { createBuildForm } from './build-form.js';

export function createAccordionView() {
    const buildForm = createBuildForm();
    
    return {
        // Import the BuildForm model
        ...buildForm,
        
        // Accordion-specific state
        openSections: new Set(['base-image']), // Start with base-image open
        
        // Accordion methods
        toggleSection(sectionName) {
            if (this.openSections.has(sectionName)) {
                this.openSections.delete(sectionName);
            } else {
                this.openSections.add(sectionName);
            }
        },

        isSectionOpen(sectionName) {
            return this.openSections.has(sectionName);
        },

        // Override form methods to add accordion behavior
        handleArchitectureChange() {
            // Call the parent method
            buildForm.handleArchitectureChange.call(this);
            
            // Auto-open model section when architecture changes
            this.openSections.add('model');
        },

        handleVariantChange() {
            // Call the parent method
            buildForm.handleVariantChange.call(this);
            
            // Show/hide Kubernetes sections based on variant
            if (this.formData.variant === 'standard') {
                this.openSections.add('kubernetes');
                this.openSections.add('kubernetes-release');
            } else {
                this.openSections.delete('kubernetes');
                this.openSections.delete('kubernetes-release');
            }
        },

        // Form validation with accordion behavior
        validateForm() {
            const validation = buildForm.validateForm.call(this);
            
            if (!validation.isValid) {
                // Open sections with errors
                if (validation.errors.includes('Version is required')) {
                    this.openSections.add('version');
                }
                if (validation.errors.includes('Kubernetes distribution is required for Standard variant')) {
                    this.openSections.add('kubernetes');
                }
                if (validation.errors.includes('Custom image URL is required for Bring Your Own Image')) {
                    this.openSections.add('base-image');
                }
            }
            
            return validation;
        },

        // Accordion-specific helper methods
        getSectionIcon(sectionName) {
            const icons = {
                'base-image': '📦',
                'architecture': '🏗️',
                'model': '🔧',
                'variant': '⚙️',
                'kubernetes': '☸️',
                'kubernetes-release': '📋',
                'version': '🏷️',
                'configuration': '⚙️',
                'artifacts': '📦'
            };
            return icons[sectionName] || '📄';
        },

        getSectionDescription(sectionName) {
            const descriptions = {
                'base-image': 'Choose your base operating system',
                'architecture': 'Select target architecture',
                'model': 'Choose hardware model',
                'variant': 'Select Kairos variant',
                'kubernetes': 'Choose Kubernetes distribution',
                'kubernetes-release': 'Specify Kubernetes version',
                'version': 'Set your image version',
                'configuration': 'Add cloud-init configuration',
                'artifacts': 'Select output formats'
            };
            return descriptions[sectionName] || '';
        },

        // Model-specific methods
        getSelectedModelLabel() {
            const selected = this.models.find(model => model.value === this.formData.model);
            return selected?.label || 'Not selected';
        }
    };
} 