package gateway

// apikeys_test.go
// API key lifecycle tests are covered in server_test.go:
//   TestGatewayAPIKeyManagement — create / list / revoke / auth
//   TestAuthMiddlewareAPIKeyContext — context propagation
//   TestAuthMiddlewareInvalidAPIKey — rejection of bad keys
//   TestGatewayOnboardingClaim — first-call-wins claim flow
