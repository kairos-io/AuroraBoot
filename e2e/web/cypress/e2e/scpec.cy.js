describe('Kairos Factory Web Interface', () => {
    beforeEach(() => {
        cy.visit('/')
        // Wait for the page to be fully loaded
        cy.get('#accordion-collapse').should('exist')
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
        
        // Check for required sections
        cy.get('#accordion-heading-base-image').should('exist');
        cy.get('#accordion-heading-architecture').should('exist');
        cy.get('#accordion-heading-model').should('exist');
        cy.get('#accordion-heading-variant').should('exist');
        cy.get('#accordion-heading-kubernetes').should('exist');
        cy.get('#accordion-heading-kubernetes-release').should('exist');
        cy.get('#accordion-heading-version').should('exist');
    });

    it('should have all required accordion bodies', () => {
        cy.get('#accordion-body-base-image').should('exist');
        cy.get('#accordion-body-architecture').should('exist');
        cy.get('#accordion-body-model').should('exist');
        cy.get('#accordion-body-variant').should('exist');
        cy.get('#accordion-body-kubernetes').should('exist');
        cy.get('#accordion-body-kubernetes-release').should('exist');
        cy.get('#accordion-body-version').should('exist');
    });

    it('should allow section interaction', () => {
        // Helper function to check section state
        const checkSectionState = (headingId, bodyId, shouldBeExpanded) => {
            cy.get(`#${headingId} button`).should('have.attr', 'aria-expanded', shouldBeExpanded.toString());
            cy.get(`#${bodyId}`).should(shouldBeExpanded ? 'not.have.class' : 'have.class', 'hidden');
        };

        // Check initial state - first section should be expanded
        checkSectionState('accordion-heading-base-image', 'accordion-body-base-image', true);
        checkSectionState('accordion-heading-architecture', 'accordion-body-architecture', false);

        // Click on architecture section
        cy.get('#accordion-heading-architecture button').click();
        checkSectionState('accordion-heading-architecture', 'accordion-body-architecture', true);
        checkSectionState('accordion-heading-base-image', 'accordion-body-base-image', false);

        // Click on model section
        cy.get('#accordion-heading-model button').click();
        checkSectionState('accordion-heading-model', 'accordion-body-model', true);
        checkSectionState('accordion-heading-architecture', 'accordion-body-architecture', false);
    });

    it('should handle radio button selections', () => {
        // Helper function to check radio selection
        const checkRadioSelection = (name, value) => {
            cy.get(`input[type="radio"][name="${name}"][value="${value}"]`).should('be.checked');
        };

        // Select base image
        cy.get('label[for="ubuntu-option"]').click();
        checkRadioSelection('base_image', 'ubuntu:24.04');

        // Select architecture
        cy.get('#accordion-heading-architecture button').click();
        cy.get('label[for="amd64-option"]').click();
        checkRadioSelection('architecture', 'amd64');

        // Select model
        cy.get('#accordion-heading-model button').click();
        cy.get('label[for="generic-option"]').click();
        checkRadioSelection('model', 'generic');
    });

    it('should handle form submission', () => {
        // Intercept the build start request
        cy.intercept('POST', '/start').as('startBuild');

        // Fill out required fields
        cy.get('label[for="ubuntu-option"]').click();
        cy.get('#accordion-heading-architecture button').click();
        cy.get('label[for="amd64-option"]').click();
        cy.get('#accordion-heading-model button').click();
        cy.get('label[for="generic-option"]').click();
        cy.get('#accordion-heading-variant button').click();
        cy.get('label[for="core-option"]').click();
        cy.get('#accordion-heading-version button').click();
        cy.get('#version').type('v0.1.0-alpha');

        // Submit form
        cy.get('#submit-button').click();

        // Wait for the build start request to complete
        cy.wait('@startBuild');

        // Polling function to check for visibility at increasing intervals
        function checkBuildingContainerImage(attempt = 1) {
            const waitTimes = [5000, 15000, 30000, 60000, 120000];
            if (attempt > waitTimes.length) {
                // Final fail if not visible after all attempts
                cy.get('#building-container-image').should('be.visible');
                return;
            }
            cy.wait(waitTimes[attempt - 1]).then(() => {
                cy.get('#building-container-image').then($el => {
                    if (Cypress.dom.isVisible($el)) {
                        // Element is visible, test passes
                        expect(Cypress.dom.isVisible($el)).to.be.true;
                    } else {
                        // Try again with the next wait time
                        checkBuildingContainerImage(attempt + 1);
                    }
                });
            });
        }

        // Check if modal appears
        cy.get('#static-modal').should('be.visible');
        checkBuildingContainerImage();
    });

    it('should show ARM-specific options when ARM64 is selected', () => {
        // Select ARM64 architecture
        cy.get('#accordion-heading-architecture button').click();
        cy.get('label[for="arm64-option"]').click();
        // Open the model accordion section
        cy.get('#accordion-heading-model button').click();
        // Check if ARM-specific model options are visible
        cy.get('.model-option.arm-only').each($el => {
            cy.wrap($el).should('be.visible');
          });
    });

    it('should hide ARM-specific options when AMD64 is selected', () => {
        // Select AMD64 architecture
        cy.get('#accordion-heading-architecture button').click();
        cy.get('label[for="amd64-option"]').click();

        // Check if ARM-specific model options are hidden
        cy.get('.model-option.arm-only').should('have.class', 'hidden');
    });

    it('should handle BYOI (Bring Your Own Image) option', () => {
        // Select BYOI option
        cy.get('label[for="byoi-option"]').click();

        // Check if BYOI input field is visible and enabled
        cy.get('#byoi').should('be.visible').and('be.enabled');
        
        // Enter custom image
        cy.get('#byoi').type('custom-repo.com/image:tag');
        
        // Verify the value was entered
        cy.get('#byoi').should('have.value', 'custom-repo.com/image:tag');
    });
});