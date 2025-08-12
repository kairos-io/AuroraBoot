// BuildForm Model - Contains all form logic and state management
// This is independent of the UI implementation (accordion, simple form, etc.)

// Validation error constants to avoid duplication
export const VALIDATION_ERRORS = {
    VERSION_REQUIRED: 'Version is required',
    KUBERNETES_DISTRIBUTION_REQUIRED: 'Kubernetes distribution is required for Standard variant',
    BYOI_IMAGE_REQUIRED: 'Custom image URL is required for Bring Your Own Image'
};

export function createBuildForm() {
    return {
        // Form data state
        formData: {
            base_image: 'ubuntu:24.04',
            architecture: 'amd64',
            model: 'generic',
            variant: 'core',
            kubernetes_distribution: 'k3s',
            kubernetes_release: '',
            version: '',
            cloud_config: '',
            byoi_image: '',
            artifact_raw: true,
            artifact_iso: true,
            artifact_tar: true,
            artifact_gcp: false,
            artifact_azure: false
        },

        // Available base images with their metadata
        baseImages: [
            { value: 'ubuntu:24.04', label: 'Ubuntu 24.04 LTS', icon: 'assets/img/ubuntu.svg' },
            { value: 'fedora:40', label: 'Fedora 40', icon: 'assets/img/fedora.svg' },
            { value: 'opensuse/leap:15.6', label: 'openSUSE Leap 15.6', icon: 'assets/img/opensuse.svg' },
            { value: 'debian:12', label: 'Debian 12 (Bookworm)', icon: 'assets/img/debian.svg' },
            { value: 'alpine:3.21', label: 'Alpine 3.21', icon: 'assets/img/alpine.svg' },
            { value: 'rockylinux:9', label: 'Rocky Linux 9', icon: 'assets/img/rockylinux.svg' },
            { value: 'byoi', label: 'Bring Your Own Image', icon: null, isCustom: true }
        ],

        // Available models with their compatible architectures
        models: [
            { value: 'generic', label: 'Generic', icon: 'assets/img/cd.svg', archs: ['amd64', 'arm64'], description: 'For generic boards and virtualization.' },
            { value: 'rpi3', label: 'Raspberry Pi 3', icon: 'assets/img/raspberry-pi.svg', archs: ['arm64'], description: 'For Raspberry Pi 3 boards' },
            { value: 'rpi4', label: 'Raspberry Pi 4', icon: 'assets/img/raspberry-pi.svg', archs: ['arm64'], description: 'For Raspberry Pi 4 boards' },
            { value: 'nvidia-agx-orin', label: 'Nvidia AGX Orin', icon: 'assets/img/nvidia.svg', archs: ['arm64'], description: 'For Nvidia AGX Orin boards.' }
        ],

        // Available architectures
        architectures: [
            { value: 'amd64', label: 'AMD64', icon: 'assets/img/amd.svg', description: 'For devices using the x86-64 (AMD64) architecture, commonly found in Intel and AMD processors.' },
            { value: 'arm64', label: 'ARM64', icon: 'assets/img/arm.svg', description: 'For devices using the 64-bit ARM (AArch64) architecture.' }
        ],

        // Available variants
        variants: [
            { value: 'core', label: 'Core', icon: 'assets/img/kairos-core.svg', description: 'Immutable, A/B Upgrades and cloud-init based configuration.' },
            { value: 'standard', label: 'Standard', icon: 'assets/img/kairos-standard.svg', description: 'Everything in Core plus Kubernetes, K9s and EdgeVPN included.' }
        ],

        // Available Kubernetes distributions
        kubernetesDistributions: [
            { value: 'k3s', label: 'K3s', icon: 'assets/img/k3s.svg' },
            { value: 'k0s', label: 'K0s', icon: 'assets/img/k0s.svg' }
        ],

        // Available artifacts
        artifacts: [
            { value: 'raw', label: 'RAW', icon: 'assets/img/raw.svg', description: '(Always generated) Ready to copy to a USB, SD card, board or use with AWS', alwaysSelected: true },
            { value: 'iso', label: 'ISO', icon: 'assets/img/iso.svg', description: 'Bootable installer for VMs and bare metal' },
            { value: 'tar', label: 'TAR', icon: 'assets/img/tar.svg', description: 'A tar file with the container image for upgrades' },
            { value: 'gcp', label: 'Google Cloud', icon: 'assets/img/gcp.svg', description: 'Cloud image for Google Compute Engine' },
            { value: 'azure', label: 'Azure', icon: 'assets/img/azure.svg', description: 'Cloud image for Microsoft Azure' }
        ],

        // Form logic methods
        getCompatibleModels() {
            if (!this.formData.architecture) {
                return this.models;
            }
            return this.models.filter(model => 
                model.archs.includes(this.formData.architecture)
            );
        },

        handleArchitectureChange() {
            // Auto-select generic model if current model is incompatible
            const currentModel = this.models.find(m => m.value === this.formData.model);
            if (!currentModel || !currentModel.archs.includes(this.formData.architecture)) {
                this.formData.model = 'generic';
            }
        },

        handleVariantChange() {
            // Reset Kubernetes distribution when variant changes
            if (this.formData.variant === 'core') {
                this.formData.kubernetes_distribution = '';
                this.formData.kubernetes_release = '';
            } else if (this.formData.variant === 'standard') {
                this.formData.kubernetes_distribution = 'k3s';
            }
        },

        // Display methods for UI
        getSelectedBaseImageLabel() {
            const selected = this.baseImages.find(img => img.value === this.formData.base_image);
            if (selected?.isCustom) {
                return this.formData.byoi_image || 'Not set';
            }
            return selected?.label || 'Not selected';
        },

        getSelectedBaseImageIcon() {
            const selected = this.baseImages.find(img => img.value === this.formData.base_image);
            return selected?.icon;
        },

        getSelectedModelLabel() {
            const selected = this.models.find(m => m.value === this.formData.model);
            return selected?.label || 'Not selected';
        },

        getSelectedModelIcon() {
            const selected = this.models.find(m => m.value === this.formData.model);
            return selected?.icon;
        },

        getSelectedArchitectureLabel() {
            const selected = this.architectures.find(a => a.value === this.formData.architecture);
            return selected?.label || 'Not selected';
        },

        getSelectedArchitectureIcon() {
            const selected = this.architectures.find(a => a.value === this.formData.architecture);
            return selected?.icon;
        },

        getSelectedVariantLabel() {
            const selected = this.variants.find(v => v.value === this.formData.variant);
            return selected?.label || 'Not selected';
        },

        getSelectedVariantIcon() {
            const selected = this.variants.find(v => v.value === this.formData.variant);
            return selected?.icon;
        },

        getSelectedK8sLabel() {
            const selected = this.kubernetesDistributions.find(k => k.value === this.formData.kubernetes_distribution);
            return selected?.label || 'Not selected';
        },

        getSelectedK8sIcon() {
            const selected = this.kubernetesDistributions.find(k => k.value === this.formData.kubernetes_distribution);
            return selected?.icon;
        },

        getK8sReleaseLabel() {
            return this.formData.kubernetes_release.trim() || 'Latest';
        },

        getVersionLabel() {
            return this.formData.version.trim() || 'Missing';
        },

        getConfigurationLabel() {
            return this.formData.cloud_config.trim() ? 'added' : 'none';
        },

        // Artifact methods
        getSelectedArtifacts() {
            return this.artifacts.filter(artifact => this.formData[`artifact_${artifact.value}`]);
        },

        getSelectedArtifactIcons() {
            return this.getSelectedArtifacts().map(artifact => artifact.icon);
        },

        getArtifactsLabel() {
            const selected = this.getSelectedArtifacts();
            if (selected.length === 0) return 'None selected';
            return `${selected.length} artifact${selected.length > 1 ? 's' : ''}`;
        },

        // Form validation
        validateForm() {
            const errors = [];
            
            if (!this.formData.version.trim()) {
                errors.push(VALIDATION_ERRORS.VERSION_REQUIRED);
            }
            
            if (this.formData.variant === 'standard' && !this.formData.kubernetes_distribution) {
                errors.push(VALIDATION_ERRORS.KUBERNETES_DISTRIBUTION_REQUIRED);
            }
            
            if (this.formData.base_image === 'byoi' && !this.formData.byoi_image.trim()) {
                errors.push(VALIDATION_ERRORS.BYOI_IMAGE_REQUIRED);
            }
            
            return {
                isValid: errors.length === 0,
                errors: errors
            };
        },

        // Form submission
        getFormData() {
            const data = new FormData();
            
            // Add all form fields
            Object.keys(this.formData).forEach(key => {
                const value = this.formData[key];
                if (typeof value === 'boolean') {
                    if (value) {
                        data.append(key, 'true');
                    }
                } else if (value !== null && value !== undefined) {
                    data.append(key, value);
                }
            });
            
            return data;
        },

        // Reset form
        resetForm() {
            this.formData = {
                base_image: 'ubuntu:24.04',
                architecture: 'amd64',
                model: 'generic',
                variant: 'core',
                kubernetes_distribution: 'k3s',
                kubernetes_release: '',
                version: '',
                cloud_config: '',
                byoi_image: '',
                artifact_raw: true,
                artifact_iso: true,
                artifact_tar: true,
                artifact_gcp: false,
                artifact_azure: false
            };
        }
    };
} 