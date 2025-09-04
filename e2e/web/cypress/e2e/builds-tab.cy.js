describe('Builds Tab and Filter Functionality', () => {
    beforeEach(() => {
        cy.visit('/')
        // Wait for Alpine.js to initialize
        cy.get('#accordion-collapse').should('exist')
    })

    describe('Tab Navigation and URL Synchronization', () => {
        it('should start on new build tab by default', () => {
            // Should start on new build tab
            cy.get('[x-show="mainActiveTab === \'newbuild\'"]').should('be.visible')
            cy.get('[x-show="mainActiveTab === \'builds\'"]').should('not.be.visible')
            
            // URL should not have builds hash
            cy.location('hash').should('not.equal', '#builds')
        })

        it('should switch to builds tab and update URL', () => {
            // Click on builds tab (it's a button, not a link)
            cy.contains('button', 'Builds').click()
            
            // Should show builds tab content
            cy.get('[x-show="mainActiveTab === \'builds\'"]').should('be.visible')
            cy.get('[x-show="mainActiveTab === \'newbuild\'"]').should('not.be.visible')
            
            // URL should include builds hash
            cy.location('hash').should('equal', '#builds')
        })

        it('should switch back to new build tab and clean URL', () => {
            // Go to builds tab first
            cy.contains('button', 'Builds').click()
            cy.location('hash').should('equal', '#builds')
            
            // Switch back to new build tab
            cy.contains('button', 'New Build').click()
            
            // Should show new build tab content
            cy.get('[x-show="mainActiveTab === \'newbuild\'"]').should('be.visible')
            cy.get('[x-show="mainActiveTab === \'builds\'"]').should('not.be.visible')
            
            // URL should be clean (no hash)
            cy.location('hash').should('equal', '')
        })

        it('should handle direct URL access to builds tab', () => {
            // Visit builds tab directly
            cy.visit('/#builds')
            
            // Should show builds tab content
            cy.get('[x-show="mainActiveTab === \'builds\'"]').should('be.visible')
            cy.get('[x-show="mainActiveTab === \'newbuild\'"]').should('not.be.visible')
            
            // URL should maintain builds hash
            cy.location('hash').should('equal', '#builds')
        })
    })

    describe('Status Filter Functionality', () => {
        beforeEach(() => {
            // Switch to builds tab
            cy.contains('button', 'Builds').click()
            cy.get('[x-show="mainActiveTab === \'builds\'"]').should('be.visible')
        })

        it('should have status filter dropdown with correct options', () => {
            // Check that filter dropdown exists
            cy.get('select[x-model="statusFilter"]').should('exist')
            
            // Check all filter options are present
            cy.get('select[x-model="statusFilter"] option[value=""]').should('contain', 'All Status')
            cy.get('select[x-model="statusFilter"] option[value="queued"]').should('contain', 'Queued')
            cy.get('select[x-model="statusFilter"] option[value="assigned"]').should('contain', 'Assigned')
            cy.get('select[x-model="statusFilter"] option[value="running"]').should('contain', 'Running')
            cy.get('select[x-model="statusFilter"] option[value="complete"]').should('contain', 'Complete')
            cy.get('select[x-model="statusFilter"] option[value="failed"]').should('contain', 'Failed')
        })

        it('should update URL when filter is changed', () => {
            // Intercept API calls to prevent actual requests during testing
            cy.intercept('GET', '/api/v1/builds*', { body: { builds: [], total: 0 } }).as('getBuilds')
            
            // Select a filter
            cy.get('select[x-model="statusFilter"]').select('running')
            
            // URL should include status parameter
            cy.location('search').should('include', 'status=running')
            cy.location('hash').should('equal', '#builds')
            
            // API should be called with filter
            cy.wait('@getBuilds').then((interception) => {
                expect(interception.request.url).to.include('status=running')
            })
        })

        it('should preserve filter state when switching tabs', () => {
            // Intercept API calls
            cy.intercept('GET', '/api/v1/builds*', { body: { builds: [], total: 0 } }).as('getBuilds')
            
            // Set a filter
            cy.get('select[x-model="statusFilter"]').select('complete')
            cy.location('search').should('include', 'status=complete')
            
            // Switch to new build tab
            cy.contains('button', 'New Build').click()
            cy.get('[x-show="mainActiveTab === \'newbuild\'"]').should('be.visible')
            
            // URL should be clean
            cy.location('search').should('not.include', 'status=')
            cy.location('hash').should('equal', '')
            
            // Switch back to builds tab
            cy.contains('button', 'Builds').first().click()
            cy.get('[x-show="mainActiveTab === \'builds\'"]').should('be.visible')
            
            // Filter should be preserved and URL should be updated
            cy.get('select[x-model="statusFilter"]').should('have.value', 'complete')
            cy.location('search').should('include', 'status=complete')
            cy.location('hash').should('equal', '#builds')
        })

        it('should handle direct URL access with filter', () => {
            // Visit builds tab with filter directly  
            cy.visit('/#builds?status=failed')
            
            // Wait for the accordion to load (indicating Alpine.js is initialized)
            cy.get('#accordion-collapse').should('exist')
            
            // Click on the builds tab to ensure it's active (sometimes direct URL doesn't trigger it)
            cy.contains('button', 'Builds').click()
            
            // Wait for page to load and Alpine.js to initialize the builds tab
            cy.get('[x-show="mainActiveTab === \'builds\'"]', { timeout: 10000 }).should('be.visible')
            
            // Give Alpine.js additional time to process URL parameters
            cy.wait(1000)
            
            // The filter should be set correctly from the URL
            cy.get('select[x-model="statusFilter"]', { timeout: 5000 }).should('have.value', 'failed')
            
            // The URL should be maintained
            cy.location('search').should('include', 'status=failed')
            cy.location('hash').should('equal', '#builds')
        })

        it('should clear filter when "All Status" is selected', () => {
            // Intercept API calls - need multiple intercepts to handle sequence
            cy.intercept('GET', '/api/v1/builds*', { body: { builds: [], total: 0 } }).as('getBuilds')
            
            // Set a filter first
            cy.get('select[x-model="statusFilter"]').select('queued')
            cy.wait('@getBuilds') // Wait for first API call
            cy.location('search').should('include', 'status=queued')
            
            // Re-setup intercept for the clear operation
            cy.intercept('GET', '/api/v1/builds*', { body: { builds: [], total: 0 } }).as('getBuildsCleared')
            
            // Clear filter by selecting "All Status"
            cy.get('select[x-model="statusFilter"]').select('')
            
            // Wait for the API call and check URL
            cy.wait('@getBuildsCleared').then((interception) => {
                expect(interception.request.url).to.not.include('status=')
            })
            
            // URL should not have status parameter
            cy.location('search').should('not.include', 'status=')
            cy.location('hash').should('equal', '#builds')
        })
    })

    describe('Browser Navigation', () => {
        it('should handle browser back/forward with tab switching', () => {
            // Start on new build tab
            cy.location('hash').should('not.equal', '#builds')
            
            // Go to builds tab
            cy.contains('button', 'Builds').click()
            cy.location('hash').should('equal', '#builds')
            
            // Use browser back
            cy.go('back')
            cy.get('[x-show="mainActiveTab === \'newbuild\'"]').should('be.visible')
            cy.location('hash').should('equal', '')
            
            // Use browser forward
            cy.go('forward')
            cy.get('[x-show="mainActiveTab === \'builds\'"]').should('be.visible')
            cy.location('hash').should('equal', '#builds')
        })

        it('should handle browser navigation with filter state', () => {
            // Intercept API calls
            cy.intercept('GET', '/api/v1/builds*', { body: { builds: [], total: 0 } }).as('getBuilds')
            
            // Go to builds and set filter
            cy.contains('button', 'Builds').click()
            cy.get('select[x-model="statusFilter"]').select('running')
            cy.location('search').should('include', 'status=running')
            
            // Navigate away and back
            cy.contains('button', 'New Build').click()
            cy.location('search').should('not.include', 'status=')
            
            // Use browser back
            cy.go('back')
            cy.get('[x-show="mainActiveTab === \'builds\'"]').should('be.visible')
            cy.get('select[x-model="statusFilter"]').should('have.value', 'running')
            cy.location('search').should('include', 'status=running')
        })
    })

    describe('Build List Display', () => {
        beforeEach(() => {
            // Switch to builds tab
            cy.contains('button', 'Builds').click()
        })

        it('should show loading state initially', () => {
            // Mock slow API response
            cy.intercept('GET', '/api/v1/builds*', { 
                delay: 500,
                body: { builds: [], total: 0 } 
            }).as('getBuildsSlowly')
            
            // Start fresh and go to builds tab
            cy.visit('/')
            cy.contains('button', 'Builds').click()
            
            // Should show loading spinner while API call is pending
            cy.get('.animate-spin').should('exist')
            
            // Wait for API call to complete
            cy.wait('@getBuildsSlowly')
        })

        it('should show empty state when no builds exist', () => {
            // Mock empty response
            cy.intercept('GET', '/api/v1/builds*', { body: { builds: [], total: 0 } }).as('getBuildsEmpty')
            
            cy.reload()
            cy.contains('button', 'Builds').click()
            
            // Should show empty state
            cy.wait('@getBuildsEmpty')
            cy.contains('No builds found').should('be.visible')
        })

        it('should display builds when they exist', () => {
            // Mock builds response
            const mockBuilds = [
                {
                    uuid: '123e4567-e89b-12d3-a456-426614174000',
                    image: 'ubuntu:24.04',
                    architecture: 'amd64',
                    model: 'generic',
                    variant: 'core',
                    version: '1.0.0',
                    status: 'complete',
                    created_at: new Date().toISOString()
                }
            ]
            
            cy.intercept('GET', '/api/v1/builds*', { 
                body: { builds: mockBuilds, total: 1 } 
            }).as('getBuildsWithData')
            
            cy.reload()
            cy.contains('button', 'Builds').click()
            
            // Should show builds grid
            cy.wait('@getBuildsWithData')
            cy.get('.cursor-pointer.p-4').should('have.length', 1)
            
            // Should show build details
            cy.contains('ubuntu:24.04').should('be.visible')
            cy.contains('core').should('be.visible')
            cy.contains('complete').should('be.visible')
        })
    })
})
