# WhatsApp Connector Development Plan

## 1. MVP Definition
- Expose a webhook endpoint to receive incoming WhatsApp messages.
- Support text and basic media (images/audio) ingestion.
- Route commands to NAVI.
- Send text responses back via Graph API.

## 2. Architecture Integration
- Implement `connectors.Connector` and `connectors.WebhookHandler`.
- Register the webhook with the central gateway.

## 3. Future Enhancements (v2)
- Support WhatsApp Interactive Messages (Buttons, Lists) for HITL approvals.
- Session timeout management (24-hour service window rules).
