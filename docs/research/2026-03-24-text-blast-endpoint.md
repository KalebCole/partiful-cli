# Partiful Text Blast API — Endpoint Research

**Date:** 2026-03-24
**Method:** Browser interception on partiful.com (logged in as Kaleb Cole)
**Source file:** `6631.793447c2446d40ae.js` → `SendTextBlastModal.tsx`

---

## Endpoint

```
POST https://api.partiful.com/createTextBlast
```

Uses the same Firebase callable function pattern as all other Partiful API endpoints:
- Auth via Firebase access token in `Authorization: Bearer <token>` header
- Request body wrapped in standard `data.params` envelope
- Includes `amplitudeDeviceId`, `amplitudeSessionId`, `userId` metadata

## Request Payload

```json
{
  "data": {
    "params": {
      "eventId": "<event-id>",
      "message": {
        "text": "<message-text>",
        "to": ["GOING", "MAYBE", "DECLINED"],
        "showOnEventPage": true,
        "images": [
          {
            "url": "<uploaded-image-url>",
            "upload": {
              "contentType": "image/jpeg",
              "size": 123456
            }
          }
        ]
      }
    },
    "amplitudeDeviceId": "<device-id>",
    "amplitudeSessionId": <timestamp>,
    "userId": "<firebase-uid>"
  }
}
```

### Field Details

| Field | Type | Required | Notes |
|---|---|---|---|
| `eventId` | string | Yes | Partiful event ID |
| `message.text` | string | Yes | Message body, max 480 chars |
| `message.to` | string[] | Yes | Array of guest statuses to send to |
| `message.showOnEventPage` | boolean | Yes | Whether to show in activity feed |
| `message.images` | array | No | Uploaded image objects (max 1 image, max 5MB total) |

### Valid `to` Values (Guest Status Enum — `LF`)

From module `73621` in the app bundle:

| Value | UI Label | Typical Use in Blasts |
|---|---|---|
| `GOING` | Going | ✅ Primary target |
| `MAYBE` | Maybe | ✅ Common target |
| `DECLINED` | Can't Go | ✅ Available |
| `SENT` | Invited | ⚠️ Only for small groups |
| `INTERESTED` | Interested | Available |
| `WAITLIST` | Waitlist | Available (if enabled) |
| `APPROVED` | Approved | Available (ticketed events) |
| `RESPONDED_TO_FIND_A_TIME` | Find-a-Time | Available |

**Note:** The UI shows "Texts to large groups of Invited guests are not allowed" — there's a server-side limit on blasting to `SENT` status guests.

### Limits

- **Max 10 text blasts per event** (enforced client-side as `f=10`)
- **Max 480 characters** per message
- **Max 5MB** total image upload size
- **Max 1 image** per blast
- **Allowed image types:** Checked via `ee.V2` array (likely standard image MIME types)
- **Event must not be expired** (`EVENT_EXPIRED` check)
- **Must have guests** (`NO_GUESTS` check)

## UI → API Mapping

| UI Element | API Field |
|---|---|
| "Going (9)" pill (selected) | `to: ["GOING"]` |
| "Maybe (1)" pill (selected) | `to` includes `"MAYBE"` |
| "Can't Go (1)" pill (selected) | `to` includes `"DECLINED"` |
| "Select all (11)" | `to: ["GOING", "MAYBE", "DECLINED"]` (all available) |
| "Also show in activity feed" checkbox | `showOnEventPage: true/false` |
| Message textarea | `message.text` |
| Image upload | `message.images` array |

## Other Discovered Endpoints (Same Session)

| Endpoint | Method | Purpose |
|---|---|---|
| `getContacts` | POST | List user's contacts (paginated, 1000/page) |
| `getHostTicketTypes` | POST | Get ticket types for an event |
| `getEventDiscoverStatus` | POST | Check if event is discoverable |
| `getEventTicketingEligibility` | POST | Check ticketing eligibility |
| `getEventPermission` | POST | Check user's permissions on event |
| `getUsers` | POST | Batch lookup users by ID (batches of ~5-10) |

## Previously Discovered Endpoints (Earlier Sessions)

| Endpoint | Method | Purpose |
|---|---|---|
| `addGuest` | POST | RSVP / add guest to event |
| `removeGuest` | POST | Remove guest from event |
| `getMutualsV2` | POST | Get mutual connections |
| `getInvitableContactsV2` | POST | Get invitable contacts for event |
| `getGuestsCsvV2` | POST | Server-side CSV export of guest list |

## Auth Token for CLI

The CLI already has auth working via Firebase refresh token → access token exchange.
The `createTextBlast` endpoint uses the same auth pattern — just needs the access token
in the standard callable function request envelope.
