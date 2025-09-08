// Accordion View Component - UI implementation using the BuildForm model
// This is the "view" layer that renders the form as an accordion
import { createBuildForm, VALIDATION_ERRORS } from './build-form.js';

export function createAccordionView() {
    const buildForm = createBuildForm();
    
    return {
        // Import the BuildForm model
        ...buildForm,
        
        // Expose validation error constants for use in templates
        VALIDATION_ERRORS,

        // Accordion-specific state
        openSections: ['base-image'], // Start with base-image open
        
        // Initialize watchers for form data changes
        init() {
            // Watch for variant changes
            this.$watch('formData.variant', (newValue, oldValue) => {
                this.handleVariantChange();
            });
            
            // Watch for architecture changes
            this.$watch('formData.architecture', (newValue, oldValue) => {
                this.handleArchitectureChange();
            });
        },
        
        // Computed property for visible sections (avoids redundant filtering)
        get visibleSections() {
            return this.sections.filter(s => s.visible);
        },

        // Section definitions - data-driven approach
        sections: [
            {
                id: 'base-image',
                title: 'Base Image',
                type: 'text-input-with-buttons',
                formField: 'base_image',
                placeholder: 'your-repo.com/path:tag',
                required: true,
                description: 'Choose from predefined images below or enter a custom image. Other versions from the previously listed distributions should work. It\'s also possible that some derivatives work as long as they use systemd or openrc as init system.',
                dataKey: 'baseImages',
                gridCols: 'md:grid-cols-3',
                showIcon: true,
                visible: true,
                getSelectedLabel: 'getSelectedBaseImageLabel',
                getSelectedIcon: 'getSelectedBaseImageIcon'
            },
            {
                id: 'architecture',
                title: 'Architecture',
                type: 'radio-grid',
                dataKey: 'architectures',
                formField: 'architecture',
                gridCols: 'md:grid-cols-2',
                showIcon: true,
                visible: true,
                onChange: 'handleArchitectureChange',
                getSelectedLabel: 'getSelectedArchitectureLabel',
                getSelectedIcon: 'getSelectedArchitectureIcon'
            },
            {
                id: 'model',
                title: 'Model',
                type: 'radio-grid',
                dataKey: 'getCompatibleModels',
                formField: 'model',
                gridCols: 'md:grid-cols-2',
                showIcon: true,
                showDescription: true,
                customIcons: true, // Special handling for model icons
                visible: true,
                infoPopover: {
                    title: 'What is a model?',
                    content: 'Depending on the architecture you choose, you\'ll be able to select from different models available under that architecture. If you\'re not targeting a specific board like a Raspberry Pi and instead plan to install on generic hardware or a virtual machine, select Generic.'
                },
                getSelectedLabel: 'getSelectedModelLabel',
                getSelectedIcon: 'getSelectedModelIcon'
            },
            {
                id: 'variant',
                title: 'Variant',
                type: 'radio-grid',
                dataKey: 'variants',
                formField: 'variant',
                gridCols: 'md:grid-cols-2',
                showIcon: true,
                showDescription: true,
                visible: true,
                onChange: 'handleVariantChange',
                getSelectedLabel: 'getSelectedVariantLabel',
                getSelectedIcon: 'getSelectedVariantIcon'
            },
            {
                id: 'kubernetes',
                title: 'Kubernetes Distribution',
                type: 'radio-grid',
                dataKey: 'kubernetesDistributions',
                formField: 'kubernetes_distribution',
                gridCols: 'md:grid-cols-2',
                showIcon: true,
                visible: false, // Initially hidden
                conditional: {
                    dependsOn: 'variant',
                    showWhen: 'standard'
                },
                infoPopover: {
                    title: 'Provider Kairos',
                    content: 'The Kairos Factory uses a component called provider to extend the functionality of the core images. In the web version, it\'s only possible to use the default Provider Kairos, which has support for K3s and K0s distributions. There are other providers available by the community with different distributions but these don\'t include the p2p functionality.',
                    link: {
                        url: 'https://github.com/kairos-io/?q=provider&type=all&language=&sort=',
                        text: 'Read more'
                    }
                },
                getSelectedLabel: 'getSelectedK8sLabel',
                getSelectedIcon: 'getSelectedK8sIcon'
            },
            {
                id: 'kubernetes-release',
                title: 'Kubernetes Release',
                type: 'text-input',
                formField: 'kubernetes_release',
                placeholder: 'v1.32.0',
                visible: false, // Initially hidden
                conditional: {
                    dependsOn: 'variant',
                    showWhen: 'standard'
                },
                infoPopover: {
                    title: 'Kubernetes Versions',
                    content: `
                        <h3 class="font-semibold text-gray-900 dark:text-white">K3s</h3>
                        <p>K3s versions are the Kubernetes version plus the k3s patch version. For example, v1.32.0+k3s1 is the Kubernetes v1.32.0 plus the k3s patch v1.</p>
                        <h3 class="font-semibold text-gray-900 dark:text-white">K0s</h3>
                        <p>K0s versions are the Kubernetes version plus the k0s patch version. For example, v1.32.0+k0s.0 is the Kubernetes v1.32.0 plus the k0s patch v0.</p>
                        <h3 class="font-semibold text-gray-900 dark:text-white">Latest</h3>
                        <p>If no version is specified, the latest version of the selected Kubernetes distribution will be used.</p>
                    `
                },
                getSelectedLabel: 'getK8sReleaseLabel'
            },
            {
                id: 'version',
                title: 'Version',
                type: 'text-input',
                formField: 'version',
                placeholder: 'v0.1.0-alpha',
                required: true,
                visible: true,
                infoPopover: {
                    title: 'Semantic Versioning',
                    content: 'Kairos uses Semantic Versioning for its versioning scheme. This means that the version starts with the letter v followed by a three-part number, with the format MAJOR.MINOR.PATCH. The MAJOR version is incremented when there are breaking changes, the MINOR version is incremented when there are new features, and the PATCH version is incremented when there are bug fixes. Build numbers are also possible. Check the Semver website for more information.',
                    link: {
                        url: 'https://semver.org/',
                        text: 'Read more'
                    }
                },
                getSelectedLabel: 'getVersionLabel'
            },
            {
                id: 'configuration',
                title: 'Configuration',
                type: 'textarea',
                formField: 'cloud_config',
                placeholder: '#cloud-config',
                rows: 10,
                description: 'Paste your cloud-config.yaml here (optional):',
                visible: true,
                infoPopover: {
                    title: 'What is a cloud-config?',
                    content: 'A <code>cloud-config.yaml</code> file allows you to preconfigure your Kairos system with users, network, and more. It is applied at first boot. See the <a href="https://kairos.io/docs/architecture/cloud-init/" class="font-medium text-blue-600 underline dark:text-blue-500 hover:no-underline" target="_blank">Kairos documentation</a> for details and examples.'
                },
                getSelectedLabel: 'getConfigurationLabel'
            },
            {
                id: 'artifacts',
                title: 'Artifacts',
                type: 'checkbox-grid',
                dataKey: 'artifacts',
                formField: 'artifact_',
                gridCols: 'md:grid-cols-3',
                showIcon: true,
                showDescription: true,
                customDisplay: true, // Special handling for artifacts display
                visible: true,
                getSelectedLabel: 'getArtifactsLabel',
                getSelectedIcons: 'getSelectedArtifactIcons' // Multiple icons for artifacts
            }
        ],
        
        // Accordion methods
        toggleSection(sectionName) {
            if (this.openSections.includes(sectionName)) {
                this.openSections = this.openSections.filter(s => s !== sectionName);
            } else {
                // Close all other sections and open this one
                this.openSections = [sectionName];
            }
        },

        isSectionOpen(sectionName) {
            return this.openSections.includes(sectionName);
        },

        // Section rendering helpers
        getSectionData(section) {
            if (typeof section.dataKey === 'function') {
                return this[section.dataKey]();
            }
            return this[section.dataKey] || [];
        },



        getSelectedValue(section) {
            if (section.getSelectedLabel) {
                return this[section.getSelectedLabel]();
            }
            return 'Not selected';
        },

        getSelectedIcon(section) {
            if (section.getSelectedIcon) {
                return this[section.getSelectedIcon]();
            }
            return null;
        },

        getSelectedIcons(section) {
            if (section.getSelectedIcons) {
                return this[section.getSelectedIcons]();
            }
            return [];
        },


        // Clear validation errors to reset UI state
        clearValidationErrors() {
            this.lastValidationResult = null;
        },

        // Override form methods to add accordion behavior
        handleArchitectureChange() {
            // Call the parent method
            buildForm.handleArchitectureChange.call(this);
        },

        handleVariantChange() {
            // Call the parent method
            buildForm.handleVariantChange.call(this);
            
            // Force Alpine.js to detect the changes by creating new section objects with updated visibility
            this.sections = this.sections.map(section => {
                const newSection = { ...section };
                
                if (this.formData.variant === 'standard') {
                    if (section.id === 'kubernetes' || section.id === 'kubernetes-release') {
                        newSection.visible = true;
                    }
                } else {
                    if (section.id === 'kubernetes' || section.id === 'kubernetes-release') {
                        newSection.visible = false;
                    }
                }
                
                return newSection;
            });
            
            // Update open sections
            if (this.formData.variant === 'standard') {
                if (!this.openSections.includes('kubernetes')) {
                    this.openSections.push('kubernetes');
                }
                if (!this.openSections.includes('kubernetes-release')) {
                    this.openSections.push('kubernetes-release');
                }
            } else {
                this.openSections = this.openSections.filter(s => s !== 'kubernetes');
                this.openSections = this.openSections.filter(s => s !== 'kubernetes-release');
            }
        },

        // Form validation with accordion behavior (pure Alpine.js)
        validateForm() {
            const validation = buildForm.validateForm.call(this);
            
            // Store validation result for UI feedback
            this.lastValidationResult = validation;

            if (!validation.isValid) {
                // Open sections with errors and add error highlighting
                if (validation.errors.includes(VALIDATION_ERRORS.VERSION_REQUIRED)) {
                    if (!this.openSections.includes('version')) {
                        this.openSections.push('version');
                    }
                }
                if (validation.errors.includes(VALIDATION_ERRORS.KUBERNETES_DISTRIBUTION_REQUIRED)) {
                    if (!this.openSections.includes('kubernetes')) {
                        this.openSections.push('kubernetes');
                    }
                }


                // Focus on the first field with an error using Alpine.js refs
                this.$nextTick(() => {
                    setTimeout(() => {
                        this.focusFirstErrorField(validation.errors);
                    }, 300);
                });
            }
            
            return validation;
        },

        // Focus first error field using Alpine.js reactive patterns
        focusFirstErrorField(errors) {
            let targetField = null;

            if (errors.includes(VALIDATION_ERRORS.BASE_IMAGE_REQUIRED)) {
                targetField = this.$refs.baseImageField;
            } else if (errors.includes(VALIDATION_ERRORS.VERSION_REQUIRED)) {
                targetField = this.$refs.versionField;
            } else if (errors.includes(VALIDATION_ERRORS.KUBERNETES_DISTRIBUTION_REQUIRED)) {
                targetField = this.$refs.kubernetesFields?.[0];
            }

            if (targetField) {
                targetField.focus();
                targetField.scrollIntoView({ behavior: 'smooth', block: 'center' });
            }
        },

        // Handle form submission through Alpine.js store communication
        handleFormSubmit(event) {
            event.preventDefault();

            // Run validation before proceeding
            const validation = this.validateForm();

            if (!validation.isValid) {
                // Validation failed - accordion sections should already be opened by validateForm()
                // Visual feedback is handled by the accordion component
                return; // Don't proceed with form submission
            }

            // Communicate with modal component through Alpine store
            const formData = new FormData(event.target);
            this.$store.formSubmission = { shouldSubmit: true, formData: formData };
        },



        // Validation error checking for UI styling
        hasValidationError(fieldName) {
            const validation = this.getLastValidation ? this.getLastValidation() : null;
            if (!validation || validation.isValid) return false;

            // Check specific field errors
            if (fieldName === 'base_image') {
                return validation.errors.includes(VALIDATION_ERRORS.BASE_IMAGE_REQUIRED);
            }
            if (fieldName === 'version') {
                return validation.errors.includes(VALIDATION_ERRORS.VERSION_REQUIRED);
            }
            if (fieldName === 'kubernetes_distribution') {
                return validation.errors.includes(VALIDATION_ERRORS.KUBERNETES_DISTRIBUTION_REQUIRED);
            }

            return false;
        },

        // Store last validation result for UI feedback
        lastValidationResult: null,

        getLastValidation() {
            return this.lastValidationResult;
        },

        // Get appropriate error message for a field
        getValidationErrorMessage(fieldName) {
            if (fieldName === 'base_image') {
                return VALIDATION_ERRORS.BASE_IMAGE_REQUIRED;
            }
            if (fieldName === 'version') {
                return VALIDATION_ERRORS.VERSION_REQUIRED;
            }
            if (fieldName === 'kubernetes_distribution') {
                return VALIDATION_ERRORS.KUBERNETES_DISTRIBUTION_REQUIRED;
            }
            return '';
        }
    };
} 