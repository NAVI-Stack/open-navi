# WhatsApp Connector Validation Plan

## 1. Testing Strategy
- **Unit Tests:** Verify webhook signature validation (`X-Hub-Signature-256`).
- **Integration Tests:** Send mock webhook payloads and assert correct backend routing.
- **E2E Tests:** Live test using a Meta test phone number.

## 2. Security Considerations
- Strictly validate webhook signatures to prevent spoofing.
- Secure storage of the permanent User Access Token.
- Enforce strict allowed phone number mappings.

## 3. Documentation
- Instructions for setting up a Meta App, WhatsApp Business Account, and getting access tokens.
- Webhook configuration guide.
