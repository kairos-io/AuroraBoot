describe('Basic Tests for webui', () => {
    beforeEach(() => {
        cy.visit('/')
        // Wait for the page to be fully loaded
        cy.get('.accordion-section').should('exist')
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
    it('Accordion sections exist and are properly structured', () => {
        cy.get('.accordion-section').should('exist')
        cy.get('.accordion-section').should('have.length.at.least', 3) // Base Image, Architecture, and Variant sections
        cy.get('.accordion-section').each(($section) => {
            cy.wrap($section).find('.accordion-header').should('exist')
            cy.wrap($section).find('.accordion-content').should('exist')
        })
    })
    // Test main heading
    it('Main heading contains Kairos Factory', () => {
        cy.get('h1').should('contain', 'Kairos Factory')
    })
    // Test accordion interaction
    it('Accordion sections can be expanded and collapsed', () => {
        // Get all accordion sections
        cy.get('.accordion-section').then(($sections) => {
            // Take the second section (index 1) to test with
            const testSection = $sections.eq(1)
            // Click the section header
            cy.wrap(testSection).find('.accordion-header').click()
            // Wait a moment for any animations
            cy.wait(100)
            // Get the current state
            cy.wrap(testSection).then(($section) => {
                const isActive = $section.hasClass('active')
                // Click again to toggle the state
                cy.wrap($section).find('.accordion-header').click()
                // Wait a moment for any animations
                cy.wait(100)
                // Verify the state has changed
                cy.wrap($section).should('not.have.class', 'active')
                // Click one more time to verify we can expand again
                cy.wrap($section).find('.accordion-header').click()
                // Wait a moment for any animations
                cy.wait(100)
                // Verify it's active again
                cy.wrap($section).should('have.class', 'active')
            })
        })
    })
    // Test content visibility in expanded sections
    expectedText.forEach((text) => {
        it(`Checking that ${text} exists and is visible when its section is expanded`, () => {
            // Find the section containing the text
            cy.contains(text).parents('.accordion-section').then(($section) => {
                // If the section is not active, click its header
                if (!$section.hasClass('active')) {
                    cy.wrap($section).find('.accordion-header').click()
                }
                // Now check that the text is visible
                cy.contains(text).should("exist").should("be.visible")
            })
        });
    });
    const values = [
        "ubuntu:24.04",
        "fedora:40",
        "alpine:3.21",
        "rockylinux:9",
        "debian:12",
    ]
    values.forEach((value) => {
        it(`Input with value ${value} exists and is accessible when section is expanded`, () => {
            // Find the section containing the input
            cy.get(`input[type="radio"][name="base_image"][value="${value}"]`).parents('.accordion-section').then(($section) => {
                // If the section is not active, click its header
                if (!$section.hasClass('active')) {
                    cy.wrap($section).find('.accordion-header').click()
                }
                // Now check that the input exists
                cy.get(`input[type="radio"][name="base_image"][value="${value}"]`, {timeout: 1000}).should("exist")
            })
        });
    });
    it("base images has the proper sizes", () => {
        // Ensure the base image section is expanded
        cy.get('.accordion-section').first().find('.accordion-header').click()
        cy.get(".baseimage-list", { timeout: 1000 }).should("exist").should("be.visible")
        // Check the number of items in the list 6 flavors + byo
        cy.get(".baseimage-list").find("li").should("have.length", 7)
    })
    it("arch has the proper sizes", () => {
        // Find and expand the architecture section
        cy.contains('Architecture').parents('.accordion-section').find('.accordion-header').click()
        cy.get(".arch-list", { timeout: 1000 }).should("exist").should("be.visible")
        cy.get(".arch-list").find("li").should("have.length", 2)
    })
    it("variant has the proper list sizes", () => {
        // Find and expand the variant section
        cy.contains('Variant').parents('.accordion-section').find('.accordion-header').click()
        cy.get(".variant-list", { timeout: 1000 }).should("exist").should("be.visible")
        cy.get(".variant-list").find("li").should("have.length", 2)
    })
    it("models for arm are disabled if we click on amd64 arch", () => {
        // Ensure architecture section is expanded
        cy.contains('Architecture').parents('.accordion-section').find('.accordion-header').click()
        // Click on the AMD64 architecture
        cy.get('ul.arch-list li').contains('AMD64').click()
        // Expand the model section
        cy.contains('Model').parents('.accordion-section').find('.accordion-header').click()
        cy.get("ul.model-list li").then(($lis) => {
            cy.wrap($lis).each(($li) => {
                cy.wrap($li).find("[data-js='model-title']").then(($div) => {
                    if ($div.text().includes("Generic")) { // Generic is the only model that should be visible always
                        } else {
                            cy.wrap($li).should("have.css", "display", "none")
                    };
                });
            });
        });
    });
    it("models for arm are enabled if we click on arm64 arch", () => {
        // Ensure architecture section is expanded
        cy.contains('Architecture').parents('.accordion-section').find('.accordion-header').click()
        // Click on the ARM64 architecture
        cy.get('ul.arch-list li').contains('ARM64').click()
        // Expand the model section
        cy.contains('Model').parents('.accordion-section').find('.accordion-header').click()
        cy.get("ul.model-list li").then(($lis) => {
            cy.wrap($lis).each(($li) => {
                cy.wrap($li).find("[data-js='model-title']").then(($div) => {
                    if ($div.text().includes("Generic")) { // Generic is the only model that should be visible always
                    } else {
                        cy.wrap($li).should("have.css", "display", "block")
                    };
                });
            });
        });
    });
})