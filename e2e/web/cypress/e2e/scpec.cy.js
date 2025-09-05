describe('Kairos Factory Web Interface', () => {
    beforeEach(() => {
        cy.visit('/')
        // Wait for the page to be fully loaded
        cy.get('#accordion-collapse').should('exist')
        // Wait for Alpine.js to initialize the accordion sections
        cy.get('[id^="accordion-heading-"]').should('have.length.at.least', 4)
    })

    const expectedText = [
        "Ubuntu 24.04 LTS",
        "Fedora 40",
        "Alpine 3.21",
        "Rocky Linux 9",
        "Debian 12 (Bookworm)",
        "AMD64",
        "ARM64",
        "Core",
        "Standard",
    ];

    // Test accordion sections exist
    it('should have all required accordion sections', () => {
        // Check for the main accordion container
        cy.get('#accordion-collapse').should('exist');

        // Check for required sections (these are always visible)
        cy.get('#accordion-heading-base-image').should('exist');
        cy.get('#accordion-heading-architecture').should('exist');
        cy.get('#accordion-heading-model').should('exist');
        cy.get('#accordion-heading-variant').should('exist');
        cy.get('#accordion-heading-version').should('exist');
        cy.get('#accordion-heading-configuration').should('exist');
        cy.get('#accordion-heading-artifacts').should('exist');

        // Kubernetes sections are conditional - need to select Standard variant first
        // First open the variant section and wait for it to be visible
        cy.get('#accordion-heading-variant').click();
        cy.get('#accordion-body-variant').should('be.visible');
        
        // Click on standard variant option and wait for Alpine.js to process
        cy.get('label[for="option-standard"]').click();
        cy.wait(500); // Give Alpine.js time to process the variant change
        
        // Wait for the conditional sections to appear after variant change
        cy.get('#accordion-heading-kubernetes', { timeout: 10000 }).should('exist');
        cy.get('#accordion-heading-kubernetes-release', { timeout: 10000 }).should('exist');
    });

    it('should have all required accordion bodies', () => {
        cy.get('#accordion-body-base-image').should('exist');
        cy.get('#accordion-body-architecture').should('exist');
        cy.get('#accordion-body-model').should('exist');
        cy.get('#accordion-body-variant').should('exist');
        cy.get('#accordion-body-version').should('exist');
        cy.get('#accordion-body-configuration').should('exist');
        cy.get('#accordion-body-artifacts').should('exist');
        
        // Kubernetes sections are conditional
        cy.get('#accordion-heading-variant button').click();
        // Then wait for the body to be visible
        cy.get('#accordion-body-variant', { timeout: 5000 }).should('be.visible');
        cy.get('label[for="option-standard"]').click();
        cy.wait(1000); // Give Alpine.js time to process the variant change
        
        // Wait for the conditional sections to appear after variant change
        cy.get('#accordion-body-kubernetes', { timeout: 10000 }).should('exist');
        cy.get('#accordion-body-kubernetes-release', { timeout: 10000 }).should('exist');
    });

    it('should allow section interaction', () => {
        // Helper function to check section state
        const checkSectionState = (headingId, bodyId, shouldBeExpanded) => {
            cy.get(`#${headingId} button`).should('have.attr', 'aria-expanded', shouldBeExpanded.toString());
            if (shouldBeExpanded) {
                cy.get(`#${bodyId}`).should('be.visible');
            } else {
                cy.get(`#${bodyId}`).should('not.be.visible');
            }
        };

        // Check initial state - first section should be expanded
        checkSectionState('accordion-heading-base-image', 'accordion-body-base-image', true);
        checkSectionState('accordion-heading-architecture', 'accordion-body-architecture', false);

        // Click on architecture section
        cy.get('#accordion-heading-architecture button').first().click();
        checkSectionState('accordion-heading-architecture', 'accordion-body-architecture', true);
        checkSectionState('accordion-heading-base-image', 'accordion-body-base-image', false);

        // Click on model section
        cy.get('#accordion-heading-model button').first().click();
        checkSectionState('accordion-heading-model', 'accordion-body-model', true);
        checkSectionState('accordion-heading-architecture', 'accordion-body-architecture', false);
    });

    it('should handle radio button selections', () => {
        // Helper function to check radio selection
        const checkRadioSelection = (name, value) => {
            cy.get(`input[type="radio"][name="${name}"][value="${value}"]`).should('be.checked');
        };

        // Helper function to check text input value
        const checkTextInputValue = (name, value) => {
            cy.get(`input[type="text"][name="${name}"]`).should('have.value', value);
        };

        // Select base image using the new button-based system (Ubuntu is default)
        cy.get('button').contains('Ubuntu 24.04 LTS').click();
        checkTextInputValue('base_image', 'ubuntu:24.04');

        // Select architecture (AMD64 is selected by default)
        cy.get('#accordion-heading-architecture button').first().click();
        cy.get('label[for="option-amd64"]').click();
        checkRadioSelection('architecture', 'amd64');

        // Select model (Generic is selected by default)
        cy.get('#accordion-heading-model button').first().click();
        cy.get('label[for="option-generic"]').click();
        checkRadioSelection('model', 'generic');
    });

    it('should handle form submission', () => {
        // Intercept the build start request
        cy.intercept('POST', '/start').as('startBuild');

        // Fill out required fields using new Alpine.js structure
        cy.get('button').contains('Ubuntu 24.04 LTS').click();
        cy.get('#accordion-heading-architecture button').first().click();
        cy.get('label[for="option-amd64"]').click();
        cy.get('#accordion-heading-model button').first().click();
        cy.get('label[for="option-generic"]').click();
        cy.get('#accordion-heading-variant button').first().click();
        cy.get('label[for="option-core"]').click();
        cy.get('#accordion-heading-version button').first().click();
        cy.get('#version').type('v0.1.0-alpha');

        // Submit form
        cy.get('#submit-button').click();

        // Wait for the build start request to complete
        cy.wait('@startBuild');

        // After form submission, should redirect to builds tab
        cy.get('[x-show="mainActiveTab === \'builds\'"]', { timeout: 10000 }).should('be.visible');
        
        // Should be on builds tab with the hash
        cy.location('hash').should('equal', '#builds');
    });

    it('should show ARM-specific options when ARM64 is selected', () => {
        // Select ARM64 architecture
        cy.get('#accordion-heading-architecture button').first().click();
        cy.get('label[for="option-arm64"]').click();
        // Open the model accordion section
        cy.get('#accordion-heading-model button').first().click();
        // Check if ARM-specific model options are visible (Raspberry Pi models)
        cy.get('label[for="option-rpi3"]').should('be.visible');
        cy.get('label[for="option-rpi4"]').should('be.visible');
        cy.get('label[for="option-nvidia-agx-orin"]').should('be.visible');
    });

    it('should hide ARM-specific options when AMD64 is selected', () => {
        // Select AMD64 architecture (default)
        cy.get('#accordion-heading-architecture button').first().click();
        cy.get('label[for="option-amd64"]').click();
        cy.get('#accordion-heading-model button').first().click();

        // ARM-specific model options should not be present in the DOM for AMD64
        cy.get('label[for="option-rpi3"]').should('not.exist');
        cy.get('label[for="option-rpi4"]').should('not.exist');
        cy.get('label[for="option-nvidia-agx-orin"]').should('not.exist');
    });

    it('should handle BYOI (Bring Your Own Image) option', () => {
        // Check if base image input field is visible and enabled
        cy.get('#base_image').should('be.visible').and('be.enabled');
        
        // Clear the default value and enter custom image
        cy.get('#base_image').clear().type('custom-repo.com/image:tag');
        
        // Verify the value was entered
        cy.get('#base_image').should('have.value', 'custom-repo.com/image:tag');
        
        // Test that clicking a helper button updates the field
        cy.get('button').contains('Fedora 40').click();
        cy.get('#base_image').should('have.value', 'fedora:40');
        
        // Test that we can edit after clicking a button
        cy.get('#base_image').clear().type('my-custom:latest');
        cy.get('#base_image').should('have.value', 'my-custom:latest');
    });
});