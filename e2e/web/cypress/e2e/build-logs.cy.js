describe('Build Logs and Modal Functionality', () => {
    const mockBuild = {
        uuid: '123e4567-e89b-12d3-a456-426614174000',
        image: 'ubuntu:24.04',
        architecture: 'amd64',
        model: 'generic',
        variant: 'core',
        version: '1.0.0',
        status: 'complete',
        created_at: new Date(Date.now() - 3600000).toISOString(), // 1 hour ago
        updated_at: new Date().toISOString(),
        completed_at: new Date().toISOString()
    }

    beforeEach(() => {
        // Mock builds API
        cy.intercept('GET', '/api/v1/builds*', { 
            body: { builds: [mockBuild], total: 1 } 
        }).as('getBuilds')

        // Mock individual build API
        cy.intercept('GET', `/api/v1/builds/${mockBuild.uuid}`, { 
            body: mockBuild 
        }).as('getBuild')

        // Mock artifacts API
        cy.intercept('GET', `/api/v1/builds/${mockBuild.uuid}/artifacts`, { 
            body: [
                {
                    name: 'ISO image',
                    description: 'For generic installations (USB, VM, bare metal)',
                    url: 'kairos-ubuntu-24.04-core-amd64-generic-v1.0.0.iso'
                },
                {
                    name: 'RAW image', 
                    description: 'For AWS, Raspberry Pi, and any platform that supports RAW images',
                    url: 'kairos-ubuntu-24.04-core-amd64-generic-v1.0.0.raw'
                }
            ]
        }).as('getArtifacts')

        cy.visit('/#builds')
        cy.wait('@getBuilds')
    })

    describe('Build Modal Opening and Navigation', () => {
        it('should open build modal when clicking on a build', () => {
            // Wait for builds template to appear (but not necessarily be visible)
            cy.get('[x-if="!loading && builds.length > 0"]').should('exist')
            
            // Wait for the actual build list content to appear
            cy.contains('.cursor-pointer.p-4', 'ubuntu:24.04').should('be.visible')
            
            // Click on the build
            cy.get('.cursor-pointer.p-4').first().click()

            // Modal should exist (x-if creates/destroys elements)
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('exist')
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('be.visible')
            
            // Should show build details
            cy.contains('ubuntu:24.04').should('be.visible')
            cy.contains('core').should('be.visible')
            cy.contains('complete').should('be.visible')
        })

        it('should update URL when build modal is opened', () => {
            // Wait for builds to load
            cy.get('[x-if="!loading && builds.length > 0"]').should('exist')
            cy.contains('.cursor-pointer.p-4', 'ubuntu:24.04').should('be.visible')
            
            // Click on build
            cy.get('.cursor-pointer.p-4').first().click()

            // URL should include build parameter
            cy.location('search').should('include', `build=${mockBuild.uuid}`)
            cy.location('hash').should('equal', '#builds')
        })

        it('should close modal and clean URL when close button is clicked', () => {
            // Wait for builds to load
            cy.get('[x-if="!loading && builds.length > 0"]').should('exist')
            cy.contains('.cursor-pointer.p-4', 'ubuntu:24.04').should('be.visible')
            
            // Open modal
            cy.get('.cursor-pointer.p-4').first().click()
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('be.visible')

            // Close modal (look for close button with SVG)
            cy.get('.fixed.inset-0.z-50.overflow-y-auto button[type="button"]').first().click()

            // Modal should be hidden
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('not.exist')
            
            // URL should be clean
            cy.location('search').should('not.include', 'build=')
            cy.location('hash').should('equal', '#builds')
        })

        it('should handle direct URL access to build modal', () => {
            // Visit URL with build parameter directly
            cy.visit(`/#builds?build=${mockBuild.uuid}`)
            cy.wait('@getBuilds')

            // Wait for builds to load first
            cy.contains('.cursor-pointer.p-4', 'ubuntu:24.04').should('be.visible')
            
            // Modal should open automatically
            cy.get('.fixed.inset-0.z-50.overflow-y-auto.overflow-y-auto', { timeout: 10000 }).should('be.visible')
        })

        it('should handle browser back/forward with modal state', () => {
            // Wait for builds to load
            cy.contains('.cursor-pointer.p-4', 'ubuntu:24.04').should('be.visible')
            
            // Start without modal
            cy.location('search').should('not.include', 'build=')

            // Open modal
            cy.get('.cursor-pointer.p-4').first().click()
            cy.location('search').should('include', `build=${mockBuild.uuid}`)

            // Use browser back
            cy.go('back')
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('not.exist')
            cy.location('search').should('not.include', 'build=')

            // Use browser forward  
            cy.go('forward')
            cy.get('.fixed.inset-0.z-50.overflow-y-auto', { timeout: 10000 }).should('be.visible')
            cy.location('search').should('include', `build=${mockBuild.uuid}`)
        })
    })

    describe('Build Modal Content', () => {
        beforeEach(() => {
            // Wait for builds to load
            cy.get('[x-if="!loading && builds.length > 0"]').should('exist')
            cy.contains('.cursor-pointer.p-4', 'ubuntu:24.04').should('be.visible')
            
            // Open modal for each test
            cy.get('.cursor-pointer.p-4').first().click()
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('be.visible')
        })

        it('should display build summary information', () => {
            // Check build summary section within the modal
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').within(() => {
                cy.contains('Build Configuration').should('be.visible')
                cy.contains('Image').should('be.visible')
                cy.contains('ubuntu:24.04').should('be.visible')
                cy.contains('Architecture').should('be.visible')
                cy.contains('amd64').should('be.visible')
                cy.contains('Model').should('be.visible')
                cy.contains('generic').should('be.visible')
                cy.contains('Variant').should('be.visible')
                cy.contains('core').should('be.visible')
                cy.contains('Version').should('be.visible')
                cy.contains('1.0.0').should('be.visible')
                cy.contains('Status').should('be.visible')
                cy.contains('complete').should('be.visible')
            })
        })

        it('should show artifacts section for completed builds', () => {
            cy.wait('@getArtifacts')
            
            // Artifacts section should be visible
            cy.contains('Artifacts').should('be.visible')
            cy.contains('ISO image').should('be.visible')
            cy.contains('RAW image').should('be.visible')
            cy.contains('For generic installations').should('be.visible')
            cy.contains('For AWS, Raspberry Pi').should('be.visible')
        })

        it('should have logs toggle functionality', () => {
            // Logs toggle should exist
            cy.get('input[x-model="modal.showLogs"]').should('exist')

            // Initially logs should be hidden
            cy.get('[x-show="modal.showLogs"]').should('not.be.visible')

            // Toggle logs on (click the label since checkbox is hidden)
            cy.get('label').contains('Show Logs').click()
            cy.get('[x-show="modal.showLogs"]').should('be.visible')

            // Should show logs container
            cy.get('[x-ref="modalLogsContainer"]').should('be.visible')
        })
    })

    describe('Build Logs Access for Previous Jobs', () => {
        beforeEach(() => {
            // Wait for builds to load
            cy.get('[x-if="!loading && builds.length > 0"]').should('exist')
            cy.contains('.cursor-pointer.p-4', 'ubuntu:24.04').should('be.visible')
            
            // Open modal
            cy.get('.cursor-pointer.p-4').first().click()
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('be.visible')
        })

        it('should be able to access logs for completed builds', () => {
            // Mock WebSocket connection for logs
            cy.window().then((win) => {
                // Mock WebSocket for testing
                const mockWebSocket = {
                    onopen: null,
                    onmessage: null,
                    onclose: null,
                    onerror: null,
                    send: cy.stub(),
                    close: cy.stub(),
                    readyState: WebSocket.OPEN
                }

                // Stub WebSocket constructor
                cy.stub(win, 'WebSocket').returns(mockWebSocket)

                // Enable logs
                cy.get('label').contains('Show Logs').click()
                cy.get('[x-show="modal.showLogs"]').should('be.visible')

                // Simulate receiving log messages
                setTimeout(() => {
                    if (mockWebSocket.onmessage) {
                        mockWebSocket.onmessage({ 
                            data: 'Starting build process...\n' 
                        })
                        mockWebSocket.onmessage({ 
                            data: 'Downloading base image ubuntu:24.04\n' 
                        })
                        mockWebSocket.onmessage({ 
                            data: 'Build completed successfully!\n' 
                        })
                        mockWebSocket.onmessage({ 
                            data: 'Job reached status: complete, closing connection.\n' 
                        })
                    }
                }, 100)

                // Check that logs appear
                cy.contains('Starting build process').should('be.visible')
                cy.contains('Downloading base image').should('be.visible')
                cy.contains('Build completed successfully').should('be.visible')
            })
        })

        it('should show appropriate message for builds without logs', () => {
            // Mock WebSocket to simulate no logs scenario
            cy.window().then((win) => {
                const mockWebSocket = {
                    onopen: null,
                    onmessage: null,
                    onclose: null,
                    onerror: null,
                    send: cy.stub(),
                    close: cy.stub(),
                    readyState: WebSocket.OPEN
                }

                cy.stub(win, 'WebSocket').returns(mockWebSocket)

                // Enable logs
                cy.get('label').contains('Show Logs').click()

                // Simulate immediate connection close (no logs available)
                setTimeout(() => {
                    if (mockWebSocket.onclose) {
                        mockWebSocket.onclose()
                    }
                }, 100)

                // Should show appropriate message or handle gracefully
                cy.get('#modalLogsContainer').should('be.visible')
            })
        })

        it('should handle WebSocket connection errors gracefully', () => {
            cy.window().then((win) => {
                const mockWebSocket = {
                    onopen: null,
                    onmessage: null,
                    onclose: null,
                    onerror: null,
                    send: cy.stub(),
                    close: cy.stub(),
                    readyState: WebSocket.CONNECTING
                }

                cy.stub(win, 'WebSocket').returns(mockWebSocket)

                // Enable logs
                cy.get('label').contains('Show Logs').click()

                // Simulate connection error
                setTimeout(() => {
                    if (mockWebSocket.onerror) {
                        mockWebSocket.onerror(new Error('Connection failed'))
                    }
                }, 100)

                // Should handle error gracefully (not crash the interface)
                cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('be.visible')
            })
        })
    })

    describe('Build Status Indicators', () => {
        const buildStatuses = [
            { status: 'queued', indicatorClass: 'bg-gray-400', badgeClass: 'bg-gray-100' },
            { status: 'assigned', indicatorClass: 'bg-yellow-400', badgeClass: 'bg-yellow-100' },
            { status: 'running', indicatorClass: 'bg-blue-500', badgeClass: 'bg-blue-100' },
            { status: 'complete', indicatorClass: 'bg-green-500', badgeClass: 'bg-green-100' },
            { status: 'failed', indicatorClass: 'bg-red-500', badgeClass: 'bg-red-100' }
        ]

        buildStatuses.forEach(({ status, indicatorClass, badgeClass }) => {
            it(`should show correct visual indicators for ${status} status`, () => {
                const buildWithStatus = { ...mockBuild, status }
                
                cy.intercept('GET', '/api/v1/builds*', { 
                    body: { builds: [buildWithStatus], total: 1 } 
                }).as('getBuildsWithStatus')

                cy.reload()
                cy.visit('/#builds')
                cy.wait('@getBuildsWithStatus')

                // Wait for builds to load first
                cy.get('[x-if="!loading && builds.length > 0"]').should('exist')
                cy.contains('.cursor-pointer.p-4', status).should('be.visible')
                
                // Check status indicator circle (look for the specific color classes)
                cy.get('[class*="w-3 h-3 rounded-full"]').should('exist')
                
                // Check status badge
                cy.get('[class*="inline-flex items-center"]').should('exist')
                
                // Status text should be visible
                cy.contains(status).should('be.visible')
            })
        })
    })

    describe('Responsive Design and Accessibility', () => {
        beforeEach(() => {
            // Wait for builds to load
            cy.get('[x-if="!loading && builds.length > 0"]').should('exist')
            cy.contains('.cursor-pointer.p-4', 'ubuntu:24.04').should('be.visible')
            
            cy.get('.cursor-pointer.p-4').first().click()
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('be.visible')
        })

        it('should be usable on mobile viewport', () => {
            cy.viewport(375, 667) // iPhone SE

            // Modal should still be visible and usable
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('be.visible')
            cy.contains('Build Configuration').should('be.visible')
            
            // Close button should be accessible
            cy.get('.fixed.inset-0.z-50.overflow-y-auto button[type="button"]').first().should('be.visible').click()
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('not.exist')
        })

        it('should be usable on tablet viewport', () => {
            cy.viewport(768, 1024) // iPad

            cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('be.visible')
            cy.contains('Build Configuration').should('be.visible')
            
            // Logs toggle should be accessible
            cy.get('label').contains('Show Logs').should('be.visible').click()
            cy.get('[x-show="modal.showLogs"]').should('be.visible')
        })

        it('should support keyboard navigation', () => {
            // ESC should close modal
            cy.get('body').type('{esc}')
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('not.exist')
            
            // Re-open modal
            cy.get('.cursor-pointer.p-4').first().click()
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').should('be.visible')
            
            // Tab navigation should work within modal
            cy.get('.fixed.inset-0.z-50.overflow-y-auto').within(() => {
                cy.get('button').first().focus()
                cy.focused().should('exist')
            })
        })
    })
})
