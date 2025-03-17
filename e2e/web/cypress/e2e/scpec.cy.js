describe('Basic Tests for webui', () => {
    beforeEach(() => {
        cy.visit('/')
    })
    const expectedText = [
        "Ubuntu 24.04 LTS",
        "Fedora 40",
        "Alpine 19",
        "Rocky Linux 9",
        "AMD64",
        "ARM64",
        "Core",
        "Standard",
        "Welcome to the Kairos Factory"
    ];

    expectedText.forEach((text) => {
        it(`Checking that ${text} exists and is visible`, () => {
            cy.log(`Checking that ${text} exists and is visible`)
            cy.contains(text).should("exist").should("be.visible")
        });
    });

    it("base images has the proper sizes", () => {
        cy.get(".baseimage-list", { timeout: 1000 }).should("exist").should("be.visible")
        // Check the number of items in the list 6 flavors + byo
        cy.get(".baseimage-list").find("li").should("have.length", 7)
    })

     it("arch has the proper sizes", () => {
        cy.get(".arch-list", { timeout: 1000 }).should("exist").should("be.visible")
        cy.get(".arch-list").find("li").should("have.length", 2)
     })

     it("variant has the proper list sizes", () => {
         cy.get(".variant-list", { timeout: 1000 }).should("exist").should("be.visible")
         cy.get(".variant-list").find("li").should("have.length", 2)
     })

     it("models for arm are disabled if we click on amd64 arch", () => {
        // Click on the AMD64 architecture
        cy.get('ul.arch-list li').contains('AMD64').click()
        cy.get("ul.model-list li").then(($lis) => {
            cy.wrap($lis).each(($li) => {
                cy.wrap($li).find("div.model-title").then(($div) => {
                    if ($div.text().includes("Generic")) { // Generic is the only model that should be visible always
                        } else {
                            cy.wrap($li).should("have.css", "display", "none")
                    };
                });
            });
        });
    });

    it("models for arm are enabled if we click on arm64 arch", () => {
        // Click on the ARM64 architecture
        cy.get('ul.arch-list li').contains('ARM64').click()
        cy.get("ul.model-list li").then(($lis) => {
            cy.wrap($lis).each(($li) => {
                cy.wrap($li).find("div.model-title").then(($div) => {
                    if ($div.text().includes("Generic")) { // Generic is the only model that should be visible always
                    } else {
                        cy.wrap($li).should("have.css", "display", "block")
                    };
                });
            });
        });
    });
})