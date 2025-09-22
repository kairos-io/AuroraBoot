# E2E Web Tests for AuroraBoot

This directory contains end-to-end tests for the AuroraBoot web interface using Cypress.

## Test Coverage

The test suite covers the following functionality:

### Original Features (`scpec.cy.js`)
- Accordion-based build form interface
- Form validation and submission
- Architecture-specific options (ARM/AMD64)
- BYOI (Bring Your Own Image) functionality

### New Tab and Filter Features (`builds-tab.cy.js`)
- Tab navigation between "New Build" and "Builds List"
- URL synchronization with tab state
- Status filter functionality with URL persistence
- Tab-specific URL parameter handling
- Browser back/forward navigation
- Filter state preservation across tab switches
- Direct URL access to filtered views

### Build Modal and Logs (`build-logs.cy.js`)
- Build list display and interaction
- Build modal opening/closing
- URL synchronization with selected build
- Build summary display
- Artifact listing for completed builds
- Log access for previous builds via WebSocket
- Status indicators and visual feedback
- Responsive design testing
- Keyboard accessibility

### Advanced Options Feature (`advanced-options.cy.js`)
- Advanced options checkbox functionality
- Kairos-init version field visibility toggling
- Form submission with hidden/visible advanced fields
- Config API integration for default values
- User interaction testing (label clicking, keyboard navigation)
- State persistence across tab switches
- Error handling for config API failures
- Accessibility and user experience validation

## Running Tests

### Prerequisites
```bash
cd e2e/web
npm install
```

### Test Commands

#### Run all tests
```bash
npm test
```

#### Open Cypress UI for interactive testing
```bash
npm run test:open
```

#### Run specific test suites
```bash
# Test tab navigation and filtering
npm run test:builds

# Test build modal and logs
npm run test:logs

# Test original form functionality
npm run test:original

# Test advanced options functionality
npm run test:advanced

# Run headless
npm run test:headless
```

### Running with Different Browsers
```bash
# Chrome (default)
npx cypress run

# Firefox
npx cypress run --browser firefox

# Edge
npx cypress run --browser edge
```

## Test Structure

### API Mocking
Tests use `cy.intercept()` to mock API responses, allowing for:
- Testing different build statuses
- Simulating loading states
- Testing error conditions
- Consistent test data

### WebSocket Testing
Build logs functionality is tested by:
- Mocking WebSocket connections
- Simulating real-time log streaming
- Testing connection errors and recovery
- Verifying proper connection cleanup

### URL Testing
Comprehensive URL state testing covers:
- Hash-based tab navigation (`#builds`)
- Query parameter persistence (`?status=running&build=uuid`)
- Tab-specific parameter isolation
- Browser navigation integration

## Test Data

Tests use mock data that matches the actual API schema:
```javascript
const mockBuild = {
    uuid: '123e4567-e89b-12d3-a456-426614174000',
    image: 'ubuntu:24.04',
    architecture: 'amd64',
    model: 'generic',
    variant: 'core',
    version: '1.0.0',
    status: 'complete',
    created_at: '2024-01-01T12:00:00Z'
}
```

## Best Practices

### Test Isolation
- Each test starts with a clean state
- API responses are mocked per test
- No dependencies between tests

### Responsive Testing
- Tests verify functionality across viewport sizes
- Mobile, tablet, and desktop layouts are covered
- Touch interactions are tested where applicable

### Accessibility
- Keyboard navigation is tested
- Focus management is verified
- Screen reader compatibility is considered

## Integration with Backend Tests

The E2E tests complement the Go-based API tests:
- **Go tests**: Verify API contract and business logic
- **Cypress tests**: Verify user interface and user experience
- **Combined**: Ensure end-to-end functionality

## CI/CD Integration

Tests can be integrated into CI/CD pipelines:
```bash
# Start AuroraBoot server
./start.sh &

# Wait for server to be ready
sleep 5

# Run E2E tests
cd e2e/web && npm test

# Clean up
pkill auroraboot
```

## Debugging Tests

### Interactive Mode
```bash
npm run test:open
```

### Debug Information
- Screenshots are captured on failure
- Video recordings for headless runs
- Console logs are preserved
- Network request details are logged

### Common Issues
1. **Server not running**: Ensure AuroraBoot is accessible at `http://localhost:8080`
2. **Timing issues**: Use `cy.wait()` for async operations
3. **Element not found**: Verify Alpine.js has initialized with `cy.get('#accordion-collapse').should('exist')`
