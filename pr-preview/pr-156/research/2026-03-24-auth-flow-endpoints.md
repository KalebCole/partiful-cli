# Partiful Auth Flow — Endpoint Research

**Date:** 2026-03-24
**Method:** Browser interception on partiful.com login page + app bundle analysis
**Source:** Module 31983 in `_app-86627f0803d70c85.js`

---

## Auth Endpoints

### 1. Send Verification Code

```
POST https://api.partiful.com/sendAuthCode
```

**Request:**
```json
{
  "data": {
    "params": {
      "displayName": "Kaleb Cole",
      "phoneNumber": "+12066993977",
      "silent": false,
      "channelPreference": "sms",
      "captchaToken": "<optional-recaptcha-token>",
      "useAppleBusinessUpdates": false
    }
  }
}
```

**Notes:**
- `channelPreference` can be `"sms"` or `"whatsapp"` (UI shows "Send with WhatsApp instead")
- `captchaToken` appears optional (invisible reCAPTCHA, may be needed for untrusted clients)
- `silent` flag exists for silent auth flows
- There's also `sendAuthCodeTrusted` variant for trusted phone sources

### 2. Verify Code & Get Login Token

```
POST https://api.partiful.com/getLoginToken
```

**Request:**
```json
{
  "data": {
    "params": {
      "phoneNumber": "+12066993977",
      "authCode": "889885",
      "affiliateId": null,
      "utms": {}
    }
  }
}
```

**Response:**
```json
{
  "result": {
    "data": {
      "token": "<firebase-custom-jwt>",
      "shouldUseCookiePersistence": false,
      "isNewUser": false
    }
  }
}
```

### 3. Exchange Custom Token for Firebase Auth

```
POST https://identitytoolkit.googleapis.com/v1/accounts:signInWithCustomToken?key=AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k
```

**Request:**
```json
{
  "token": "<custom-jwt-from-step-2>",
  "returnSecureToken": true
}
```

**Response (Firebase standard):**
```json
{
  "idToken": "<firebase-id-token>",
  "refreshToken": "<firebase-refresh-token>",
  "expiresIn": "3600"
}
```

### 4. (Optional) Look Up User Info

```
POST https://identitytoolkit.googleapis.com/v1/accounts:lookup?key=AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k
```

**Request:**
```json
{
  "idToken": "<firebase-id-token>"
}
```

---

## Complete CLI Auth Flow

```
sendAuthCode(phone) → SMS arrives → getLoginToken(phone, code) → signInWithCustomToken(token) → save refreshToken
```

No reCAPTCHA needed for the REST API calls when using the standard `data.params` envelope.

## Firebase API Key

`AIzaSyCky6PJ7cHRdBKk5X7gjuWERWaKWBHr4_k` — Partiful's Firebase web API key (public, embedded in client).

## SMS Source

- Phone: `+18449460698` — Partiful's SMS sender
- Message format: `{code} is your Partiful verification code`
- Code: 6 digits
