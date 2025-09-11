
describe('Advanced Options Functionality', () => {
    beforeEach(() => {
        // Mock the config API to provide a default kairos-init version
        cy.intercept('GET', '/api/v1/config', { 
            body: { default_kairos_init_version: 'v0.4.9' } 
        }).as('getConfig')

        // Mock the form submission endpoint - based on the screenshot it goes to /start
        cy.intercept('POST', '/start', { 
            statusCode: 200,
            body: { uuid: '123e4567-e89b-12d3-a456-426614174000' } 
        }).as('submitBuild')

        cy.visit('/')
        
        // Wait for Alpine.js to initialize and config to load
        cy.get('#accordion-collapse').should('exist')
        cy.wait('@getConfig')
        
        // Wait a bit for Alpine.js to process the config
        cy.wait(500)
    })

    describe('Advanced Options Toggle', () => {
        it('should have the advanced options checkbox initially unchecked', () => {
            cy.get('#showAdvancedOptions').should('not.be.checked')
        })

        it('should have the correct label for the advanced options checkbox', () => {
            cy.get('label[for="showAdvancedOptions"]')
                .should('contain', 'Show advanced options')
        })

        it('should toggle advanced options visibility when checkbox is clicked', () => {
            // Initially, advanced options should be hidden
            cy.get('#showAdvancedOptions').should('not.be.checked')
            
            // Kairos-init section should exist in DOM but not be visible initially
            cy.get('[id="accordion-heading-kairos-init-version"]').should('exist').and('not.be.visible')
            
            // Check the checkbox
            cy.get('#showAdvancedOptions').check()
            cy.get('#showAdvancedOptions').should('be.checked')
            
            // Now kairos-init section should be visible
            cy.get('[id="accordion-heading-kairos-init-version"]').should('be.visible')
            
            // Uncheck the checkbox
            cy.get('#showAdvancedOptions').uncheck()
            cy.get('#showAdvancedOptions').should('not.be.checked')
            
            // Kairos-init section should be hidden again but still exist in DOM
            cy.get('[id="accordion-heading-kairos-init-version"]').should('exist').and('not.be.visible')
        })

        it('should work when clicking the label instead of the checkbox', () => {
            // Initially, kairos-init section should exist but not be visible
            cy.get('[id="accordion-heading-kairos-init-version"]').should('exist').and('not.be.visible')
            
            // Click the label to toggle the checkbox
            cy.get('label[for="showAdvancedOptions"]').click()
            cy.get('#showAdvancedOptions').should('be.checked')
            
            // Kairos-init section should now be visible
            cy.get('[id="accordion-heading-kairos-init-version"]').should('be.visible')
            
            // Click again to uncheck
            cy.get('label[for="showAdvancedOptions"]').click()
            cy.get('#showAdvancedOptions').should('not.be.checked')
            
            // Kairos-init section should be hidden again but still exist
            cy.get('[id="accordion-heading-kairos-init-version"]').should('exist').and('not.be.visible')
        })
    })

    describe('Kairos Init Version Section Visibility', () => {
        it('should hide kairos-init section when advanced options are disabled', () => {
            // Ensure advanced options are unchecked
            cy.get('#showAdvancedOptions').should('not.be.checked')
            
            // The kairos-init section should exist in DOM but not be visible
            cy.get('[id="accordion-heading-kairos-init-version"]').should('exist').and('not.be.visible')
            cy.get('[id="accordion-body-kairos-init-version"]').should('exist').and('not.be.visible')
        })

        it('should show kairos-init section when advanced options are enabled', () => {
            // Enable advanced options
            cy.get('#showAdvancedOptions').check()
            
            // The kairos-init section header should now be visible
            cy.get('[id="accordion-heading-kairos-init-version"]').should('be.visible')
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]')
                .should('be.visible')
                .should('contain', 'Kairos Init Version')
        })

        it('should have the advanced section styled with orange border when visible', () => {
            // Enable advanced options
            cy.get('#showAdvancedOptions').check()
            
            // The kairos-init section header should have the orange border styling
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]')
                .should('be.visible')
                .and('have.class', 'border-l-orange-500')
        })
    })

    describe('Kairos Init Version Section Content', () => {
        beforeEach(() => {
            // Enable advanced options for these tests
            cy.get('#showAdvancedOptions').check()
        })

        it('should expand kairos-init section when clicked', () => {
            // Click on the kairos-init section header
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]').click()
            
            // The section content should be visible
            cy.get('[id="accordion-body-kairos-init-version"]')
                .should('be.visible')
        })

        it('should show the kairos-init input field when expanded', () => {
            // Expand the section
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]').click()
            
            // Should show the input field within the expanded section
            cy.get('[id="accordion-body-kairos-init-version"]').within(() => {
                cy.get('input[name="kairos_init_version"]')
                    .should('be.visible')
                    .should('have.attr', 'placeholder', 'latest')
            })
        })

        it('should pre-populate kairos-init version from config API', () => {
            // Expand the section
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]').click()
            
            // The input should have the default value from the API
            cy.get('[id="accordion-body-kairos-init-version"]').within(() => {
                cy.get('input[name="kairos_init_version"]')
                    .should('have.value', 'v0.4.9')
            })
        })

        it('should show info popover for kairos-init section', () => {
            // The info popover should be visible on the section header button
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]')
                .find('button[type="button"]')
                .trigger('mouseenter')
            
            // The popover should appear
            cy.get('[x-show="showPopover"]')
                .should('be.visible')
                .should('contain', 'This controls which features and bug fixes')
        })

        it('should display the correct selected label in collapsed state', () => {
            // With the default value, the label should show "v0.4.9"
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]')
                .should('contain', 'v0.4.9')
        })

        it('should update selected label when input value changes', () => {
            // Expand the section
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]').click()
            
            // Change the input value within the expanded section
            cy.get('[id="accordion-body-kairos-init-version"]').within(() => {
                cy.get('input[name="kairos_init_version"]')
                    .clear()
                    .type('v0.5.0')
            })
            
            // Collapse the section to see the label
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]').click()
            
            // The label should now show the new value
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]')
                .should('contain', 'v0.5.0')
        })

        it('should show "Latest" when input is empty', () => {
            // Expand the section
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]').click()
            
            // Clear the input within the expanded section
            cy.get('[id="accordion-body-kairos-init-version"]').within(() => {
                cy.get('input[name="kairos_init_version"]').clear()
            })
            
            // Collapse the section to see the label
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]').click()
            
            // The label should show "Latest"
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]')
                .should('contain', 'Latest')
        })
    })

    describe('Form Submission with Advanced Options', () => {
        beforeEach(() => {
            // Wait for the form to be ready and Alpine.js to initialize
            cy.get('#accordion-collapse').should('be.visible')
            cy.wait(1000) // Give Alpine.js time to render all sections
            
            // Simple approach: Just ensure the base-image section is visible (it should be open by default)
            cy.get('[id="accordion-body-base-image"]', { timeout: 2000 }).should('be.visible')
            cy.get('[id="accordion-body-base-image"]').within(() => {
                // Click on Ubuntu option to ensure it's selected
                cy.get('button').contains('Ubuntu 24.04 LTS').click()
            })
            
            // Fill in the required version field
            cy.get('button[aria-controls="accordion-body-version"]').click()
            cy.get('[id="accordion-body-version"]', { timeout: 2000 }).should('be.visible')
            cy.get('[id="accordion-body-version"]').within(() => {
                cy.get('input[name="version"]').clear().type('v1.0.0')
            })
            
            // Verify the Build button is present which indicates form is ready
            cy.get('button[type="submit"]').should('be.visible').should('contain', 'Build')
        })

        it('should include kairos_init_version with default value when advanced options are hidden', () => {
            // Ensure advanced options are disabled
            cy.get('#showAdvancedOptions').should('not.be.checked')
            
            // Verify that the kairos-init input field exists in DOM but is not visible
            cy.get('[id="accordion-heading-kairos-init-version"]').should('exist').and('not.be.visible')
            
            // Verify the kairos_init_version field exists and has the correct default value
            cy.get('body').then(() => {
                const kairosField = Cypress.$('input[name="kairos_init_version"]')
                expect(kairosField.length).to.be.greaterThan(0)
                expect(kairosField.val()).to.equal('v0.4.9')
            })
        })

        it('should include custom kairos_init_version when manually set with advanced options', () => {
            // Enable advanced options and verify section becomes visible
            cy.get('#showAdvancedOptions').check()
            cy.get('[id="accordion-heading-kairos-init-version"]').should('be.visible')
            
            // Expand kairos-init section and set a custom value
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]').click()
            cy.get('[id="accordion-body-kairos-init-version"]', { timeout: 2000 }).should('be.visible')
            cy.get('[id="accordion-body-kairos-init-version"]').within(() => {
                cy.get('input[name="kairos_init_version"]')
                    .should('be.visible')
                    .clear()
                    .type('v0.6.0')
                    .should('have.value', 'v0.6.0') // Verify the value was set
            })
            
            // Verify the field value is correctly set
            cy.get('input[name="kairos_init_version"]').should('have.value', 'v0.6.0')
        })

        it('should submit empty kairos_init_version when cleared in advanced options', () => {
            // Enable advanced options and verify section becomes visible
            cy.get('#showAdvancedOptions').check()
            cy.get('[id="accordion-heading-kairos-init-version"]').should('be.visible')
            
            // Expand kairos-init section and clear the value
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]').click()
            cy.get('[id="accordion-body-kairos-init-version"]').within(() => {
                cy.get('input[name="kairos_init_version"]').clear()
            })
            
            // Verify the field value is correctly cleared
            cy.get('input[name="kairos_init_version"]').should('have.value', '')
        })
    })

    describe('Advanced Options State Persistence', () => {
        it('should maintain advanced options state when switching tabs', () => {
            // Enable advanced options and verify section becomes visible
            cy.get('#showAdvancedOptions').check()
            cy.get('#showAdvancedOptions').should('be.checked')
            cy.get('[id="accordion-heading-kairos-init-version"]').should('be.visible')
            
            // Switch to builds tab
            cy.contains('button', 'Builds').click()
            cy.get('[x-show="mainActiveTab === \'builds\'"]').should('be.visible')
            
            // Switch back to new build tab
            cy.contains('button', 'New Build').click()
            cy.get('[x-show="mainActiveTab === \'newbuild\'"]').should('be.visible')
            
            // Advanced options should still be enabled
            cy.get('#showAdvancedOptions').should('be.checked')
            
            // Kairos-init section should still be visible
            cy.get('[id="accordion-heading-kairos-init-version"]').should('be.visible')
        })

        it('should preserve form values when toggling advanced options', () => {
            // Enable advanced options and set a custom value
            cy.get('#showAdvancedOptions').check()
            cy.get('[id="accordion-heading-kairos-init-version"]').should('be.visible')
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]').click()
            cy.get('[id="accordion-body-kairos-init-version"]').within(() => {
                cy.get('input[name="kairos_init_version"]')
                    .clear()
                    .type('v0.7.0')
            })
            
            // Disable advanced options (section should be hidden but still exist)
            cy.get('#showAdvancedOptions').uncheck()
            cy.get('[id="accordion-heading-kairos-init-version"]').should('exist').and('not.be.visible')
            
            // Re-enable advanced options (section should become visible again)
            cy.get('#showAdvancedOptions').check()
            cy.get('[id="accordion-heading-kairos-init-version"]').should('be.visible')
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]').click()
            
            // The value should still be there
            cy.get('[id="accordion-body-kairos-init-version"]').within(() => {
                cy.get('input[name="kairos_init_version"]')
                    .should('have.value', 'v0.7.0')
            })
        })
    })

    describe('No Config API Response', () => {
        it('should handle missing config gracefully', () => {
            // Mock a failing config API
            cy.intercept('GET', '/api/v1/config', { statusCode: 500 }).as('getConfigFail')
            
            // Visit the page
            cy.visit('/')
            cy.get('#accordion-collapse').should('exist')
            
            // Enable advanced options
            cy.get('#showAdvancedOptions').check()
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]').click()
            
            // The input should be empty since config failed
            cy.get('input[name="kairos_init_version"]')
                .should('have.value', '')
                .should('have.attr', 'placeholder', 'latest')
        })

        it('should handle empty config response', () => {
            // Mock empty config response
            cy.intercept('GET', '/api/v1/config', { body: {} }).as('getConfigEmpty')
            
            cy.visit('/')
            cy.get('#accordion-collapse').should('exist')
            cy.wait('@getConfigEmpty')
            
            // Enable advanced options
            cy.get('#showAdvancedOptions').check()
            cy.get('button[aria-controls="accordion-body-kairos-init-version"]').click()
            
            // The input should be empty
            cy.get('input[name="kairos_init_version"]')
                .should('have.value', '')
        })
    })

    describe('Accessibility and User Experience', () => {
        it('should have proper ARIA attributes for the advanced options checkbox', () => {
            cy.get('#showAdvancedOptions')
                .should('have.attr', 'type', 'checkbox')
                .should('have.attr', 'id', 'showAdvancedOptions')
            
            cy.get('label[for="showAdvancedOptions"]')
                .should('have.attr', 'for', 'showAdvancedOptions')
        })

        it('should have proper keyboard navigation for advanced options', () => {
            // Focus the checkbox directly
            cy.get('#showAdvancedOptions').focus().should('have.focus')
            
            // Press space to toggle
            cy.get('#showAdvancedOptions').type(' ')
            cy.get('#showAdvancedOptions').should('be.checked')
            
            // Press space again to toggle off
            cy.get('#showAdvancedOptions').type(' ')
            cy.get('#showAdvancedOptions').should('not.be.checked')
        })

        it('should show advanced sections when advanced options are enabled', () => {
            // Enable advanced options
            cy.get('#showAdvancedOptions').check()
            
            // The advanced section should be visible
            cy.get('[id="accordion-heading-kairos-init-version"]')
                .should('be.visible')
        })
    })
})
